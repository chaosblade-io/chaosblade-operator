/*
 * Copyright 2025 The ChaosBlade Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"flag"
	"net/http"
	"runtime"
	"strings"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	apiruntime "k8s.io/apimachinery/pkg/runtime"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis"
	"github.com/chaosblade-io/chaosblade-operator/pkg/controller"
	operator "github.com/chaosblade-io/chaosblade-operator/pkg/runtime"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
	webhookcfg "github.com/chaosblade-io/chaosblade-operator/pkg/webhook"
	mutator "github.com/chaosblade-io/chaosblade-operator/pkg/webhook/pod"
	"github.com/chaosblade-io/chaosblade-operator/version"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("Version of operator-sdk: %v", sdkVersion.Version)
	logrus.Infof("Operator Version: %v", version.Version)
	logrus.Infof("Operator Product: %v", version.Product)
	logrus.Infof("Build Time: %v", version.BuildTime)
	logrus.Infof("Git Commit: %v", version.GitCommit)
	logrus.Infof("Git Branch: %v", version.GitBranch)
	logrus.Infof("Platform: %v", version.Platform)
	logrus.Infof("Daemonset Enable: %t", chaosblade.DaemonsetEnable)
}

func main() {
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddFlagSet(operator.FlagSet())
	pflag.CommandLine.AddFlagSet(webhookcfg.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	// 添加版本标志
	showVersion := pflag.Bool("version", false, "显示版本信息")
	pflag.Parse()

	// 如果只是查看版本，则显示后退出
	if *showVersion {
		printVersion()
		return
	}

	initLogger()
	printVersion()

	cfg, err := config.GetConfig()
	if err != nil {
		logrus.Fatalf("Get apiserver config error, %v", err)
	}
	err = leader.Become(context.Background(), "chaosblade-operator-lock")
	if err != nil {
		logrus.Fatalf("Become leader error, %v", err)
	}
	cfg.QPS = operator.QPS
	mgr, err := createManager(cfg)
	if err != nil {
		logrus.Fatalf("Create operator manager error, %v", err)
	}
	addComponentsToManager(mgr)
	logrus.Infoln("Starting the manager.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		logrus.Fatalf("Manager exited non-zero, %v", err)
	}
}

func addComponentsToManager(mgr manager.Manager) {
	logrus.Infof("Add all resources to scheme")
	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		logrus.Fatalf("Add all resources to scheme error, %v", err)
	}
	logrus.Infof("Add all controllers to manager")
	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		logrus.Fatalf("Add all controllers to manager error, %v", err)
	}
	if webhookcfg.Enable {
		logrus.Infof("Webhook enabled, add it to manager")
		if err := addWebhook(mgr); err != nil {
			logrus.Fatalf("Add webhook to manager error, %v", err)
		}
	}
}

// Init logrus and controller-runtime log
func initLogger() {
	level, err := logrus.ParseLevel(operator.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)
	log.SetLogger(zap.Logger())
}

func addWebhook(m manager.Manager) error {
	server := webhook.NewServer(webhook.Options{
		Port: webhookcfg.Port,
	})
	if err := m.Add(server); err != nil {
		return err
	}
	logrus.Infof("registering %s to the webhook server", "mutating-pods")
	server.Register("/mutating-pods", &webhook.Admission{Handler: &mutator.Mutator{}})
	return nil
}

// createManager supports multi namespaces configuration
func createManager(cfg *rest.Config) (manager.Manager, error) {
	scheme := apiruntime.NewScheme()
	runtimeutil.Must(metav1.AddMetaToScheme(scheme))
	runtimeutil.Must(corev1.AddToScheme(scheme))
	runtimeutil.Must(apis.AddToScheme(scheme))
	watchNamespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		return nil, err
	}
	logrus.Infof("Get watch namespace is %s", watchNamespace)
	if strings.Contains(watchNamespace, ",") {
		defaultNsps := make(map[string]cache.Config)
		for _, nsp := range strings.Split(watchNamespace, ",") {
			defaultNsps[nsp] = cache.Config{}
		}
		return manager.New(cfg, manager.Options{
			Cache: cache.Options{
				Scheme:            scheme,
				DefaultNamespaces: defaultNsps,
			},
			Scheme: scheme,
			MapperProvider: func(c *rest.Config, httpClient *http.Client) (meta.RESTMapper, error) {
				return apiutil.NewDynamicRESTMapper(c, httpClient)
			},
			NewClient: channel.NewClientFunc(),
		})
	}
	return manager.New(cfg, manager.Options{
		Cache: cache.Options{
			Scheme:            scheme,
			DefaultNamespaces: map[string]cache.Config{watchNamespace: {}},
		},
		Scheme: scheme,
		MapperProvider: func(c *rest.Config, httpClient *http.Client) (meta.RESTMapper, error) {
			return apiutil.NewDynamicRESTMapper(c, httpClient)
		},
		NewClient: channel.NewClientFunc(),
	})
}

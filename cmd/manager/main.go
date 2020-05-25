/*
 * Copyright 1999-2020 Alibaba Group Holding Ltd.
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
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis"
	"github.com/chaosblade-io/chaosblade-operator/pkg/controller"
	operator "github.com/chaosblade-io/chaosblade-operator/pkg/runtime"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
	webhookcfg "github.com/chaosblade-io/chaosblade-operator/pkg/webhook"
	mutator "github.com/chaosblade-io/chaosblade-operator/pkg/webhook/pod"
	"github.com/chaosblade-io/chaosblade-operator/version"
)

var log = logf.Log.WithName("cmd")

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
	log.Info(fmt.Sprintf("Operator Version: %v", version.Version))
	log.Info(fmt.Sprintf("Operator Product: %v", version.Product))
	log.Info(fmt.Sprintf("Chaosblade Version: %v", chaosblade.Version))
}

func main() {
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	pflag.CommandLine.AddFlagSet(operator.FlagSet())
	pflag.CommandLine.AddFlagSet(webhookcfg.FlagSet())
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()
	logf.SetLogger(zap.Logger())

	printVersion()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "chaosblade-operator-lock")
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	mgr, err := createManager(cfg)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}
	if webhookcfg.Enable {
		if err := addWebhook(mgr); err != nil {
			log.Error(err, "add webhook failed")
			os.Exit(1)
		}
	}
	log.Info("Starting the Cmd.")
	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}

func addWebhook(m manager.Manager) error {
	// Setup webhooks
	hookServer := &webhook.Server{
		Port: webhookcfg.Point,
	}
	if err := m.Add(hookServer); err != nil {
		return err
	}
	klog.Infof("registering mutating-pods to the webhook server")
	hookServer.Register("/mutating-pods", &webhook.Admission{Handler: &mutator.Mutator{}})
	return nil
}

func createManager(cfg *rest.Config) (manager.Manager, error) {
	watchNamespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		return nil, err
	}
	if strings.Contains(watchNamespace, ",") {
		namespaces := strings.Split(watchNamespace, ",")
		return manager.New(cfg, manager.Options{
			NewCache: cache.MultiNamespacedCacheBuilder(namespaces),
			MapperProvider: func(c *rest.Config) (meta.RESTMapper, error) {
				return apiutil.NewDynamicRESTMapper(c)
			},
			NewClient: channel.NewClientFunc(),
		})
	}
	return manager.New(cfg, manager.Options{
		Namespace: watchNamespace,
		MapperProvider: func(c *rest.Config) (meta.RESTMapper, error) {
			return apiutil.NewDynamicRESTMapper(c)
		},
		NewClient: channel.NewClientFunc(),
	})
}

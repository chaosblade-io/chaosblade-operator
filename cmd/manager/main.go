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
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/chaosblade-io/chaosblade-operator/pkg/apis"
	"github.com/chaosblade-io/chaosblade-operator/pkg/controller"
	"github.com/chaosblade-io/chaosblade-operator/version"

	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/meta"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost               = "0.0.0.0"
	metricsPort         int32 = 8383
	operatorMetricsPort int32 = 8686
)
var log = logf.Log.WithName("cmd")

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
	log.Info(fmt.Sprintf("Operator Version: %v", version.Ver))
	log.Info(fmt.Sprintf("Operator Vendor: %v", version.Vendor))
}

func main() {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())
	// Add the controller meta flag set to the cli
	pflag.CommandLine.AddFlagSet(meta.FlagSet())
	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()
	// Use a zap logr.Logger implementation. If none of the zap
	// flags are configured (or if the zap flag set is not being
	// used), this defaults to a production zap logger.
	//
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
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

	log.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
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
			MapperProvider: func(c *rest.Config) (apimeta.RESTMapper, error) {
				return apiutil.NewDynamicRESTMapper(c)
			},
			NewClient: channel.NewClientFunc(),
		})
	}
	return manager.New(cfg, manager.Options{
		Namespace: watchNamespace,
		MapperProvider: func(c *rest.Config) (apimeta.RESTMapper, error) {
			return apiutil.NewDynamicRESTMapper(c)
		},
		NewClient: channel.NewClientFunc(),
	})
}

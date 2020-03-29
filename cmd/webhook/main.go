/*
 * Copyright 1999-2019 Alibaba Group Holding Ltd.
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
	"flag"
	"fmt"
	"os"
	"runtime"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/chaosblade-io/chaosblade-operator/pkg/apis"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/meta"
	hook "github.com/chaosblade-io/chaosblade-operator/pkg/webhook"
	mutator "github.com/chaosblade-io/chaosblade-operator/pkg/webhook/pod"
)

// Change below variables to serve metrics on different host or port.
var (
	DisableWebhookConfigInstaller bool
	BindPort                      int
)
var log = logf.Log.WithName("cmd")

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
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

	pflag.BoolVar(&DisableWebhookConfigInstaller, "disable-webhook-config-installer", false,
		"disable the installer in the webhook server, so it won't install webhook configuration resources during bootstrapping")
	pflag.IntVar(&BindPort, "port", 443, "The port on which to serve HTTPS.")
	pflag.StringVar(&mutator.SidecarImage, "sidecar-image", "", "sidecar container images.")
	pflag.Int32Var(&mutator.FuseServerPort, "fuse-port", 65534, "Fuse server port.")

	// Use a zap logr.Logger implementation. If none of the zap
	// flags are configured (or if the zap flag set is not being
	// used), this defaults to a production zap logger.
	//
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.Logger())

	pflag.Parse()
	printVersion()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{})
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

	namespace := os.Getenv("NAMESPACE")
	if len(namespace) == 0 {
		log.Error(err, "can not found NAMESPACE env")
		os.Exit(1)
	}
	ws, err := webhook.NewServer("chaosblade-admission-server", mgr, webhook.ServerOptions{
		Port:                          int32(BindPort),
		CertDir:                       "/etc/chaosblade/cert",
		DisableWebhookConfigInstaller: &DisableWebhookConfigInstaller,
		BootstrapOptions: &webhook.BootstrapOptions{
			MutatingWebhookConfigName:   "chaosblade-mutating-webhook-configuration",
			ValidatingWebhookConfigName: "chaosblade-validating-webhook-configuration",
			Secret: &types.NamespacedName{
				Namespace: namespace,
				Name:      "chaosblade-admission-server",
			},

			Service: &webhook.Service{
				Namespace: namespace,
				Name:      "chaosblade-admission-server",
				// Selectors should select the pods that runs this webhook server.
				Selectors: map[string]string{
					"app": "chaosblade-admission-server",
				},
			},
		},
	})
	if err != nil {
		klog.Errorf("unable to create a new webhook server, %v", err)
		os.Exit(1)
	}

	klog.Infof("registering webhooks to the webhook server")
	err = hook.AddToManager(ws, mgr)
	if err != nil {
		log.Error(err, "unable to register webhooks in the admission server")
		os.Exit(1)
	}
	log.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}

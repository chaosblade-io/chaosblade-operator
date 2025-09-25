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

package chaosblade

import (
	"github.com/spf13/pflag"

	"github.com/chaosblade-io/chaosblade-operator/version"
)

var (
	ImageRepository     string
	Version             string
	PullPolicy          string
	DaemonsetEnable     bool
	RemoveBladeInterval string
	DownloadUrl         string
)

const (
	OperatorChaosBladePath  = "/opt/chaosblade"
	OperatorChaosBladeBin   = "/opt/chaosblade/bin"
	OperatorChaosBladeLib   = "/opt/chaosblade/lib"
	OperatorChaosBladeYaml  = "/opt/chaosblade/yaml"
	OperatorChaosBladeBlade = "/opt/chaosblade/blade"
)

const (
	DaemonsetPodName           = "chaosblade-tool"
	DefaultRemoveBladeInterval = "72h"
)

var DaemonsetPodLabels = map[string]string{
	"app": "chaosblade-tool",
}

// set in runtime
var (
	DaemonsetPodNamespace string
	DaemonsetPodNames     = map[string]string{}
)

var Products = map[string]*ProductConstant{}

var Constant *ProductConstant

type ProductConstant struct {
	ImageRepoFunc func() string
}

var f *pflag.FlagSet

func init() {
	f = pflag.NewFlagSet("chaosblade", pflag.ExitOnError)
	// chaosblade config
	f.StringVar(&Version, "chaosblade-version", version.Version, "Chaosblade tool version")
	f.StringVar(&ImageRepository, "chaosblade-image-repository", "chaosbladeio/chaosblade-tool", "Image repository of chaosblade tool")
	f.StringVar(&PullPolicy, "chaosblade-image-pull-policy", "IfNotPresent", "Pulling policy of chaosblade image, default value is IfNotPresent.")
	f.BoolVar(&DaemonsetEnable, "daemonset-enable", false, "Deploy chaosblade daemonset to resolve chaos experiment environment of network, default value is false.")
	f.StringVar(&RemoveBladeInterval, "remove-blade-interval", DefaultRemoveBladeInterval, "Periodically clean up blade state is destroying, default value is 24h.")
	f.StringVar(&DownloadUrl, "chaosblade-download-url", "", "The chaosblade downloaded address which works when the chaosblade is deployed in download mode.")
	f.StringVar(&DaemonsetPodNamespace, "chaosblade-namespace", "chaosblade", "The chaosblade deployment namespace")
}

func FlagSet() *pflag.FlagSet {
	return f
}

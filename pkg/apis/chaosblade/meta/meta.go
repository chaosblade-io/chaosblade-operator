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

package meta

import (
	"fmt"
	"path"

	"github.com/spf13/pflag"

	"github.com/chaosblade-io/chaosblade-operator/version"
)

const (
	Community = "community"
	AHAS      = "ahas"
)

const (
	DefaultImageRepo = "registry.cn-hangzhou.aliyuncs.com/chaosblade/chaosblade-tool"
)

var Vendors = map[string]*chaosBladeConstant{
	// FOR OPENSOURCE
	Community: {
		Home:          "/opt/chaosblade",
		BladeBin:      "/opt/chaosblade/blade",
		PodName:       "chaosblade-tool",
		ImageRepoFunc: ImageRepoForCommunity,
		PodLabels:     map[string]string{"app": "chaosblade-tool"},
	},
	// FOR ALIYUN AHAS
	AHAS: {
		Home:          "/opt/chaosblade",
		BladeBin:      "/opt/chaosblade/blade",
		PodName:       "ahas-agent",
		ImageRepoFunc: ImageRepoForAliyun,
		PodLabels:     map[string]string{"app": "ahas"},
	},
}

var Constant *chaosBladeConstant

type chaosBladeConstant struct {
	Home          string
	BladeBin      string
	PodName       string
	ImageRepoFunc func() string
	PodLabels     map[string]string
}

var ImageRepoForCommunity = func() string {
	return imageRepo
}

const (
	prodEnv      = "prod"
	publicRegion = "cn-public"
)

var ImageRepoForAliyun = func() string {
	region := aliyunRegion
	env := runtimeEnv
	if region == publicRegion {
		if env == prodEnv {
			return fmt.Sprintf("registry.cn-hangzhou.aliyuncs.com/ahascr-public/chaosblade-tool")
		}
		return fmt.Sprintf("registry.cn-hangzhou.aliyuncs.com/ahas-public/chaosblade-tool")
	}
	if env == prodEnv {
		return fmt.Sprintf("registry-vpc.%s.aliyuncs.com/ahascr/chaosblade-tool", region)
	}
	return fmt.Sprintf("registry-vpc.%s.aliyuncs.com/ahas/chaosblade-tool", region)
}

type PointerString *string

var (
	metaFlagSet *pflag.FlagSet

	aliyunRegion      string
	runtimeEnv        string
	imageRepo         string
	chaosBladeVersion string
	pullPolicy        string
	namespace         string
)

func init() {
	metaFlagSet = pflag.NewFlagSet("meta", pflag.ExitOnError)
	metaFlagSet.StringVar(&aliyunRegion, "aliyun-region", "cn-public", "Aliyun region")
	metaFlagSet.StringVar(&imageRepo, "image-repo", DefaultImageRepo,
		fmt.Sprintf("ChaosBlade image repository, default value is %s", DefaultImageRepo))
	metaFlagSet.StringVar(&chaosBladeVersion, "blade-version", "latest",
		"Chaosblade image version, default value is latest")
	metaFlagSet.StringVar(&pullPolicy, "pull-policy", "IfNotPresent", "Pull image policy, default value is IfNotPresent")
	metaFlagSet.StringVar(&namespace, "namespace", "kube-system", "the kubernetes namespace which chaosblade operator deployed")
	metaFlagSet.StringVar(&runtimeEnv, "aliyun-env", "prod", "environment")

	switch version.Vendor {
	case Community:
		Constant = Vendors[Community]
	case AHAS:
		Constant = Vendors[AHAS]
	default:
		Constant = Vendors[Community]
	}
}

func FlagSet() *pflag.FlagSet {
	return metaFlagSet
}

func GetChaosBladeVersion() string {
	return chaosBladeVersion
}

func GetPullImagePolicy() string {
	return pullPolicy
}

func GetNamespace() string {
	return namespace
}

func GetChaosBladePkgPath() string {
	return path.Join(path.Dir(Constant.Home), fmt.Sprintf("chaosblade-%s.tar.gz", chaosBladeVersion))
}

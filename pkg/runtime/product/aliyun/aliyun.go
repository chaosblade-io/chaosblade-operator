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

package aliyun

import (
	"fmt"

	"github.com/spf13/pflag"

	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
)

var (
	// flag
	RegionId    string
	Environment string
)

const (
	AHAS         = "ahas"
	prodEnv      = "prod"
	publicRegion = "cn-public"
)

var f *pflag.FlagSet

func init() {
	f = pflag.NewFlagSet("aliyun", pflag.ExitOnError)
	f.StringVar(&RegionId, "aliyun-region-id", "", "Region id for cloud provider")
	f.StringVar(&Environment, "aliyun-environment", "", "Environment for cloud provider")

	chaosblade.Products[AHAS] = &chaosblade.ProductConstant{
		ImageRepoFunc: ImageRepoForAliyun,
	}
}

var ImageRepoForAliyun = func() string {
	if RegionId == publicRegion {
		if Environment == prodEnv {
			return fmt.Sprintf("registry.cn-hangzhou.aliyuncs.com/ahascr-public/chaosblade-tool")
		}
		return fmt.Sprintf("registry.cn-hangzhou.aliyuncs.com/ahas-public/chaosblade-tool")
	}
	if Environment == prodEnv {
		return fmt.Sprintf("registry-vpc.%s.aliyuncs.com/ahascr/chaosblade-tool", RegionId)
	}
	return fmt.Sprintf("registry-vpc.%s.aliyuncs.com/ahas/chaosblade-tool", RegionId)
}

func FlagSet() *pflag.FlagSet {
	return f
}

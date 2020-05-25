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

package runtime

import (
	"fmt"
	"path"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/spf13/pflag"

	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/product/aliyun"
	_ "github.com/chaosblade-io/chaosblade-operator/pkg/runtime/product/community"
	"github.com/chaosblade-io/chaosblade-operator/version"
)

var flagSet *pflag.FlagSet

func init() {
	flagSet = pflag.NewFlagSet("operator", pflag.ExitOnError)
	flagSet.AddFlagSet(aliyun.FlagSet())
	flagSet.AddFlagSet(chaosblade.FlagSet())

	initRuntimeData()
}

func initRuntimeData() {
	chaosblade.Constant = chaosblade.Products[version.Product]
}

func FlagSet() *pflag.FlagSet {
	return flagSet
}

func GetOperatorNamespace() string {
	operatorNs, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return chaosblade.Namespace
	}
	return operatorNs
}

func GetChaosBladePkgPath() string {
	return path.Join(path.Dir(chaosblade.Constant.Home), fmt.Sprintf("chaosblade-%s.tar.gz", chaosblade.Version))
}

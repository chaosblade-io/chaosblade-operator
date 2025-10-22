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

package runtime

import (
	"github.com/spf13/pflag"

	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/product/aliyun"
	_ "github.com/chaosblade-io/chaosblade-operator/pkg/runtime/product/community"
	"github.com/chaosblade-io/chaosblade-operator/version"
)

var (
	flagSet                 *pflag.FlagSet
	LogLevel                string
	MaxConcurrentReconciles int
	QPS                     float32
)

func init() {
	flagSet = pflag.NewFlagSet("operator", pflag.ExitOnError)
	flagSet.StringVar(&LogLevel, "log-level", "info", "Log level, such as panic|fatal|error|warn|info|debug|trace")
	flagSet.IntVar(&MaxConcurrentReconciles, "reconcile-count", 20, "Max concurrent reconciles count, default value is 20")
	flagSet.Float32Var(&QPS, "qps", 20, "qps of kubernetes client")

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

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

package webhook

import (
	"github.com/spf13/pflag"

	mutator "github.com/chaosblade-io/chaosblade-operator/pkg/webhook/pod"
)

var (
	Port   int
	Enable bool
)

var f *pflag.FlagSet

func init() {
	f = pflag.NewFlagSet("webhook", pflag.ExitOnError)
	f.StringVar(&mutator.SidecarImage, "fuse-sidecar-image", "", "Fuse sidecar image")
	f.Int32Var(&mutator.FuseServerPort, "fuse-server-port", 65534, "Fuse server port")

	f.IntVar(&Port, "webhook-port", 9443, "The port on which to serve HTTPS.")
	f.BoolVar(&Enable, "webhook-enable", false, "Whether to enable webhook")
}

func FlagSet() *pflag.FlagSet {
	return f
}

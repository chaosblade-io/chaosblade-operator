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

package model

import (
	"github.com/chaosblade-io/chaosblade-exec-os/exec/cpu"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/disk"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/file"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/mem"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/network"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/process"
	"github.com/chaosblade-io/chaosblade-exec-os/exec/script"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"
)

type OSSubResourceModelSpec struct {
	BaseSubResourceExpModelSpec
}

func NewOSSubResourceModelSpec() SubResourceExpModelSpec {
	modelSpec := &OSSubResourceModelSpec{
		BaseSubResourceExpModelSpec{
			ExpModelSpecs: []spec.ExpModelCommandSpec{
				cpu.NewCpuCommandModelSpec(),
				network.NewNetworkCommandSpec(),
				process.NewProcessCommandModelSpec(),
				disk.NewDiskCommandSpec(),
				mem.NewMemCommandModelSpec(),
				file.NewFileCommandSpec(),
				script.NewScriptCommandModelSpec(),
			},
		},
	}
	return modelSpec
}

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

package service

import (
	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
)

type ResourceModelSpec struct {
	model.BaseResourceExpModelSpec
}

func NewResourceModelSpec(client *channel.Client) model.ResourceExpModelSpec {
	modelSpec := &ResourceModelSpec{
		model.NewBaseResourceExpModelSpec("service", client),
	}
	expModels := []spec.ExpModelCommandSpec{
		NewSelfExpModelCommandSpec(client),
	}
	spec.AddFlagsToModelSpec(getResourceFlags, expModels...)
	modelSpec.RegisterExpModels(expModels...)
	return modelSpec
}

func getResourceFlags() []spec.ExpFlagSpec {
	return []spec.ExpFlagSpec{
		&spec.ExpFlag{
			Name:     "namespace",
			Desc:     "Namespace for the services, default is default",
			NoArgs:   false,
			Required: false,
		},
	}
}

type SelfExpModelCommandSpec struct {
	spec.BaseExpModelCommandSpec
}

func NewSelfExpModelCommandSpec(client *channel.Client) spec.ExpModelCommandSpec {
	return &SelfExpModelCommandSpec{
		spec.BaseExpModelCommandSpec{
			ExpFlags: []spec.ExpFlagSpec{},
			ExpActions: []spec.ExpActionCommandSpec{
				NewCreateServiceActionSpec(client),
				NewModifyServiceActionSpec(client),
			},
		},
	}
}

func (*SelfExpModelCommandSpec) Name() string {
	return "self"
}

func (*SelfExpModelCommandSpec) ShortDesc() string {
	return "Service experiments"
}

func (*SelfExpModelCommandSpec) LongDesc() string {
	return "Service experiments, such as creating services in batch or modifying service traffic policy"
}

func (*SelfExpModelCommandSpec) Example() string {
	return "blade create k8s service-self create --name-prefix my-service --namespace default --count 1000 --kubeconfig ~/.kube/config"
}

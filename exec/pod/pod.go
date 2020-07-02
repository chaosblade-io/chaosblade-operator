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

package pod

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
		model.NewBaseResourceExpModelSpec("pod", client),
	}
	osExpModels := NewOSSubResourceModelSpec(client).ExpModels()
	expModels := append(osExpModels, NewSelfExpModelCommandSpec(client))

	spec.AddFlagsToModelSpec(getResourceFlags, expModels...)
	modelSpec.RegisterExpModels(expModels...)
	return modelSpec
}

func getResourceFlags() []spec.ExpFlagSpec {
	coverageFlags := model.GetResourceCoverageFlags()
	commonFlags := model.GetResourceCommonFlags()
	return append(coverageFlags, commonFlags...)
}

type SelfExpModelCommandSpec struct {
	spec.BaseExpModelCommandSpec
}

func NewSelfExpModelCommandSpec(client *channel.Client) spec.ExpModelCommandSpec {
	return &SelfExpModelCommandSpec{
		spec.BaseExpModelCommandSpec{
			ExpFlags: []spec.ExpFlagSpec{},
			ExpActions: []spec.ExpActionCommandSpec{
				NewDeletePodActionSpec(client),
				NewPodIOActionSpec(client),
				NewFailPodActionSpec(client),
			},
		},
	}
}

func (*SelfExpModelCommandSpec) Name() string {
	return "pod"
}

func (*SelfExpModelCommandSpec) ShortDesc() string {
	return "Pod experiments"
}

func (*SelfExpModelCommandSpec) LongDesc() string {
	return "Pod experiments"
}

func (*SelfExpModelCommandSpec) Example() string {
	return "blade c k8s pod-pod delete --names redis-slave-674d68586-n5s4q --namespace default --kubeconfig ~/.kube/config"
}


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

package cluster

import (
	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
)

// ResourceModelSpec describes cluster-level chaos experiment models.
// Cluster-level experiments operate on cluster-wide infrastructure components,
// such as the cluster DNS server (CoreDNS / kube-dns).
type ResourceModelSpec struct {
	model.BaseResourceExpModelSpec
}

func NewResourceModelSpec(client *channel.Client) model.ResourceExpModelSpec {
	modelSpec := &ResourceModelSpec{
		model.NewBaseResourceExpModelSpec("cluster", client),
	}
	expModels := []spec.ExpModelCommandSpec{
		NewSelfExpModelCommandSpec(client),
	}
	modelSpec.RegisterExpModels(expModels...)
	return modelSpec
}

type SelfExpModelCommandSpec struct {
	spec.BaseExpModelCommandSpec
}

func NewSelfExpModelCommandSpec(client *channel.Client) spec.ExpModelCommandSpec {
	return &SelfExpModelCommandSpec{
		spec.BaseExpModelCommandSpec{
			ExpFlags: []spec.ExpFlagSpec{},
			ExpActions: []spec.ExpActionCommandSpec{
				NewClusterDnsFailureActionSpec(client),
			},
		},
	}
}

func (*SelfExpModelCommandSpec) Name() string {
	return "dns"
}

func (*SelfExpModelCommandSpec) ShortDesc() string {
	return "Cluster-level DNS server experiments"
}

func (*SelfExpModelCommandSpec) LongDesc() string {
	return "Cluster-level DNS server experiments. Inject faults into the cluster DNS " +
		"server (e.g. CoreDNS / kube-dns) workload to make cluster name resolution " +
		"completely unavailable. The experiment can be recovered by destroying the " +
		"ChaosBlade resource."
}

func (*SelfExpModelCommandSpec) Example() string {
	return "blade create k8s cluster-dns dnsfailure --dns-service kube-dns --dns-service-namespace kube-system --kubeconfig ~/.kube/config"
}

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

package model

import (
	"github.com/chaosblade-io/chaosblade-exec-cri/exec"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
)

// ResourceExpModelSpec contains node, pod, container
type ResourceExpModelSpec interface {
	Scope() string
	ExpModels() map[string]spec.ExpModelCommandSpec

	GetExpActionModelSpec(target, action string) spec.ExpActionCommandSpec
}

func NewBaseResourceExpModelSpec(scopeName string, client *channel.Client) BaseResourceExpModelSpec {
	return BaseResourceExpModelSpec{
		ScopeName:     scopeName,
		Client:        client,
		ExpModelSpecs: make(map[string]spec.ExpModelCommandSpec, 0),
	}
}

type BaseResourceExpModelSpec struct {
	ScopeName     string
	Client        *channel.Client
	ExpModelSpecs map[string]spec.ExpModelCommandSpec
}

func (b *BaseResourceExpModelSpec) Scope() string {
	return b.ScopeName
}

func (b *BaseResourceExpModelSpec) ExpModels() map[string]spec.ExpModelCommandSpec {
	return b.ExpModelSpecs
}

func (b *BaseResourceExpModelSpec) GetExpActionModelSpec(target, actionName string) spec.ExpActionCommandSpec {
	commandSpec := b.ExpModelSpecs[target]
	if commandSpec == nil {
		return nil
	}
	actions := commandSpec.Actions()
	if actions == nil {
		return nil
	}
	for _, action := range actions {
		if action.Name() == actionName {
			return action
		}
		for _, alias := range action.Aliases() {
			if alias == actionName {
				return action
			}
		}
	}
	return nil
}

func (b *BaseResourceExpModelSpec) RegisterExpModels(expModel ...spec.ExpModelCommandSpec) {
	for _, model := range expModel {
		b.ExpModelSpecs[model.Name()] = model
	}
}

// SubResourceExpModelSpec contains os exps in node, network exp in pod and os exps in container
type SubResourceExpModelSpec interface {
	ExpModels() []spec.ExpModelCommandSpec

	Executor() spec.Executor
}

type BaseSubResourceExpModelSpec struct {
	ExpModelSpecs []spec.ExpModelCommandSpec
	ExpExecutor   spec.Executor
}

func (b *BaseSubResourceExpModelSpec) ExpModels() []spec.ExpModelCommandSpec {
	return b.ExpModelSpecs
}

func (b *BaseSubResourceExpModelSpec) Executor() spec.Executor {
	return b.ExpExecutor
}

var ResourceCountFlag = &spec.ExpFlag{
	Name:     "evict-count",
	Desc:     "Count of affected resource",
	NoArgs:   false,
	Required: false,
}

var ResourcePercentFlag = &spec.ExpFlag{
	Name:     "evict-percent",
	Desc:     "Percent of affected resource, integer value without %",
	NoArgs:   false,
	Required: false,
}

func GetResourceCoverageFlags() []spec.ExpFlagSpec {
	return []spec.ExpFlagSpec{
		ResourceCountFlag,
		ResourcePercentFlag,
	}
}

var ResourceNamesFlag = &spec.ExpFlag{
	Name:     "names",
	Desc:     "Resource names, such as pod name. You must add namespace flag for it. Multiple parameters are separated directly by commas",
	NoArgs:   false,
	Required: false,
}

var ResourceNamespaceFlag = &spec.ExpFlag{
	Name:     "namespace",
	Desc:     "Namespace, such as default, only one value can be specified",
	NoArgs:   false,
	Required: true,
}

var ResourceLabelsFlag = &spec.ExpFlag{
	Name:     "labels",
	Desc:     "Label selector, the relationship between values that are or",
	NoArgs:   false,
	Required: false,
}

var ResourceGroupKeyFlag = &spec.ExpFlag{
	Name:     "evict-group",
	Desc:     "Group key from labels",
	NoArgs:   false,
	Required: false,
}

var ContainerIdsFlag = &spec.ExpFlag{
	Name:     "container-ids",
	Desc:     "Container ids",
	NoArgs:   false,
	Required: false,
}

var ContainerNamesFlag = &spec.ExpFlag{
	Name:     "container-names",
	Desc:     "Container names",
	NoArgs:   false,
	Required: false,
}

var ContainerIndexFlag = &spec.ExpFlag{
	Name: "container-index",
	Desc: "Container index, start from 0",
}

var ChaosBladePathFlag = &spec.ExpFlag{
	Name: "chaosblade-path",
	Desc: "Chaosblade tool deployment path, default value is /opt. Please select a path with write permission",
}

var ChaosBladeDownloadUrlFlag = &spec.ExpFlag{
	Name: "chaosblade-download-url",
	Desc: "The chaosblade downloaded address. If you use download deployment mode, you must specify the value, or config chaosblade-download-url when deploying the operator",
}

var (
	DownloadMode = "download"
	CopyMode     = "copy"
)

var ChaosBladeDeployModeFlag = &spec.ExpFlag{
	Name: "chaosblade-deploy-mode",
	Desc: "The mode of chaosblade deployment in container, the values are copy and download, the default value is copy which copy tool from the operator to the target container. If you select download mode, the operator will download chaosblade tool from the chaosblade-download-url.",
}

var IsDockerNetworkFlag = &spec.ExpFlag{
	Name:     "is-docker-network",
	Desc:     "Used when a docker container is used and there is no tc command in the target container. Just for docker command, Deprecated！ Please use use-sidecar-container-network flag.",
	NoArgs:   true,
	Required: false,
}

var UseSidecarContainerNetworkFlag = &spec.ExpFlag{
	Name:     "use-sidecar-container-network",
	Desc:     "When there is no tc command in the target container. Set the sidecar container network true.",
	NoArgs:   true,
	Required: false,
}

func GetNetworkFlags() []spec.ExpFlagSpec {
	return []spec.ExpFlagSpec{
		IsDockerNetworkFlag,
		UseSidecarContainerNetworkFlag,
	}
}

func GetContainerFlags() []spec.ExpFlagSpec {
	return []spec.ExpFlagSpec{
		ContainerIdsFlag,
		ContainerNamesFlag,
		ContainerIndexFlag,
	}
}

func GetResourceCommonFlags() []spec.ExpFlagSpec {
	return []spec.ExpFlagSpec{
		ResourceNamesFlag,
		ResourceNamespaceFlag,
		ResourceLabelsFlag,
		ResourceGroupKeyFlag,
	}
}

func GetChaosBladeFlags() []spec.ExpFlagSpec {
	return []spec.ExpFlagSpec{
		ChaosBladePathFlag,
		exec.ChaosBladeOverrideFlag,
		ChaosBladeDeployModeFlag,
		ChaosBladeDownloadUrlFlag,
	}
}

func GetResourceFlagNames() map[string]spec.Empty {
	flagNames := []string{
		ResourceCountFlag.Name,
		ResourcePercentFlag.Name,
		ResourceNamesFlag.Name,
		ResourceNamespaceFlag.Name,
		ResourceLabelsFlag.Name,
		ContainerIdsFlag.Name,
		ContainerNamesFlag.Name,
		ContainerIndexFlag.Name,
		ChaosBladePathFlag.Name,
		exec.ChaosBladeOverrideFlag.Name,
		ChaosBladeDeployModeFlag.Name,
		ChaosBladeDownloadUrlFlag.Name,
		IsDockerNetworkFlag.Name,
		UseSidecarContainerNetworkFlag.Name,
	}
	names := make(map[string]spec.Empty, 0)
	for _, name := range flagNames {
		names[name] = spec.Empty{}
	}
	return names
}

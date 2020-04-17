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

package container

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/chaosblade-io/chaosblade-exec-docker/exec"
	osexec "github.com/chaosblade-io/chaosblade-exec-os/exec"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/meta"
)

type DockerSubResourceModelSpec struct {
	model.BaseSubResourceExpModelSpec
}

// NewDockerSubResourceModelSpec the container model spec
func NewDockerSubResourceModelSpec(client *channel.Client) model.SubResourceExpModelSpec {
	modelCommandSpecs := make([]spec.ExpModelCommandSpec, 0)
	dockerExpModelSpecs := exec.NewDockerExpModelSpec().ExpModels()
	for _, expModelSpec := range dockerExpModelSpecs {
		modelCommandSpecs = append(modelCommandSpecs, expModelSpec)
	}

	modelSpec := &DockerSubResourceModelSpec{
		model.BaseSubResourceExpModelSpec{
			ExpModelSpecs: modelCommandSpecs,
			ExpExecutor:   NewDockerSubResourceExecutor(client),
		},
	}
	spec.AddExecutorToModelSpec(modelSpec.ExpExecutor, modelCommandSpecs...)
	return modelSpec
}

type DockerSubResourceExecutor struct {
	model.ExecCommandInPodExecutor
}

var CommandInPodExecutorFunc = func(ctx context.Context, expModel *spec.ExpModel,
	resourceIdentifier *model.ResourceIdentifier) ([]model.ExperimentIdentifier, error) {
	bladeBin := meta.Constant.BladeBin
	identifiers := make([]model.ExperimentIdentifier, 0)
	// container network does not need --blade-tar-file and --override flags
	excludeFlagsFunc := model.ExcludeKeyFunc()
	isNetworkTarget := expModel.Target == osexec.NewNetworkCommandSpec().Name()
	isContainerSelfTarget := expModel.Target == exec.NewContainerCommandSpec().Name()
	matchers := spec.ConvertExpMatchersToString(expModel, excludeFlagsFunc)
	if !isNetworkTarget && !isContainerSelfTarget {
		matchers = fmt.Sprintf("%s --blade-tar-file %s", matchers, meta.GetChaosBladePkgPath())
	}
	if isNetworkTarget {
		matchers = fmt.Sprintf("%s --image-repo %s --image-version %s",
			matchers, meta.Constant.ImageRepoFunc(), meta.GetChaosBladeVersion())
	}
	if _, ok := spec.IsDestroy(ctx); ok {
		return generateDestroyCommands(ctx, expModel, resourceIdentifier, matchers, bladeBin, identifiers)
	}
	return generateCreateCommands(ctx, expModel, resourceIdentifier, matchers, bladeBin, identifiers)
}

// NewDockerSubResourceExecutor returns the container executor
func NewDockerSubResourceExecutor(client *channel.Client) spec.Executor {
	return &DockerSubResourceExecutor{
		model.ExecCommandInPodExecutor{
			Client:      client,
			CommandFunc: CommandInPodExecutorFunc,
		},
	}
}

func generateDestroyCommands(ctx context.Context, expModel *spec.ExpModel, resourceIdentifier *model.ResourceIdentifier, matchers string, bladeBin string, identifiers []model.ExperimentIdentifier) ([]model.ExperimentIdentifier, error) {
	nodeNameExpObjectMetasMaps, err := model.ExtractNodeNameExpObjectMetasMapFromContext(ctx)
	if err != nil {
		return identifiers, err
	}
	logrus.Infof("nodeNameExpObjectMetasMaps: %+v", nodeNameExpObjectMetasMaps)
	expObjectMetas := nodeNameExpObjectMetasMaps[resourceIdentifier.NodeName]
	if expObjectMetas == nil {
		return identifiers, fmt.Errorf("cannot find the matched container on the node: %s", resourceIdentifier.NodeName)
	}
	for _, expObjectMeta := range expObjectMetas {
		flags := fmt.Sprintf("%s --container-id %s", matchers, expObjectMeta.Uid)
		command := fmt.Sprintf("%s destroy docker %s %s %s", bladeBin, expModel.Target, expModel.ActionName, flags)
		if expObjectMeta.Id != "" {
			command = fmt.Sprintf("%s --uid %s", command, expObjectMeta.Id)
		}
		identifier := model.NewExperimentIdentifier(expObjectMeta.Id, expObjectMeta.Uid, expObjectMeta.Name, command)
		identifiers = append(identifiers, identifier)
	}
	return identifiers, nil
}

func generateCreateCommands(ctx context.Context, expModel *spec.ExpModel, resourceIdentifier *model.ResourceIdentifier,
	matchers string, bladeBin string, identifiers []model.ExperimentIdentifier) ([]model.ExperimentIdentifier, error) {
	nodeNameContainerObjectMetasMaps, err := model.ExtractNodeNameContainerMetasMapFromContext(ctx)
	if err != nil {
		return identifiers, err
	}
	containerObjectMetas := nodeNameContainerObjectMetasMaps[resourceIdentifier.NodeName]
	if containerObjectMetas == nil {
		return identifiers, fmt.Errorf("cannot find the matched container on the node: %s", resourceIdentifier.NodeName)
	}
	for _, objectMeta := range containerObjectMetas {
		flags := fmt.Sprintf("%s --container-id %s", matchers, objectMeta.Uid)
		command := fmt.Sprintf("%s create docker %s %s %s", bladeBin, expModel.Target, expModel.ActionName, flags)
		identifier := model.NewExperimentIdentifier("", objectMeta.Uid, objectMeta.Name, command)
		identifiers = append(identifiers, identifier)
	}
	return identifiers, nil
}

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
	"context"
	"fmt"
	"strings"

	"github.com/chaosblade-io/chaosblade-exec-os/exec"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/meta"
)

type OSSubResourceModelSpec struct {
	model.BaseSubResourceExpModelSpec
}

func NewOSSubResourceModelSpec(client *channel.Client) model.SubResourceExpModelSpec {
	modelSpec := &OSSubResourceModelSpec{
		model.BaseSubResourceExpModelSpec{
			ExpModelSpecs: []spec.ExpModelCommandSpec{
				exec.NewNetworkCommandSpec(),
			},
			ExpExecutor: NewOSSubResourceExecutor(client),
		},
	}
	spec.AddExecutorToModelSpec(modelSpec.ExpExecutor, modelSpec.ExpModelSpecs...)
	return modelSpec
}

type OSSubResourceExecutor struct {
	model.ExecCommandInPodExecutor
}

func NewOSSubResourceExecutor(client *channel.Client) spec.Executor {
	return &OSSubResourceExecutor{
		model.ExecCommandInPodExecutor{
			Client: client,
			CommandFunc: func(ctx context.Context, expModel *spec.ExpModel,
				resourceIdentifier *model.ResourceIdentifier) ([]model.ExperimentIdentifier, error) {
				bladeBin := meta.Constant.BladeBin
				identifiers := make([]model.ExperimentIdentifier, 0)

				if expIdValues, ok := spec.IsDestroy(ctx); ok {
					expIds := strings.Split(expIdValues, ",")
					for _, expId := range expIds {
						command := fmt.Sprintf("%s destroy %s", bladeBin, expId)
						identifier := model.NewExperimentIdentifier(expId, resourceIdentifier.PodUid,
							resourceIdentifier.PodName, command)
						identifiers = append(identifiers, identifier)
					}
					return identifiers, nil
				}

				matchers := spec.ConvertExpMatchersToString(expModel, model.ExcludeKeyFunc())
				containerId := resourceIdentifier.ContainerId
				if containerId == "" {
					return identifiers, fmt.Errorf("cannot find a valiable container in the %s pod", resourceIdentifier.PodName)
				}
				flags := fmt.Sprintf("%s --container-id %s", matchers, containerId)
				command := fmt.Sprintf("%s create docker %s %s %s", bladeBin, expModel.Target, expModel.ActionName, flags)
				identifier := model.NewExperimentIdentifier("", resourceIdentifier.PodUid, resourceIdentifier.PodName, command)
				identifiers = append(identifiers, identifier)
				return identifiers, nil
			},
		},
	}
}

func (*OSSubResourceExecutor) Name() string {
	return "osExecutorForNode"
}

func (*OSSubResourceExecutor) SetChannel(channel spec.Channel) {
}

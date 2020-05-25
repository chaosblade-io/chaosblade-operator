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

package node

import (
	"context"
	"fmt"

	"github.com/chaosblade-io/chaosblade-exec-os/exec"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
)

type OSSubResourceModelSpec struct {
	model.BaseSubResourceExpModelSpec
}

func NewOSSubResourceModelSpec(client *channel.Client) model.SubResourceExpModelSpec {
	modelSpec := &OSSubResourceModelSpec{
		model.BaseSubResourceExpModelSpec{
			ExpModelSpecs: []spec.ExpModelCommandSpec{
				exec.NewCpuCommandModelSpec(),
				exec.NewNetworkCommandSpec(),
				exec.NewProcessCommandModelSpec(),
				exec.NewDiskCommandSpec(),
				exec.NewMemCommandModelSpec(),
			},
			ExpExecutor: NewOSSubResourceExecutor(client),
		},
	}
	spec.AddExecutorToModelSpec(modelSpec.ExpExecutor, modelSpec.ExpModelSpecs...)
	return modelSpec
}

func NewOSSubResourceExecutor(client *channel.Client) spec.Executor {
	return &model.ExecCommandInPodExecutor{
		Client: client,
		CommandFunc: func(ctx context.Context, expModel *spec.ExpModel,
			resourceIdentifier *model.ResourceIdentifier) ([]model.ExperimentIdentifier, error) {
			bladeBin := chaosblade.Constant.BladeBin
			identifier := model.NewExperimentIdentifier("", resourceIdentifier.NodeUid, resourceIdentifier.NodeName, "")

			if suid, ok := spec.IsDestroy(ctx); ok {
				identifier.Command = fmt.Sprintf("%s destroy %s", bladeBin, suid)
				identifier.Id = suid
				return []model.ExperimentIdentifier{identifier}, nil
			}
			matchers := spec.ConvertExpMatchersToString(expModel, model.ExcludeKeyFunc())
			identifier.Command = fmt.Sprintf("%s create %s %s %s", bladeBin, expModel.Target, expModel.ActionName, matchers)
			return []model.ExperimentIdentifier{identifier}, nil
		},
	}
}

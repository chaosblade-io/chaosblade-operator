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

package pod

import (
	"context"
	"fmt"

	"github.com/chaosblade-io/chaosblade-exec-os/exec"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

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
				exec.NewDiskCommandSpec(),
				exec.NewMemCommandModelSpec(),
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
				// Destroy
				if _, ok := spec.IsDestroy(ctx); ok {
					expObjectMetasMaps, err := model.ExtractNodeNameExpObjectMetasMapFromContext(ctx)
					if err != nil {
						return identifiers, err
					}
					expObjectMetas := expObjectMetasMaps[resourceIdentifier.NodeName]
					for _, expObjectMeta := range expObjectMetas {
						if expObjectMeta.Id == "" {
							continue
						}
						command := fmt.Sprintf("%s destroy %s", bladeBin, expObjectMeta.Id)
						identifier := model.NewExperimentIdentifier(expObjectMeta.Id, expObjectMeta.Uid,
							expObjectMeta.Name, command)
						identifiers = append(identifiers, identifier)
					}
					return identifiers, nil
				}
				// Create
				matchers := spec.ConvertExpMatchersToString(expModel, model.ExcludeKeyFunc())
				// Get pods from context
				podObjectMetaList, err := model.ExtractPodObjectMetasFromContext(ctx)
				if err != nil {
					return identifiers, err
				}
				// Traverse the pod list to get the container running in every pod
				for _, podObjectMeta := range podObjectMetaList {
					if podObjectMeta.NodeName != resourceIdentifier.NodeName {
						continue
					}
					pod := v1.Pod{}
					err := client.Get(context.Background(), types.NamespacedName{
						Name:      podObjectMeta.Name,
						Namespace: podObjectMeta.Namespace}, &pod)
					if err != nil {
						identifier := model.NewExperimentIdentifierWithError("", podObjectMeta.Uid, podObjectMeta.Name, err.Error())
						identifiers = append(identifiers, identifier)
						continue
					}
					containerId, err := model.GetOneAvailableContainerIdFromPod(pod)
					if err != nil {
						identifier := model.NewExperimentIdentifierWithError("", podObjectMeta.Uid, podObjectMeta.Name, err.Error())
						identifiers = append(identifiers, identifier)
						continue
					}
					flags := fmt.Sprintf("%s --container-id %s --image-repo %s --image-version %s",
						matchers, containerId, meta.Constant.ImageRepoFunc(), meta.GetChaosBladeVersion())
					command := fmt.Sprintf("%s create docker %s %s %s", bladeBin, expModel.Target, expModel.ActionName, flags)
					identifier := model.NewExperimentIdentifier("", podObjectMeta.Uid, podObjectMeta.Name, command)
					identifiers = append(identifiers, identifier)
				}
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

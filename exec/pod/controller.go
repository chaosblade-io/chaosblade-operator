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

	"k8s.io/api/core/v1"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

type ExpController struct {
	model.BaseExperimentController
}

func NewExpController(client *channel.Client) model.ExperimentController {
	return &ExpController{
		model.BaseExperimentController{
			Client:            client,
			ResourceModelSpec: NewResourceModelSpec(client),
		},
	}
}

func (*ExpController) Name() string {
	return "pod"
}

func (e *ExpController) Create(ctx context.Context, expSpec v1alpha1.ExperimentSpec) *spec.Response {
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	pods, err := e.GetMatchedPodResources(*expModel)
	if err != nil {
		return spec.ReturnFailWitResult(spec.Code[spec.IgnoreCode], err.Error(),
			v1alpha1.CreateFailExperimentStatus(err.Error(), nil))
	}
	if len(pods) == 0 {
		return spec.ReturnFailWitResult(spec.Code[spec.IgnoreCode], err.Error(),
			v1alpha1.CreateFailExperimentStatus("cannot find the pods", nil))
	}
	ctx = setNecessaryObjectsToContext(ctx, pods)
	return e.Exec(ctx, expModel)
}

func setNecessaryObjectsToContext(ctx context.Context, pods []v1.Pod) context.Context {
	podObjectMetas := make([]model.PodObjectMeta, 0)
	nodeNameUidMap := model.NodeNameUidMap{}
	for _, pod := range pods {
		podObjectMeta := model.PodObjectMeta{
			Name: pod.Name, Namespace: pod.Namespace, Uid: string(pod.UID), NodeName: pod.Spec.NodeName,
		}
		podObjectMetas = append(podObjectMetas, podObjectMeta)
		// node uid is unuseful for pod experiments
		nodeNameUidMap[pod.Spec.NodeName] = ""
	}
	ctx = context.WithValue(ctx, model.PodObjectMetaKey, podObjectMetas)
	ctx = context.WithValue(ctx, model.NodeNameUidMapKey, nodeNameUidMap)
	return ctx
}

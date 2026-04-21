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

package pod

import (
	"context"

	"github.com/chaosblade-io/chaosblade-exec-cri/exec/container"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

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

// Create pod resource experiments
func (e *ExpController) Create(ctx context.Context, expSpec v1alpha1.ExperimentSpec) *spec.Response {
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	// Get action spec to check if it implements ActionPreProcessor
	actionSpec := e.ResourceModelSpec.GetExpActionModelSpec(expModel.Target, expModel.ActionName)
	if actionSpec != nil {
		if preProcessor, ok := actionSpec.(model.ActionPreProcessor); ok {
			newCtx, resp := preProcessor.PreCreate(ctx, expModel, e.Client)
			if resp != nil {
				return resp
			}
			ctx = newCtx
			logrusField.Infof("creating %s experiment with pre-processing", expModel.ActionName)
			return e.Exec(ctx, expModel)
		}
	}

	// Default flow: find matched pods and execute
	pods, resp := e.GetMatchedPodResources(ctx, *expModel)
	if !resp.Success {
		logrusField.Errorf("uid: %s, get matched pod experiment failed, %v", experimentId, resp.Err)
		resp.Result = v1alpha1.CreateFailExperimentStatus(resp.Err, []v1alpha1.ResourceStatus{})
	}
	logrusField.Infof("creating pod experiment, pod count is %d", len(pods))
	containerObjectMetaList := getContainerMatchedList(experimentId, pods)
	if len(containerObjectMetaList) == 0 {
		logrusField.Errorf("uid: %s, get container from context failed", experimentId)
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return e.Exec(ctx, expModel)
}

func (e *ExpController) Destroy(ctx context.Context, expSpec v1alpha1.ExperimentSpec, oldExpStatus v1alpha1.ExperimentStatus) *spec.Response {
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrus.WithField("experiment", experimentId).Infoln("start to destroy")

	// Check if action implements ActionPreProcessor - use the same path as Create
	actionSpec := e.ResourceModelSpec.GetExpActionModelSpec(expModel.Target, expModel.ActionName)
	if actionSpec != nil {
		if preProcessor, ok := actionSpec.(model.ActionPreProcessor); ok {
			newCtx, resp := preProcessor.PreDestroy(ctx, expModel, e.Client, oldExpStatus)
			if resp != nil {
				return resp
			}
			ctx = newCtx
			return e.Exec(ctx, expModel)
		}
	}

	// Default flow: find matched containers and destroy
	statuses := oldExpStatus.ResStatuses
	if len(statuses) == 0 {
		return spec.ReturnSuccess(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{}))
	}
	containerObjectMetaList := model.ContainerMatchedList{}
	for _, status := range statuses {
		if !status.Success {
			continue
		}
		containerObjectMeta := model.ParseIdentifier(status.Identifier)
		containerObjectMeta.Id = status.Id
		containerObjectMetaList = append(containerObjectMetaList, containerObjectMeta)
	}
	if len(containerObjectMetaList) == 0 {
		return spec.ReturnSuccess(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{}))
	}
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return e.Exec(ctx, expModel)
}

// getContainerMatchedList transports selected pods
func getContainerMatchedList(experimentId string, pods []v1.Pod) model.ContainerMatchedList {
	containerObjectMetaList := model.ContainerMatchedList{}
	for _, p := range pods {
		containerId, containerName, runtime, err := model.GetOneAvailableContainerIdFromPod(p)
		if runtime == container.DockerRuntime {
			containerId = containerId[:12]
		}

		if err != nil {
			logrus.WithField("experiment", experimentId).WithField("pod", p.Name).
				Errorf("get an available container failed, %v", err)
			continue
		}
		containerObjectMetaList = append(containerObjectMetaList, model.ContainerObjectMeta{
			ContainerId:      containerId,
			ContainerName:    containerName,
			ContainerRuntime: runtime,
			PodName:          p.Name,
			NodeName:         p.Spec.NodeName,
			Namespace:        p.Namespace,
		})
	}
	return containerObjectMetaList
}

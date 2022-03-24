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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/chaosblade-io/chaosblade-exec-cri/exec/container"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
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
	return "container"
}

// Create an experiment about container
func (e *ExpController) Create(ctx context.Context, expSpec v1alpha1.ExperimentSpec) *spec.Response {
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	// priority: id > name > index
	containerIdsValue := strings.TrimSpace(expModel.ActionFlags[model.ContainerIdsFlag.Name])
	containerNamesValue := strings.TrimSpace(expModel.ActionFlags[model.ContainerNamesFlag.Name])
	containerIndexValue := strings.TrimSpace(expModel.ActionFlags[model.ContainerIndexFlag.Name])
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId).WithField("location", util.GetRunFuncName())
	lessParameter := fmt.Sprintf("%s|%s|%s", model.ContainerIdsFlag.Name, model.ContainerNamesFlag.Name, model.ContainerIndexFlag.Name)
	if containerIdsValue == "" && containerNamesValue == "" && containerIndexValue == "" {
		errMsg := spec.ParameterLess.Sprintf(lessParameter)
		logrusField.Errorln(errMsg)
		return spec.ResponseFailWithResult(spec.ParameterLess, v1alpha1.CreateFailExperimentStatus(errMsg, nil), lessParameter)
	}
	pods, resp := e.GetMatchedPodResources(ctx, *expModel)
	if !resp.Success {
		logrusField.Errorf("uid: %s, get matched pod resources failed, %v", experimentId, resp.Err)
		resp.Result = v1alpha1.CreateFailExperimentStatus(resp.Err, []v1alpha1.ResourceStatus{})
		return resp
	}
	containerObjectMetaList, resp := getMatchedContainerMetaList(pods, containerIdsValue, containerNamesValue, containerIndexValue)
	if !resp.Success {
		logrusField.Errorf("get matched container meta list failed, %v", resp.Err)
		resp.Result = v1alpha1.CreateFailExperimentStatus(resp.Err, []v1alpha1.ResourceStatus{})
		return resp
	}
	if len(containerObjectMetaList) == 0 {
		// TODO need to optimize
		errMsg := spec.ParameterInvalid.Sprintf(
			strings.Join([]string{model.ContainerIdsFlag.Name, model.ContainerNamesFlag.Name, model.ContainerIndexFlag.Name}, "|"),
			strings.Join([]string{containerIdsValue, containerNamesValue, containerIndexValue}, "|"),
			"cannot find the containers")
		logrusField.Errorln(errMsg)
		response := spec.ResponseFailWithResult(
			spec.ParameterInvalid,
			v1alpha1.CreateFailExperimentStatus(errMsg, []v1alpha1.ResourceStatus{}),
			strings.Join([]string{model.ContainerIdsFlag.Name, model.ContainerNamesFlag.Name, model.ContainerIndexFlag.Name}, "|"),
			strings.Join([]string{containerIdsValue, containerNamesValue, containerIndexValue}, "|"),
			"cannot find the containers")
		return response
	}
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return e.Exec(ctx, expModel)
}

// Destroy
func (e *ExpController) Destroy(ctx context.Context, expSpec v1alpha1.ExperimentSpec, oldExpStatus v1alpha1.ExperimentStatus) *spec.Response {
	logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx)).WithField("location", util.GetRunFuncName()).Infof("start to destroy")
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	statuses := oldExpStatus.ResStatuses
	if statuses == nil {
		return spec.ReturnSuccess(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{}))
	}
	containerObjectMetaList := model.ContainerMatchedList{}
	for _, status := range statuses {
		if !status.Success {
			// does not need to destroy
			continue
		}
		containerObjectMeta := model.ParseIdentifier(status.Identifier)
		containerObjectMeta.Id = status.Id
		containerObjectMetaList = append(containerObjectMetaList, containerObjectMeta)
	}
	if len(containerObjectMetaList) == 0 {
		return spec.ReturnSuccess(v1alpha1.CreateSuccessExperimentStatus(statuses))
	}
	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return e.Exec(ctx, expModel)
}

// getMatchedContainerMetaList which will be used in the executor
func getMatchedContainerMetaList(pods []v1.Pod, containerIdsValue, containerNamesValue, containerIndexValue string) (
	model.ContainerMatchedList, *spec.Response) {
	containerObjectMetaList := model.ContainerMatchedList{}
	expectedContainerIds := strings.Split(containerIdsValue, ",")
	expectedContainerNames := strings.Split(containerNamesValue, ",")
	// priority id>name>index
	for _, pod := range pods {
		containerStatuses := pod.Status.ContainerStatuses
		if containerStatuses == nil {
			continue
		}
		var containerStatusErr error
		for _, containerStatus := range containerStatuses {
			// If target container's status is not running, the containerId can not be obtained
			// The container's status should be checked and return err if not running
			var containerRuntime, containerId string
			containerName := containerStatus.Name
			if containerStatus.ContainerID == "" {
				containerStatusErr = errors.New("containerId is empty")
			} else {
				containerRuntime,containerId = model.TruncateContainerObjectMetaUid(containerStatus.ContainerID)
				if containerRuntime == container.DockerRuntime {
					containerId = containerId[:12]
				}
			}
			if containerStatus.State.Running == nil {
				if containerStatusErr != nil {
					containerStatusErr = errors.New("is not running, " + containerStatusErr.Error())
				} else {
					containerStatusErr = errors.New("is not running, containerId: " + containerStatus.ContainerID)
				}
			}
			if containerIdsValue != "" {
				for _, expectedContainerId := range expectedContainerIds {
					if expectedContainerId == "" {
						continue
					}
					if strings.HasPrefix(containerId, expectedContainerId) {
						if containerStatusErr != nil {
							return containerObjectMetaList, spec.ResponseFailWithFlags(spec.ParameterInvalid,
								model.ContainerIdsFlag.Name, expectedContainerId,
								fmt.Sprintf("container: %s %s", containerName, containerStatusErr.Error()))
						}
						containerObjectMetaList = append(containerObjectMetaList, model.ContainerObjectMeta{
							ContainerRuntime: containerRuntime,
							ContainerId:      containerId,
							ContainerName:    containerName,
							PodName:          pod.Name,
							Namespace:        pod.Namespace,
							NodeName:         pod.Spec.NodeName,
						})
					}
				}
			} else if containerNamesValue != "" {
				for _, expectedName := range expectedContainerNames {
					if expectedName == "" {
						continue
					}
					if expectedName == containerName {
						// matched
						if containerStatusErr != nil {
							return containerObjectMetaList, spec.ResponseFailWithFlags(spec.ParameterInvalid,
								model.ContainerNamesFlag.Name, expectedName,
								fmt.Sprintf("container: %s %s", containerName, containerStatusErr.Error()))
						}
						containerObjectMetaList = append(containerObjectMetaList, model.ContainerObjectMeta{
							ContainerRuntime: containerRuntime,
							ContainerId:      containerId,
							ContainerName:    containerName,
							PodName:          pod.Name,
							Namespace:        pod.Namespace,
							NodeName:         pod.Spec.NodeName,
						})
					}
				}
			}
		}
		if containerIdsValue == "" && containerNamesValue == "" && containerIndexValue != "" {
			idx, err := strconv.Atoi(containerIndexValue)
			if err != nil {
				return containerObjectMetaList,
					spec.ResponseFailWithFlags(spec.ParameterIllegal, model.ContainerIndexFlag.Name, containerIndexValue, err)
			}
			if idx > len(containerStatuses)-1 {
				return containerObjectMetaList,
					spec.ResponseFailWithFlags(spec.ParameterIllegal, model.ContainerIndexFlag.Name, containerIndexValue, "out of bound")
			}
			// If target container's status is not running, the containerId can not be obtained
			// The container's status should be checked and return err if not running
			if containerStatuses[idx].ContainerID == "" {
				containerStatusErr = errors.New("containerId is empty")
			}
			if containerStatuses[idx].State.Running == nil {
				if containerStatusErr != nil {
					containerStatusErr = errors.New("is not running, " + containerStatusErr.Error())
				} else {
					containerStatusErr = errors.New("is not running, containerId: " + containerStatuses[idx].ContainerID)
				}
			}
			if containerStatusErr != nil {
				return containerObjectMetaList, spec.ResponseFailWithFlags(spec.ParameterInvalid,
					model.ContainerIndexFlag.Name, idx,
					fmt.Sprintf("container: %s %s", containerStatuses[idx].Name, containerStatusErr.Error()))
			}
			containerRuntime, containerId := model.TruncateContainerObjectMetaUid(containerStatuses[idx].ContainerID)
			if containerRuntime == container.DockerRuntime {
				containerId = containerId[:12]
			}
			containerObjectMetaList = append(containerObjectMetaList, model.ContainerObjectMeta{
				ContainerRuntime: containerRuntime,
				ContainerId:      containerId,
				ContainerName:    containerStatuses[idx].Name,
				PodName:          pod.Name,
				Namespace:        pod.Namespace,
				NodeName:         pod.Spec.NodeName,
			})
		}
	}
	return containerObjectMetaList, spec.Success()
}

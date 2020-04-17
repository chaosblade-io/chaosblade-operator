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
	"strings"

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
	containerIdsValue := expModel.ActionFlags[model.ContainerIdsFlag.Name]
	containerNamesValue := expModel.ActionFlags[model.ContainerNamesFlag.Name]

	if containerIdsValue == "" && containerNamesValue == "" {
		errMsg := fmt.Sprintf("less %s or %s flag", model.ContainerIdsFlag.Name, model.ContainerNamesFlag.Name)
		return spec.ReturnFailWitResult(spec.Code[spec.IllegalParameters], errMsg,
			v1alpha1.CreateFailExperimentStatus(errMsg, nil))
	}
	if containerIdsValue != "" && containerNamesValue != "" {
		errMsg := fmt.Sprintf("only one choice between %s and %s", model.ContainerIdsFlag.Name, model.ContainerNamesFlag.Name)
		return spec.ReturnFailWitResult(spec.Code[spec.IllegalParameters], errMsg,
			v1alpha1.CreateFailExperimentStatus(errMsg, nil))
	}
	pods, err := e.GetMatchedPodResources(*expModel)
	if err != nil {
		return spec.ReturnFailWitResult(spec.Code[spec.IgnoreCode], err.Error(),
			v1alpha1.CreateFailExperimentStatus(err.Error(), nil))
	}
	if len(pods) == 0 {
		return spec.ReturnFailWitResult(spec.Code[spec.IgnoreCode], err.Error(),
			v1alpha1.CreateFailExperimentStatus("cannot find the target pods for container resource", nil))
	}
	ctx = setNecessaryObjectsToContext(ctx, pods, containerIdsValue, containerNamesValue)
	return e.Exec(ctx, expModel)
}

// setNecessaryObjectsToContext which will be used in the executor
func setNecessaryObjectsToContext(ctx context.Context, pods []v1.Pod,
	containerIdsValue, containerNamesValue string) context.Context {
	nodeNameContainerObjectMetasMaps := model.NodeNameContainerObjectMetasMap{}
	nodeNameUidMap := model.NodeNameUidMap{}
	expectedContainerIds := strings.Split(containerIdsValue, ",")
	expectedContainerNames := strings.Split(containerNamesValue, ",")
	for _, pod := range pods {
		containerStatuses := pod.Status.ContainerStatuses
		if containerStatuses != nil {
			for _, containerStatus := range containerStatuses {
				containerId := model.TruncateContainerObjectMetaUid(containerStatus.ContainerID)
				containerName := containerStatus.Name
				if len(containerIdsValue) > 0 {
					for _, expectedContainerId := range expectedContainerIds {
						if expectedContainerId == "" {
							continue
						}
						if strings.HasPrefix(containerId, expectedContainerId) {
							// matched
							nodeNameUidMap, nodeNameContainerObjectMetasMaps =
								AddMatchedContainerAndNode(pod, containerId, containerName,
									nodeNameContainerObjectMetasMaps, nodeNameUidMap)
						}
					}
				}
				if len(containerNamesValue) > 0 {
					for _, expectedName := range expectedContainerNames {
						if expectedName == "" {
							continue
						}
						if expectedName == containerName {
							// matched
							nodeNameUidMap, nodeNameContainerObjectMetasMaps =
								AddMatchedContainerAndNode(pod, containerId, containerName,
									nodeNameContainerObjectMetasMaps, nodeNameUidMap)
						}
					}
				}
			}
		}
	}
	ctx = context.WithValue(ctx, model.NodeNameUidMapKey, nodeNameUidMap)
	ctx = context.WithValue(ctx, model.NodeNameContainerObjectMetasMapKey, nodeNameContainerObjectMetasMaps)
	return ctx
}

func AddMatchedContainerAndNode(pod v1.Pod, containerId, containerName string, nodeNameContainerObjectMetasMaps model.NodeNameContainerObjectMetasMap,
	nodeNameUidMap model.NodeNameUidMap) (model.NodeNameUidMap, model.NodeNameContainerObjectMetasMap) {
	nodeName := pod.Spec.NodeName
	logrus.Infof("matched container: %s, pod: %s, node: %s", containerId, pod.Name, nodeName)
	nameUidMap := AddMatchedNode(nodeName, nodeNameUidMap)
	nodeNameContainerObjectMetasMap := AddMatchedContainer(pod, containerId, containerName, nodeName, nodeNameContainerObjectMetasMaps)
	return nameUidMap, nodeNameContainerObjectMetasMap
}

// AddMatchedContainer to context
func AddMatchedContainer(pod v1.Pod, containerId, containerName, nodeName string,
	nodeNameContainerObjectMetasMaps model.NodeNameContainerObjectMetasMap) model.NodeNameContainerObjectMetasMap {
	containerObjectMeta := model.ContainerObjectMeta{
		Name:     containerName,
		Uid:      containerId,
		PodName:  pod.Name,
		PodUid:   string(pod.UID),
		NodeName: nodeName,
	}
	containerObjectMetas := nodeNameContainerObjectMetasMaps[nodeName]
	if containerObjectMetas == nil {
		containerObjectMetas = make([]model.ContainerObjectMeta, 0)
	}
	containerObjectMetas = append(containerObjectMetas, containerObjectMeta)
	nodeNameContainerObjectMetasMaps[nodeName] = containerObjectMetas
	return nodeNameContainerObjectMetasMaps
}

// AddMatchedNode to context
func AddMatchedNode(nodeName string, nodeNameUidMap model.NodeNameUidMap) model.NodeNameUidMap {
	// node uid is unuseful for pod experiments
	nodeNameUidMap[nodeName] = ""
	return nodeNameUidMap
}

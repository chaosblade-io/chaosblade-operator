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
	"context"
	"fmt"
	"github.com/chaosblade-io/chaosblade-exec-cri/exec/container"
	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
	"github.com/chaosblade-io/chaosblade-operator/version"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"strings"
	"sync"
)

type CommonExecutor struct {
	Client *channel.Client
}

func (e *CommonExecutor) Name() string {
	return "CommonExecutor"
}

func (e *CommonExecutor) SetChannel(channel spec.Channel) {
}

func (e *CommonExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	logrusField := logrus.WithField("experiment", GetExperimentIdFromContext(ctx))
	experimentStatus := v1alpha1.ExperimentStatus{
		ResStatuses: make([]v1alpha1.ResourceStatus, 0),
	}
	experimentIdentifiers, err := getExperimentIdentifiersWithNsexec(ctx, expModel, e.Client)
	if err != nil {
		logrusField.Errorf("get experiment identifiers failed, err: %s", err.Error())
		return spec.ResponseFailWithResult(spec.GetIdentifierFailed,
			v1alpha1.CreateFailExperimentStatus(err.Error(), []v1alpha1.ResourceStatus{}),
			err)
	}
	logrusField.Infof("experiment identifiers: %v", experimentIdentifiers)

	statuses := experimentStatus.ResStatuses
	success := true
	_, isDestroy := spec.IsDestroy(ctx)
	updateResultLock := &sync.Mutex{}

	execCommandInPod := func(i int) {
		execSuccess := true
		identifier := experimentIdentifiers[i]

		rsStatus := v1alpha1.ResourceStatus{
			Kind:       expModel.Scope,
			Identifier: identifier.GetIdentifier(),
			Id:         identifier.Id,
		}

		if identifier.Error != "" {
			rsStatus.CreateFailResourceStatus(identifier.Error, spec.K8sExecFailed.Code)
			execSuccess = false
		} else if identifier.PodName != "" {
			// check if pod exist
			pod := &v1.Pod{}
			err := e.Client.Get(context.TODO(), types.NamespacedName{Namespace: identifier.Namespace,
				Name: identifier.PodName}, pod)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// pod if not exist, the execution is considered successful.
					msg := fmt.Sprintf("pod: %s in %s not found, skip to execute command in it",
						identifier.PodName, identifier.Namespace)
					logrusField.Warningln(msg)
					rsStatus.CreateSuccessResourceStatus()
					rsStatus.Error = msg
					success = true
				} else {
					// if get pod error, the execution is considered failure
					msg := fmt.Sprintf("get pod: %s in %s error",
						identifier.PodName, identifier.Namespace)
					rsStatus.CreateFailResourceStatus(msg, spec.K8sExecFailed.Code)
					execSuccess = false
				}
			}
		}
		if execSuccess {
			logrusField.Infof("execute identifier: %+v", identifier)
			execSuccess, rsStatus = execCommands(isDestroy, rsStatus, identifier, e.Client)
		}
		updateResultLock.Lock()
		statuses = append(statuses, rsStatus)
		// If false occurs once, the result is fails
		success = success && execSuccess
		updateResultLock.Unlock()
	}

	ParallelizeExec(len(experimentIdentifiers), execCommandInPod)

	logrusField.Infof("success: %t, statuses: %+v", success, statuses)
	if success {
		experimentStatus.State = v1alpha1.SuccessState
	} else {
		experimentStatus.State = v1alpha1.ErrorState
		if len(statuses) == 0 {
			experimentStatus.Error = "the resources not found"
		} else {
			experimentStatus.Error = "see resStatus for the error details"
		}
	}
	experimentStatus.Success = success
	experimentStatus.ResStatuses = append(experimentStatus.ResStatuses, statuses...)

	checkExperimentStatus(ctx, expModel, statuses, experimentIdentifiers, e.Client)
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

func getExperimentIdentifiersWithNsexec(ctx context.Context, expModel *spec.ExpModel, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	delete(expModel.ActionFlags, "uid")
	containerObjectMetaList, err := GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		return []ExperimentIdentifierInPod{}, err
	}
	excludeFlagsFunc := ExcludeKeyFunc()
	matchers := spec.ConvertExpMatchersToString(expModel, excludeFlagsFunc)
	experimentId := GetExperimentIdFromContext(ctx)
	_, destroy := spec.IsDestroy(ctx)

	isNodeScope := expModel.Scope == "node"
	if isNodeScope {
		return getNodeExperimentIdentifiers(experimentId, expModel, containerObjectMetaList, matchers, destroy, client)
	}

	var scope string
	if version.CheckVerisonHaveCriCommand() || containerObjectMetaList[0].ContainerRuntime == container.ContainerdRuntime {
		// blade create cri --container-id containerObjectMetaList[0].ContainerId --container-runtime obj.ContainerRuntime
		scope = "cri"
	} else {
		// blade create docker --container-id containerObjectMetaList[0].ContainerId
		scope = "docker"
	}

	handle := "create"
	if destroy {
		handle = "destroy"
	}

	command := fmt.Sprintf("%s %s %s %s %s %s",
		getTargetChaosBladeBin(expModel),
		handle,
		scope,
		expModel.Target,
		expModel.ActionName,
		matchers)

	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		var generatedCommand string
		if expModel.Target == "network" && handle == "destroy" {
			labels := []string{
				fmt.Sprintf("io.kubernetes.pod.name=%s", obj.PodName),
				fmt.Sprintf("io.kubernetes.pod.namespace=%s", obj.Namespace),
			}
			if obj.ContainerRuntime == container.DockerRuntime {
				labels = append(labels, "io.kubernetes.docker.type=podsandbox")
			} else if obj.ContainerRuntime == container.ContainerdRuntime {
				labels = append(labels, "io.cri-containerd.kind=sandbox")
			} else {
				logrus.WithField("experiment", experimentId).
					Errorf("unsupported container runtime %s", obj.ContainerRuntime)
				return identifiers, fmt.Errorf("unsupported container runtime %s", obj.ContainerRuntime)
			}
			generatedCommand = fmt.Sprintf("%s --container-label-selector %s --container-runtime %s", command, strings.Join(labels, ","), obj.ContainerRuntime)
		} else {
			generatedCommand = fmt.Sprintf("%s --container-id %s", command, obj.ContainerId)
			if expModel.ActionProcessHang {
				generatedCommand = fmt.Sprintf("%s --cgroup-root /host-sys/fs/cgroup", generatedCommand)
			}
			if scope == "cri" {
				generatedCommand = fmt.Sprintf("%s --container-runtime %s", generatedCommand, obj.ContainerRuntime)
			}
			if obj.Id != "" {
				generatedCommand = fmt.Sprintf("%s --uid %s", generatedCommand, obj.Id)
			}
		}

		daemonsetPodName, err := GetChaosBladeDaemonsetPodName(obj.NodeName, client)
		if err != nil {
			logrus.WithField("experiment", experimentId).
				Errorf("get chaosblade tool pod for destroying failed on %s node, %v", obj.NodeName, err)
			return identifiers, err
		}
		identifierInPod := ExperimentIdentifierInPod{
			ContainerObjectMeta:     containerObjectMetaList[idx],
			Command:                 generatedCommand,
			ChaosBladeContainerName: chaosblade.DaemonsetPodName,
			ChaosBladeNamespace:     chaosblade.DaemonsetPodNamespace,
			ChaosBladePodName:       daemonsetPodName,
		}
		identifiers = append(identifiers, identifierInPod)
	}
	return identifiers, nil
}

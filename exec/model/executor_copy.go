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
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"path"
	"sync"
)

type ExperimentIdentifierInPod struct {
	ContainerObjectMeta
	Command string
	Error   string
	Code    int32
	// For daemonset
	ChaosBladePodName       string
	ChaosBladeNamespace     string
	ChaosBladeContainerName string
}

type ExecCommandInPodExecutor struct {
	Client *channel.Client
}

func (e *ExecCommandInPodExecutor) Name() string {
	return "execInPod"
}

func (e *ExecCommandInPodExecutor) SetChannel(channel spec.Channel) {
}

// execInMatchedPod will execute the experiment in the target pod
func (e *ExecCommandInPodExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	logrusField := logrus.WithField("experiment", GetExperimentIdFromContext(ctx))
	experimentStatus := v1alpha1.ExperimentStatus{
		ResStatuses: make([]v1alpha1.ResourceStatus, 0),
	}
	experimentIdentifiers, err := getExperimentIdentifiers(ctx, expModel, e.Client)
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

func getExperimentIdentifiers(ctx context.Context, expModel *spec.ExpModel, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	delete(expModel.ActionFlags, "uid")
	containerObjectMetaList, err := GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		return []ExperimentIdentifierInPod{}, err
	}
	excludeFlagsFunc := ExcludeKeyFunc()
	matchers := spec.ConvertExpMatchersToString(expModel, excludeFlagsFunc)
	experimentId := GetExperimentIdFromContext(ctx)
	_, destroy := spec.IsDestroy(ctx)

	isDockerNetwork := expModel.ActionFlags[IsDockerNetworkFlag.Name] == "true"
	UseSidecarContainerNetwork := expModel.ActionFlags[UseSidecarContainerNetworkFlag.Name] == "true"
	isContainerSelfTarget := expModel.Target == "container"
	isContainerNetworkTarget := expModel.Target == "network"
	isNodeScope := expModel.Scope == "node"
	if isNodeScope {
		return getNodeExperimentIdentifiers(experimentId, expModel, containerObjectMetaList, matchers, destroy, client)
	}
	if isContainerSelfTarget || (isContainerNetworkTarget && (isDockerNetwork || UseSidecarContainerNetwork)) {
		if version.CheckVerisonHaveCriCommand() || containerObjectMetaList[0].ContainerRuntime == container.ContainerdRuntime {
			return getCriExperimentIdentifiers(experimentId, expModel, containerObjectMetaList, matchers, destroy, isContainerNetworkTarget, client)
		}
		return getDockerExperimentIdentifiers(experimentId, expModel, containerObjectMetaList, matchers, destroy, isContainerNetworkTarget, client)
	}
	if destroy {
		return generateDestroyCommands(experimentId, expModel, containerObjectMetaList, matchers, client)
	}
	return generateCreateCommands(experimentId, expModel, containerObjectMetaList, matchers, client)
}

func getDockerExperimentIdentifiers(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string, destroy, isNetworkTarget bool, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	if isNetworkTarget {
		matchers = fmt.Sprintf("%s --image-repo %s --image-version %s",
			matchers, chaosblade.Constant.ImageRepoFunc(), chaosblade.Version)
	}
	if destroy {
		return generateDestroyDockerCommands(experimentId, expModel, containerObjectMetaList, matchers, isNetworkTarget, client)
	}
	return generateCreateDockerCommands(experimentId, expModel, containerObjectMetaList, matchers, client)
}

func getCriExperimentIdentifiers(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string, destroy, isNetworkTarget bool, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	if isNetworkTarget {
		matchers = fmt.Sprintf("%s --image-repo %s --image-version %s",
			matchers, chaosblade.Constant.ImageRepoFunc(), chaosblade.Version)
	}
	if destroy {
		return generateDestroyCriCommands(experimentId, expModel, containerObjectMetaList, matchers, isNetworkTarget, client)
	}
	return generateCreateCriCommands(experimentId, expModel, containerObjectMetaList, matchers, client)
}

func generateDestroyDockerCommands(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string, isNetworkTarget bool, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s destroy docker %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		daemonsetPodName, err := GetChaosBladeDaemonsetPodName(obj.NodeName, client)
		if err != nil {
			logrus.WithField("experiment", experimentId).
				Errorf("get chaosblade tool pod for destroying failed on %s node, %v", obj.NodeName, err)
			return identifiers, err
		}
		generatedCommand := command
		if isNetworkTarget {
			newContainerId, err := getNewContainerIdByPod(obj.PodName, obj.Namespace, obj.ContainerName, experimentId, client)
			if err != nil {
				logrus.WithField("experiment", experimentId).Errorf("generate destroy docker command failed, %v", err)
				continue
			}
			generatedCommand = fmt.Sprintf("%s --container-id %s", generatedCommand, newContainerId)
		} else {
			if obj.Id != "" {
				generatedCommand = fmt.Sprintf("%s --uid %s", command, obj.Id)
			}
			generatedCommand = fmt.Sprintf("%s --container-name %s", generatedCommand, obj.ContainerName)
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

func generateCreateDockerCommands(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s create docker %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)

	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		daemonsetPodName, err := GetChaosBladeDaemonsetPodName(obj.NodeName, client)
		if err != nil {
			logrus.WithField("experiment", experimentId).
				Errorf("get chaosblade tool pod for creating failed on %s node, %v", obj.NodeName, err)
			return identifiers, err
		}
		generatedCommand := fmt.Sprintf("%s --container-id %s", command, obj.ContainerId)
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

func generateDestroyCriCommands(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string, isNetworkTarget bool, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s destroy cri %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		daemonsetPodName, err := GetChaosBladeDaemonsetPodName(obj.NodeName, client)
		if err != nil {
			logrus.WithField("experiment", experimentId).
				Errorf("get chaosblade tool pod for destroying failed on %s node, %v", obj.NodeName, err)
			return identifiers, err
		}
		generatedCommand := command
		if isNetworkTarget {
			generatedCommand = fmt.Sprintf("%s --container-id %s --container-runtime %s", generatedCommand, obj.ContainerId, obj.ContainerRuntime)
		} else {
			if obj.Id != "" {
				generatedCommand = fmt.Sprintf("%s --uid %s", command, obj.Id)
			}
			generatedCommand = fmt.Sprintf("%s --container-name %s --container-runtime %s", generatedCommand, obj.ContainerName, obj.ContainerRuntime)
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

func generateCreateCriCommands(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s create cri %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)

	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		daemonsetPodName, err := GetChaosBladeDaemonsetPodName(obj.NodeName, client)
		if err != nil {
			logrus.WithField("experiment", experimentId).
				Errorf("get chaosblade tool pod for creating failed on %s node, %v", obj.NodeName, err)
			return identifiers, err
		}
		generatedCommand := fmt.Sprintf("%s --container-id %s --container-runtime %s", command, obj.ContainerId,
			containerObjectMetaList[idx].ContainerRuntime)
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

func deployChaosBlade(experimentId string, expModel *spec.ExpModel,
	obj ContainerObjectMeta, override bool, client *channel.Client) *spec.Response {
	logrusField := logrus.WithField("experiment", experimentId)
	chaosBladePath := getTargetChaosBladePath(expModel)
	options := DeployOptions{
		Container: obj.ContainerName,
		Namespace: obj.Namespace,
		PodName:   obj.PodName,
		client:    client,
	}
	deploy, err := getDeployMode(options, expModel)
	if err != nil {
		util.Errorf(experimentId, util.GetRunFuncName(), spec.ParameterLess.Sprintf(ChaosBladeDownloadUrlFlag.Name))
		return spec.ResponseFailWithFlags(spec.ParameterLess, ChaosBladeDownloadUrlFlag.Name)
	}
	logrusField.Infof("deploy chaosblade under override with %t value", override)
	chaosBladeBinPath := path.Join(chaosBladePath, "bin")
	if err := options.CheckFileExists(chaosBladeBinPath); err != nil {
		// create chaosblade path
		if err := options.CreateDir(chaosBladeBinPath); err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf("create chaosblade dir: %s, failed! err: %s", chaosBladeBinPath, err.Error()))
			return spec.ResponseFailWithFlags(spec.ParameterInvalidBladePathError, ChaosBladePathFlag.Name, chaosBladeBinPath, err)
		}
	}
	bladePath := path.Join(chaosBladePath, "blade")
	if override || options.CheckFileExists(bladePath) != nil {
		if err := deploy.DeployToPod(experimentId, chaosblade.OperatorChaosBladeBlade, bladePath); err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf("deploy blade failed! dir: %s, err: %s", bladePath, err.Error()))
			return spec.ResponseFailWithFlags(spec.DeployChaosBladeFailed, bladePath, err)
		}
	}
	yamlPath := path.Join(chaosBladePath, "yaml")
	if override || options.CheckFileExists(yamlPath) != nil {
		if err := deploy.DeployToPod(experimentId, chaosblade.OperatorChaosBladeYaml, yamlPath); err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf("deploy yaml failed! dir: %s, err: %s", yamlPath, err.Error()))
			return spec.ResponseFailWithFlags(spec.DeployChaosBladeFailed, yamlPath, err)
		}
	}
	chaosOSPath := path.Join(chaosBladePath, "bin", "chaos_os")
	if override || options.CheckFileExists(chaosOSPath) != nil {
		if err := deploy.DeployToPod(experimentId, path.Join(chaosblade.OperatorChaosBladeBin, "chaos_os"), chaosOSPath); err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf("deploy chaos_os failed! dir: %s, err: %s", chaosOSPath, err.Error()))
			return spec.ResponseFailWithFlags(spec.DeployChaosBladeFailed, chaosOSPath, err)
		}
	}
	// copy files as needed
	for _, program := range expModel.ActionPrograms {
		var programFile, operatorProgramFile string
		switch program {
		case "java":
			programFile = path.Join(chaosBladePath, "lib")
			operatorProgramFile = chaosblade.OperatorChaosBladeLib
		default:
			programFile = path.Join(chaosBladePath, "bin", program)
			operatorProgramFile = path.Join(chaosblade.OperatorChaosBladeBin, program)
		}
		if !override && options.CheckFileExists(programFile) == nil {
			logrusField.WithField("program", programFile).Infof("program exists")
			continue
		}
		err := deploy.DeployToPod(experimentId, operatorProgramFile, programFile)
		logrusField = logrusField.WithFields(logrus.Fields{
			"container": obj.ContainerName,
			"pod":       obj.PodName,
			"namespace": obj.Namespace,
		})
		if err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf("copy chaosblade to pod failed! dir: %s, err: %s", yamlPath, err.Error()))
			return spec.ResponseFailWithFlags(spec.K8sExecFailed, "copyToPod", err)
		}
		logrusField.Infof("deploy %s success", programFile)
	}
	return spec.Success()
}

func getNewContainerIdByPod(podName, podNamespace, containerName, experimentId string, client *channel.Client) (string, error) {
	pod := v1.Pod{}
	err := client.Get(context.TODO(), types.NamespacedName{Namespace: podNamespace, Name: podName}, &pod)
	if err != nil {
		logrus.WithFields(
			logrus.Fields{
				"experiment":    experimentId,
				"containerName": containerName,
			}).Warningf("can not find the pod by %s name in %s namespace, %v", podName, podNamespace, err)
		return "", err
	}
	containerStatuses := pod.Status.ContainerStatuses
	if containerStatuses == nil {
		return "", fmt.Errorf("cannot find containers in %s pod", podName)
	}
	for _, containerStatus := range containerStatuses {
		if containerName == containerStatus.Name {
			_, containerLongId := TruncateContainerObjectMetaUid(containerStatus.ContainerID)
			if len(containerLongId) > 12 {
				return containerLongId[:12], nil
			}
			return "", fmt.Errorf("the container %s id is illegal", containerLongId)
		}
	}
	return "", fmt.Errorf("cannot find the %s container in %s pod", containerName, podName)
}

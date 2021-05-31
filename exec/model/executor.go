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
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chaosblade-io/chaosblade-spec-go/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	pkglabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
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

func (e *ExecCommandInPodExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	return e.execInMatchedPod(uid, ctx, expModel)
}

// getExperimentIdentifiers
func (e *ExecCommandInPodExecutor) getExperimentIdentifiers(ctx context.Context, expModel *spec.ExpModel) ([]ExperimentIdentifierInPod, error) {
	containerObjectMetaList, err := GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		return []ExperimentIdentifierInPod{}, err
	}
	excludeFlagsFunc := ExcludeKeyFunc()
	matchers := spec.ConvertExpMatchersToString(expModel, excludeFlagsFunc)
	experimentId := GetExperimentIdFromContext(ctx)
	_, destroy := spec.IsDestroy(ctx)

	isContainerSelfTarget := expModel.Target == "container"
	isContainerNetworkTarget := expModel.Target == "network"
	isNodeScope := expModel.Scope == "node"
	if isNodeScope {
		return e.getNodeExperimentIdentifiers(experimentId, expModel, containerObjectMetaList, matchers, destroy)
	}
	if chaosblade.DaemonsetEnable && (isContainerSelfTarget || isContainerNetworkTarget) {
		return e.getDockerExperimentIdentifiers(experimentId, expModel, containerObjectMetaList, matchers, destroy, isContainerNetworkTarget)
	}
	if isContainerSelfTarget {
		return []ExperimentIdentifierInPod{},
			errors.New("daemonset-enable must be true to execute container-self chaos experiments")
	}
	if destroy {
		return e.generateDestroyCommands(experimentId, expModel, containerObjectMetaList, matchers)
	}
	return e.generateCreateCommands(experimentId, expModel, containerObjectMetaList, matchers)
}

// execInMatchedPod will execute the experiment in the target pod
func (e *ExecCommandInPodExecutor) execInMatchedPod(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	logrusField := logrus.WithField("experiment", GetExperimentIdFromContext(ctx))
	experimentStatus := v1alpha1.ExperimentStatus{
		ResStatuses: make([]v1alpha1.ResourceStatus, 0),
	}
	experimentIdentifiers, err := e.getExperimentIdentifiers(ctx, expModel)
	if err != nil {
		logrusField.Errorf("get experiment identifiers failed, err: %s", err.Error())
		return spec.ResponseFailWaitResult(spec.K8sExecFailed, fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].Err, uid),
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo,
				"getExperimentIdentifiers", err.Error()), nil))
	}
	// TODO experimentIdentifiers 中存在执行失败任务，需要透出
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
			rsStatus.CreateFailResourceStatus(identifier.Error, spec.K8sExecFailed)
			execSuccess = false
		} else {
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
					// if get pod error, the execution is considered failure.
					msg := fmt.Sprintf("get pod: %s in %s error",
						identifier.PodName, identifier.Namespace)
					rsStatus.CreateFailResourceStatus(msg, spec.K8sExecFailed)
					execSuccess = false
				}
			}
			logrusField.Infof("execute identifier: %+v", identifier)
			execSuccess, rsStatus = e.execCommands(isDestroy, rsStatus, identifier)
		}
		updateResultLock.Lock()
		statuses = append(statuses, rsStatus)
		// If false occurs once, the result is false.
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

	checkExperimentStatus(ctx, expModel, statuses, experimentIdentifiers, e)
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

func checkExperimentStatus(ctx context.Context, expModel *spec.ExpModel, statuses []v1alpha1.ResourceStatus, identifiers []ExperimentIdentifierInPod, e *ExecCommandInPodExecutor) {
	tt := expModel.ActionFlags["timeout"]
	if _, ok := spec.IsDestroy(ctx); !ok && tt != "" && len(statuses) > 0 {
		experimentId := GetExperimentIdFromContext(ctx)
		go func() {
			timeout, err := strconv.ParseUint(tt, 10, 64)
			if err != nil {
				// the err checked in RunE function
				timeDuartion, _ := time.ParseDuration(tt)
				timeout = uint64(timeDuartion.Seconds())
			}
			time.Sleep(time.Duration(timeout) * time.Second)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			defer cancel()

			ticker := time.NewTicker(time.Second)
		TickerLoop:
			for range ticker.C {
				select {
				case <-ctx.Done():
					ticker.Stop()
					break TickerLoop
				default:
					isDestroyed := true
					for i, status := range statuses {
						if !status.Success {
							continue
						}
						containerObjectMeta := ParseIdentifier(status.Identifier)
						identifier := identifiers[i]
						podName := containerObjectMeta.PodName
						podNamespace := containerObjectMeta.Namespace
						containerName := containerObjectMeta.ContainerName
						if identifier.ChaosBladePodName != "" {
							podName = identifier.ChaosBladePodName
							podNamespace = identifier.ChaosBladeNamespace
							containerName = identifier.ChaosBladeContainerName
						}
						response := e.Client.Exec(&channel.ExecOptions{
							StreamOptions: channel.StreamOptions{
								ErrDecoder: func(bytes []byte) interface{} {
									content := string(bytes)
									util.Errorf(identifier.Id, util.GetRunFuncName(), fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "exec", content))
									return spec.Decode(content, spec.ResponseFailWaitResult(spec.K8sExecFailed, fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].Err, experimentId),
										fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "exec", content)))
								},
								OutDecoder: func(bytes []byte) interface{} {
									content := string(bytes)
									util.Errorf(identifier.Id, util.GetRunFuncName(), fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "exec", content))
									return spec.Decode(content, spec.ResponseFailWaitResult(spec.K8sExecFailed, fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].Err, experimentId),
										fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "exec", content)))
								},
							},
							PodName:       podName,
							PodNamespace:  podNamespace,
							ContainerName: containerName,
							Command:       []string{getTargetChaosBladeBin(expModel), "status", status.Id},
							IgnoreOutput:  false,
						}).(*spec.Response)
						if response.Success {
							result := response.Result.(map[string]interface{})
							if result["Status"] != v1alpha1.DestroyedState {
								isDestroyed = false
								break
							}
						} else {
							isDestroyed = false
							break
						}
					}

					if isDestroyed {
						logrus.Info("The experiment was destroyed, ExperimentId: ", experimentId)
						cli := e.Client.Client
						objectMeta := metav1.ObjectMeta{Name: experimentId}
						err := cli.Delete(context.TODO(), &v1alpha1.ChaosBlade{
							TypeMeta: metav1.TypeMeta{
								APIVersion: "chaosblade.io/v1alpha1",
								Kind:       "ChaosBlade",
							},
							ObjectMeta: objectMeta,
						})
						if err != nil {
							logrus.Warn(err.Error())
						} else {
							ticker.Stop()
						}
					}
				}
			}
		}()
	}
}

func (e *ExecCommandInPodExecutor) execCommands(isDestroy bool, rsStatus v1alpha1.ResourceStatus,
	identifier ExperimentIdentifierInPod) (bool, v1alpha1.ResourceStatus) {
	success := false
	// handle chaos experiments using daemonset mode
	podName := identifier.PodName
	podNamespace := identifier.Namespace
	containerName := identifier.ContainerName
	if identifier.ChaosBladePodName != "" {
		podName = identifier.ChaosBladePodName
		podNamespace = identifier.ChaosBladeNamespace
		containerName = identifier.ChaosBladeContainerName
	}
	response := e.Client.Exec(&channel.ExecOptions{
		StreamOptions: channel.StreamOptions{
			IOStreams: channel.IOStreams{
				Out:    bytes.NewBuffer([]byte{}),
				ErrOut: bytes.NewBuffer([]byte{}),
			},
			ErrDecoder: func(bytes []byte) interface{} {
				content := string(bytes)
				util.Errorf(identifier.Id, util.GetRunFuncName(), fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "exec", content))
				return spec.Decode(content, spec.ResponseFailWaitResult(spec.K8sExecFailed, fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].Err, identifier.Id),
					fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "exec", content)))
			},
			OutDecoder: func(bytes []byte) interface{} {
				content := string(bytes)
				util.Errorf(identifier.Id, util.GetRunFuncName(), fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "exec", content))
				return spec.Decode(content, spec.ResponseFailWaitResult(spec.K8sExecFailed, fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].Err, identifier.Id),
					fmt.Sprintf(spec.ResponseErr[spec.K8sExecFailed].ErrInfo, "exec", content)))
			},
		},
		PodName:       podName,
		PodNamespace:  podNamespace,
		ContainerName: containerName,
		Command:       strings.Split(identifier.Command, " "),
	}).(*spec.Response)

	if response.Success {
		if !isDestroy {
			rsStatus.Id = response.Result.(string)
		}
		rsStatus = rsStatus.CreateSuccessResourceStatus()
		success = true
	} else {
		rsStatus = rsStatus.CreateFailResourceStatus(response.Err, response.Code)
	}
	return success, rsStatus
}

func (e *ExecCommandInPodExecutor) generateDestroyCommands(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s destroy %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	override := expModel.ActionFlags["override"] == "true"
	for idx, obj := range containerObjectMetaList {
		generatedCommand := command
		if obj.Id != "" {
			generatedCommand = fmt.Sprintf("%s --uid %s", command, obj.Id)
		}
		identifierInPod := ExperimentIdentifierInPod{
			ContainerObjectMeta: containerObjectMetaList[idx],
			Command:             generatedCommand,
		}
		err, code := e.deployChaosBlade(experimentId, expModel, obj, override)
		if err != nil {
			identifierInPod.Error = err.Error()
			identifierInPod.Code = code
		}
		identifiers = append(identifiers, identifierInPod)
	}
	return identifiers, nil
}

func (e *ExecCommandInPodExecutor) generateCreateCommands(experimentId string, expModel *spec.ExpModel, containerObjectMetaList ContainerMatchedList,
	matchers string) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s create %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	override := expModel.ActionFlags["override"] == "true"
	for idx, obj := range containerObjectMetaList {
		identifierInPod := ExperimentIdentifierInPod{
			ContainerObjectMeta: containerObjectMetaList[idx],
			Command:             command,
		}
		err, code := e.deployChaosBlade(experimentId, expModel, obj, override)
		if err != nil {
			identifierInPod.Error = err.Error()
			identifierInPod.Code = code
		}
		identifiers = append(identifiers, identifierInPod)
	}
	return identifiers, nil
}

func (e *ExecCommandInPodExecutor) deployChaosBlade(experimentId string, expModel *spec.ExpModel,
	obj ContainerObjectMeta, override bool) (error, int32) {
	logrusField := logrus.WithField("experiment", experimentId)
	chaosBladePath := getTargetChaosBladePath(expModel)
	options := CopyOptions{
		Container: obj.ContainerName,
		Namespace: obj.Namespace,
		PodName:   obj.PodName,
		client:    e.Client,
	}

	logrusField.Infof("deploy chaosblade under override with %t value", override)
	// 校验 chaosblade 目录是否存在
	chaosBladeBinPath := path.Join(chaosBladePath, "bin")
	if err := options.CheckFileExists(chaosBladeBinPath); err != nil {
		// create chaosblade path
		if err := options.CreateDir(chaosBladeBinPath); err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf("create chaosblade dir: %s, failed! err: %s", chaosBladeBinPath, err.Error()))
			return fmt.Errorf(spec.ResponseErr[spec.ParameterInvalidBladePathError].Err, chaosBladeBinPath, err.Error()), spec.ParameterInvalidBladePathError
		}
	}
	// 部署 blade 和 yaml 文件
	// todo ： 后续返回错误码，因为不确定在复制过程中会出现什么异常错误，就统一用 ParameterInvalidBladePathError 处理
	bladePath := path.Join(chaosBladePath, "blade")
	if override || options.CheckFileExists(bladePath) != nil {
		if err := options.CopyToPod(experimentId, chaosblade.OperatorChaosBladeBlade, bladePath); err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf("deploy blade failed! dir: %s, err: %s", bladePath, err.Error()))
			return fmt.Errorf("deploy blade failed, %v", err), spec.ParameterInvalidBladePathError
		}
	}
	yamlPath := path.Join(chaosBladePath, "yaml")
	if override || options.CheckFileExists(yamlPath) != nil {
		if err := options.CopyToPod(experimentId, chaosblade.OperatorChaosBladeYaml, yamlPath); err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf("deploy yaml failed! dir: %s, err: %s", yamlPath, err.Error()))
			return fmt.Errorf("deploy yaml failed, %v", err), spec.ParameterInvalidBladePathError
		}
	}
	chaosOSPath := path.Join(chaosBladePath, "bin", "chaos_os")
	if override || options.CheckFileExists(chaosOSPath) != nil {
		if err := options.CopyToPod(experimentId, path.Join(chaosblade.OperatorChaosBladeBin, "chaos_os"), chaosOSPath); err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf("deploy chaos_os failed! dir: %s, err: %s", chaosOSPath, err.Error()))
			return fmt.Errorf("deploy chaos_os failed, %v", err), spec.ParameterInvalidBladePathError
		}
	}

	// 按需复制
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
		err := options.CopyToPod(experimentId, operatorProgramFile, programFile)
		logrusField = logrusField.WithFields(logrus.Fields{
			"container": obj.ContainerName,
			"pod":       obj.PodName,
			"namespace": obj.Namespace,
		})
		if err != nil {
			util.Errorf(experimentId, util.GetRunFuncName(), fmt.Sprintf("copy chaosblade to pod failed! dir: %s, err: %s", yamlPath, err.Error()))
			return fmt.Errorf("copy chaosblade to pod failed, %v", err), spec.ParameterInvalidBladePathError
		}
		logrusField.Infof("deploy %s success", programFile)
	}
	return nil, 0
}

func (e *ExecCommandInPodExecutor) getDockerExperimentIdentifiers(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string, destroy, isNetworkTarget bool) ([]ExperimentIdentifierInPod, error) {
	if isNetworkTarget {
		matchers = fmt.Sprintf("%s --image-repo %s --image-version %s",
			matchers, chaosblade.Constant.ImageRepoFunc(), chaosblade.Version)
	}
	if destroy {
		return e.generateDestroyDockerCommands(experimentId, expModel, containerObjectMetaList, matchers)
	}
	return e.generateCreateDockerCommands(experimentId, expModel, containerObjectMetaList, matchers)
}

// GetChaosBladeDaemonsetPodName
func (e *ExecCommandInPodExecutor) GetChaosBladeDaemonsetPodName(nodeName string) (string, error) {
	podName := chaosblade.DaemonsetPodNames[nodeName]
	if podName == "" {
		if err := e.refreshChaosBladeDaemonsetPodNames(); err != nil {
			return "", err
		}
		return chaosblade.DaemonsetPodNames[nodeName], nil
	}
	// check
	pod := v1.Pod{}
	err := e.Client.Get(context.Background(), client.ObjectKey{
		Namespace: chaosblade.DaemonsetPodNamespace,
		Name:      podName,
	}, &pod)
	if err == nil {
		return podName, nil
	}
	// refresh
	if err := e.refreshChaosBladeDaemonsetPodNames(); err != nil {
		return "", err
	}
	return chaosblade.DaemonsetPodNames[nodeName], nil
}

func (e *ExecCommandInPodExecutor) refreshChaosBladeDaemonsetPodNames() error {
	podList := v1.PodList{}
	opts := client.ListOptions{
		Namespace:     chaosblade.DaemonsetPodNamespace,
		LabelSelector: pkglabels.SelectorFromSet(chaosblade.DaemonsetPodLabels),
	}
	if err := e.Client.List(context.TODO(), &podList, &opts); err != nil {
		return err
	}
	podNames := make(map[string]string, len(podList.Items))
	for _, pod := range podList.Items {
		podNames[pod.Spec.NodeName] = pod.Name
	}
	chaosblade.DaemonsetPodNames = podNames
	return nil
}

func (e *ExecCommandInPodExecutor) generateDestroyDockerCommands(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s destroy docker %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		generatedCommand := command
		if obj.Id != "" {
			generatedCommand = fmt.Sprintf("%s --uid %s", command, obj.Id)
		}
		daemonsetPodName, err := e.GetChaosBladeDaemonsetPodName(obj.NodeName)
		if err != nil {
			logrus.WithField("experiment", experimentId).
				Errorf("get chaosblade tool pod for destroying failed on %s node, %v", obj.NodeName, err)
			return identifiers, err
		}
		generatedCommand = fmt.Sprintf("%s --container-id %s", generatedCommand, obj.ContainerId)
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

func (e *ExecCommandInPodExecutor) generateCreateDockerCommands(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s create docker %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)

	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		daemonsetPodName, err := e.GetChaosBladeDaemonsetPodName(obj.NodeName)
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

func (e *ExecCommandInPodExecutor) getNodeExperimentIdentifiers(experimentId string, expModel *spec.ExpModel, containerMatchedList ContainerMatchedList, matchers string, destroy bool) ([]ExperimentIdentifierInPod, error) {
	if destroy {
		return e.generateDestroyNodeCommands(experimentId, expModel, containerMatchedList, matchers)
	}
	return e.generateCreateNodeCommands(experimentId, expModel, containerMatchedList, matchers)
}

func (e *ExecCommandInPodExecutor) generateDestroyNodeCommands(experimentId string, expModel *spec.ExpModel, containerObjectMetaList ContainerMatchedList, matchers string) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s destroy %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		generatedCommand := command
		if obj.Id != "" {
			generatedCommand = fmt.Sprintf("%s --uid %s", command, obj.Id)
		}
		daemonsetPodName, err := e.GetChaosBladeDaemonsetPodName(obj.NodeName)
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

func (e *ExecCommandInPodExecutor) generateCreateNodeCommands(experimentId string, expModel *spec.ExpModel, containerObjectMetaList ContainerMatchedList, matchers string) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s create %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		daemonsetPodName, err := e.GetChaosBladeDaemonsetPodName(obj.NodeName)
		if err != nil {
			logrus.WithField("experiment", experimentId).
				Errorf("get chaosblade tool pod for creating failed on %s node, %v", obj.NodeName, err)
			return identifiers, err
		}
		identifierInPod := ExperimentIdentifierInPod{
			ContainerObjectMeta:     containerObjectMetaList[idx],
			Command:                 command,
			ChaosBladeContainerName: chaosblade.DaemonsetPodName,
			ChaosBladeNamespace:     chaosblade.DaemonsetPodNamespace,
			ChaosBladePodName:       daemonsetPodName,
		}
		identifiers = append(identifiers, identifierInPod)
	}
	return identifiers, nil
}

// getTargetChaosBladePath return the chaosblade deployed path in target container
func getTargetChaosBladePath(expModel *spec.ExpModel) string {
	deployedPath := expModel.ActionFlags[ChaosBladeDeployedPathFlag.Name]
	if deployedPath == "" {
		return chaosblade.OperatorChaosBladePath
	}
	return path.Join(deployedPath, "chaosblade")
}

// getTargetChaosBladeBin returns the blade deployed path in target container
func getTargetChaosBladeBin(expModel *spec.ExpModel) string {
	return path.Join(getTargetChaosBladePath(expModel), "blade")
}

func ExcludeKeyFunc() func() map[string]spec.Empty {
	return GetResourceFlagNames
}

func TruncateContainerObjectMetaUid(uid string) string {
	return strings.ReplaceAll(uid, "docker://", "")
}

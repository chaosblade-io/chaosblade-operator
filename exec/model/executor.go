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
	"time"

	"github.com/chaosblade-io/chaosblade-exec-cri/exec"
	"github.com/chaosblade-io/chaosblade-exec-cri/exec/container"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	pkglabels "k8s.io/apimachinery/pkg/labels"
	cli "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
)

func checkExperimentStatus(ctx context.Context, expModel *spec.ExpModel, statuses []v1alpha1.ResourceStatus, identifiers []ExperimentIdentifierInPod, client *channel.Client) {
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
						response := client.Exec(&channel.ExecOptions{
							StreamOptions: channel.StreamOptions{
								ErrDecoder: func(bytes []byte) interface{} {
									content := string(bytes)
									util.Errorf(identifier.Id, util.GetRunFuncName(), spec.K8sExecFailed.Sprintf("pods/exec", content))
									return spec.Decode(content, spec.ResponseFailWithFlags(spec.K8sExecFailed, "pods/exec", content))
								},
								OutDecoder: func(bytes []byte) interface{} {
									content := string(bytes)
									util.Errorf(identifier.Id, util.GetRunFuncName(), spec.K8sExecFailed.Sprintf("pods/exec", content))
									return spec.Decode(content, spec.ResponseFailWithFlags(spec.K8sExecFailed, "pods/exec", content))
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
						cb := &v1alpha1.ChaosBlade{}
						err := client.Client.Get(context.TODO(), types.NamespacedName{Name: experimentId}, cb)
						if err != nil {
							logrus.Warn(err.Error())
							continue
						}

						if cb.Status.Phase != v1alpha1.ClusterPhaseDestroyed {
							cb.Status.Phase = v1alpha1.ClusterPhaseDestroyed
							err = client.Client.Status().Update(context.TODO(), cb)
							if err != nil {
								logrus.Warn(err.Error())
							}
							continue
						}

						objectMeta := metav1.ObjectMeta{Name: experimentId}

						err = client.Client.Delete(context.TODO(), &v1alpha1.ChaosBlade{
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

func execCommands(isDestroy bool, rsStatus v1alpha1.ResourceStatus,
	identifier ExperimentIdentifierInPod, client *channel.Client) (bool, v1alpha1.ResourceStatus) {
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
	response := client.Exec(&channel.ExecOptions{
		StreamOptions: channel.StreamOptions{
			IOStreams: channel.IOStreams{
				Out:    bytes.NewBuffer([]byte{}),
				ErrOut: bytes.NewBuffer([]byte{}),
			},
			ErrDecoder: func(bytes []byte) interface{} {
				content := string(bytes)
				util.Errorf(identifier.Id, util.GetRunFuncName(), spec.K8sExecFailed.Sprintf("pods/exec", content))
				return spec.Decode(content, spec.ResponseFailWithFlags(spec.K8sExecFailed, "pods/exec", content))
			},
			OutDecoder: func(bytes []byte) interface{} {
				content := string(bytes)
				util.Infof(identifier.Id, util.GetRunFuncName(), fmt.Sprintf("exec output: %s", content))
				// TODO ?? 不应该返回错我
				return spec.Decode(content, spec.ResponseFailWithFlags(spec.K8sExecFailed, "pods/exec", content))
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

func generateDestroyCommands(experimentId string, expModel *spec.ExpModel,
	containerObjectMetaList ContainerMatchedList, matchers string, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s destroy %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		generatedCommand := command
		if obj.Id != "" {
			generatedCommand = fmt.Sprintf("%s --uid %s", command, obj.Id)
		}
		identifierInPod := ExperimentIdentifierInPod{
			ContainerObjectMeta: containerObjectMetaList[idx],
			Command:             generatedCommand,
		}
		resp := deployChaosBlade(experimentId, expModel, obj, false, client)
		if !resp.Success {
			identifierInPod.Error = resp.Err
			identifierInPod.Code = resp.Code
		}
		identifiers = append(identifiers, identifierInPod)
	}
	return identifiers, nil
}

func generateCreateCommands(experimentId string, expModel *spec.ExpModel, containerObjectMetaList ContainerMatchedList,
	matchers string, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s create %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	chaosBladeOverride := expModel.ActionFlags[exec.ChaosBladeOverrideFlag.Name] == "true"
	for idx, obj := range containerObjectMetaList {
		identifierInPod := ExperimentIdentifierInPod{
			ContainerObjectMeta: containerObjectMetaList[idx],
			Command:             command,
		}
		resp := deployChaosBlade(experimentId, expModel, obj, chaosBladeOverride, client)
		if !resp.Success {
			identifierInPod.Error = resp.Err
			identifierInPod.Code = resp.Code
		}
		identifiers = append(identifiers, identifierInPod)
	}
	return identifiers, nil
}

// GetChaosBladeDaemonsetPodName
func GetChaosBladeDaemonsetPodName(nodeName string, client *channel.Client) (string, error) {
	podName := chaosblade.DaemonsetPodNames[nodeName]
	if podName == "" {
		if err := refreshChaosBladeDaemonsetPodNames(client); err != nil {
			return "", err
		}
		return chaosblade.DaemonsetPodNames[nodeName], nil
	}
	// check
	pod := v1.Pod{}
	err := client.Get(context.Background(), cli.ObjectKey{
		Namespace: chaosblade.DaemonsetPodNamespace,
		Name:      podName,
	}, &pod)
	if err == nil {
		return podName, nil
	}
	// refresh
	if err := refreshChaosBladeDaemonsetPodNames(client); err != nil {
		return "", err
	}
	return chaosblade.DaemonsetPodNames[nodeName], nil
}

func refreshChaosBladeDaemonsetPodNames(client *channel.Client) error {
	podList := v1.PodList{}
	opts := cli.ListOptions{
		Namespace:     chaosblade.DaemonsetPodNamespace,
		LabelSelector: pkglabels.SelectorFromSet(chaosblade.DaemonsetPodLabels),
	}
	if err := client.List(context.TODO(), &podList, &opts); err != nil {
		return err
	}
	podNames := make(map[string]string, len(podList.Items))
	for _, pod := range podList.Items {
		podNames[pod.Spec.NodeName] = pod.Name
	}
	chaosblade.DaemonsetPodNames = podNames
	return nil
}

func getNodeExperimentIdentifiers(experimentId string, expModel *spec.ExpModel, containerMatchedList ContainerMatchedList, matchers string, destroy bool, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	if destroy {
		return generateDestroyNodeCommands(experimentId, expModel, containerMatchedList, matchers, client)
	}
	return generateCreateNodeCommands(experimentId, expModel, containerMatchedList, matchers, client)
}

func generateDestroyNodeCommands(experimentId string, expModel *spec.ExpModel, containerObjectMetaList ContainerMatchedList, matchers string, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s destroy %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		generatedCommand := command
		if obj.Id != "" {
			generatedCommand = fmt.Sprintf("%s --uid %s", command, obj.Id)
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

func generateCreateNodeCommands(experimentId string, expModel *spec.ExpModel, containerObjectMetaList ContainerMatchedList, matchers string, client *channel.Client) ([]ExperimentIdentifierInPod, error) {
	command := fmt.Sprintf("%s create %s %s %s", getTargetChaosBladeBin(expModel), expModel.Target, expModel.ActionName, matchers)
	identifiers := make([]ExperimentIdentifierInPod, 0)
	for idx, obj := range containerObjectMetaList {
		daemonsetPodName, err := GetChaosBladeDaemonsetPodName(obj.NodeName, client)
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
	chaosbladePath := expModel.ActionFlags[ChaosBladePathFlag.Name]
	if chaosbladePath == "" {
		return chaosblade.OperatorChaosBladePath
	}
	return path.Join(chaosbladePath, "chaosblade")
}

// getTargetChaosBladeBin returns the blade deployed path in target container
func getTargetChaosBladeBin(expModel *spec.ExpModel) string {
	return path.Join(getTargetChaosBladePath(expModel), "blade")
}

func ExcludeKeyFunc() func() map[string]spec.Empty {
	return GetResourceFlagNames
}

func TruncateContainerObjectMetaUid(uid string) (containerRuntime, containerId string) {
	if strings.HasPrefix(uid, "containerd://") {
		return container.ContainerdRuntime, strings.ReplaceAll(uid, "containerd://", "")
	}

	return container.DockerRuntime, strings.ReplaceAll(uid, "docker://", "")
}

func getDeployMode(options DeployOptions, expModel *spec.ExpModel) (DeployMode, error) {
	mode := expModel.ActionFlags[ChaosBladeDeployModeFlag.Name]
	url := expModel.ActionFlags[ChaosBladeDownloadUrlFlag.Name]
	switch mode {
	case CopyMode:
		return &CopyOptions{options}, nil
	case DownloadMode:
		if url == "" {
			url = chaosblade.DownloadUrl
		}
		if url == "" {
			return nil, errors.New("must config the chaosblade-download-url flag")
		}
		return &DownloadOptions{options, url}, nil
	default:
		return &CopyOptions{options}, nil
	}
}

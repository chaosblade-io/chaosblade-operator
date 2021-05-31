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
	"fmt"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
)

func GetOneAvailableContainerIdFromPod(pod v1.Pod) (containerId, containerName string, err error) {
	containerStatuses := pod.Status.ContainerStatuses
	if containerStatuses == nil || len(containerStatuses) == 0 {
		return "", "", fmt.Errorf("the container statues is empty in %s pod", pod.Name)
	}
	for _, containerStatus := range containerStatuses {
		if containerStatus.State.Running == nil {
			continue
		}
		containerId := TruncateContainerObjectMetaUid(containerStatus.ContainerID)
		return containerId, containerStatus.Name, nil
	}
	return "", "", fmt.Errorf("cannot find a valiable container in %s pod", pod.Name)
}

func ParseLabels(labels string) map[string]string {
	labelsMap := make(map[string]string, 0)
	if labels == "" {
		return labelsMap
	}
	labelArr := strings.Split(labels, ",")
	for _, label := range labelArr {
		keyValue := strings.SplitN(label, "=", 2)
		if len(keyValue) != 2 {
			logrus.Warningf("label %s is illegal", label)
			continue
		}
		labelsMap[keyValue[0]] = keyValue[1]
	}
	return labelsMap
}

func MapContains(bigMap map[string]string, subMap map[string]string) bool {
	if bigMap == nil || subMap == nil {
		return false
	}
	for k, v := range subMap {
		if bigMap[k] != v {
			return false
		}
	}
	return true
}

func CheckFlags(flags map[string]string) (error, int32) {
	// Must include one flag in the count, percent, labels and names
	expFlags := []*spec.ExpFlag{
		ResourceCountFlag,
		ResourcePercentFlag,
		ResourceLabelsFlag,
		ResourceNamesFlag,
	}
	value := ""
	flagsNames := make([]string, 0)
	for _, flag := range expFlags {
		flagsNames = append(flagsNames, flag.Name)
		value = fmt.Sprintf("%s%s", value, flags[flag.Name])
	}
	if value == "" {
		return fmt.Errorf(spec.ResponseErr[spec.ParameterLess].ErrInfo, strings.Join(flagsNames, "|")), spec.ParameterLess
	}
	return nil, spec.Success
}

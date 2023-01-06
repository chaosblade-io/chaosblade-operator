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
	"k8s.io/apimachinery/pkg/selection"
	pkglabels "k8s.io/apimachinery/pkg/labels"
)

func GetOneAvailableContainerIdFromPod(pod v1.Pod) (containerId, containerName, runtime string, err error) {
	containerStatuses := pod.Status.ContainerStatuses
	if containerStatuses == nil || len(containerStatuses) == 0 {
		return "", "", "", fmt.Errorf("the container statues is empty in %s pod", pod.Name)
	}
	for _, containerStatus := range containerStatuses {
		if containerStatus.State.Running == nil {
			continue
		}
		runtime, containerId := TruncateContainerObjectMetaUid(containerStatus.ContainerID)
		return containerId, containerStatus.Name, runtime, nil
	}
	return "", "", "", fmt.Errorf("cannot find a valiable container in %s pod", pod.Name)
}

func ParseLabels(labels string) []pkglabels.Requirement {
	labelArr := strings.Split(labels, ",")
	requirements := make([]pkglabels.Requirement, 0, len(labelArr))
	labelsMap := make(map[string][]string, 0)
	if labels == "" {
		return requirements
	}

	for _, label := range labelArr {
		keyValue := strings.SplitN(label, "=", 2)
		if len(keyValue) != 2 {
			logrus.Warningf("label %s is illegal", label)
			continue
		}
		if labelsMap[keyValue[0]] == nil {
			valueArr := make([]string, 0)
			valueArr = append(valueArr, keyValue[1])
			labelsMap[keyValue[0]] = valueArr
		} else {
			labelsMap[keyValue[0]] = append(labelsMap[keyValue[0]], keyValue[1])
		}
	}

	for label, value := range labelsMap {
		requirement, err := pkglabels.NewRequirement(label, selection.In, value)
		if err != nil {
			logrus.Warningf("requirement %s-%s is illegal", label, value)
			continue
		}
		requirements = append(requirements, *requirement)
	}
	return requirements
}

func MapContains(bigMap map[string]string, requirements []pkglabels.Requirement) bool {
	if bigMap == nil || requirements == nil {
		return false
	}
	labelSet := pkglabels.Set(bigMap)
	for i := 0; i < len(requirements); i++ {
		if requirements[i].Matches(labelSet) {
			return true
		}
	}
	return false
}

func CheckFlags(flags map[string]string) *spec.Response {
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
		return spec.ResponseFailWithFlags(spec.ParameterLess, strings.Join(flagsNames, "|"))
	}
	return spec.Success()
}

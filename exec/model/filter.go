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

	"k8s.io/api/core/v1"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
)

func CheckFlags(flags map[string]string) error {
	// 必须包含 count,percent,labels,names中的一个
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
		return fmt.Errorf("must specify one flag in %s", strings.Join(flagsNames, ","))
	}
	return nil
}

func GetOneAvailableContainerIdFromPod(pod v1.Pod) (containerId, containerName string, err error) {
	containerStatuses := pod.Status.ContainerStatuses
	if containerStatuses == nil || len(containerStatuses) == 0 {
		return "", "", fmt.Errorf("the container statues is empty in %s pod", pod.Name)
	}
	for _, containerStatus := range containerStatuses {
		if containerStatus.State.Running == nil {
			continue
		}
		return TruncateContainerObjectMetaUid(containerStatus.ContainerID), containerStatus.Name, nil
	}
	return "", "", fmt.Errorf("cannot find a valiable container in %s pod", pod.Name)
}

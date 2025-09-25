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
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	ContainerObjectMetaListKey = "ContainerObjectMetaListKey"
	ExperimentIdKey            = "ExperimentIdKey"
)

type ContainerObjectMeta struct {
	// experiment id
	Id               string
	ContainerRuntime string
	ContainerId      string
	ContainerName    string
	PodName          string
	NodeName         string
	Namespace        string
}

type ContainerMatchedList []ContainerObjectMeta

// GetExperimentIdFromContext
func GetExperimentIdFromContext(ctx context.Context) string {
	experimentId := ctx.Value(ExperimentIdKey)
	if experimentId == nil {
		return "UnknownId"
	}
	return experimentId.(string)
}

// SetExperimentIdToContext
func SetExperimentIdToContext(ctx context.Context, experimentId string) context.Context {
	return context.WithValue(ctx, ExperimentIdKey, experimentId)
}

// GetContainerObjectMetaListFromContext returns the matched container list
func GetContainerObjectMetaListFromContext(ctx context.Context) (ContainerMatchedList, error) {
	containerObjectMetaListValue := ctx.Value(ContainerObjectMetaListKey)
	if containerObjectMetaListValue == nil {
		return nil, fmt.Errorf("less container object meta in context")
	}
	containerObjectMetaList := containerObjectMetaListValue.(ContainerMatchedList)
	return containerObjectMetaList, nil
}

// SetContainerObjectMetaListToContext
func SetContainerObjectMetaListToContext(ctx context.Context, containerMatchedList ContainerMatchedList) context.Context {
	logrus.WithField("experiment", GetExperimentIdFromContext(ctx)).Infof("set container list: %+v", containerMatchedList)
	return context.WithValue(ctx, ContainerObjectMetaListKey, containerMatchedList)
}

func (c *ContainerObjectMeta) GetIdentifier() string {
	identifier := fmt.Sprintf("%s/%s/%s", c.Namespace, c.NodeName, c.PodName)
	if c.ContainerName != "" {
		identifier = fmt.Sprintf("%s/%s", identifier, c.ContainerName)
	}
	if c.ContainerId != "" {
		identifier = fmt.Sprintf("%s/%s", identifier, c.ContainerId)
	}
	if c.ContainerRuntime != "" {
		identifier = fmt.Sprintf("%s/%s", identifier, c.ContainerRuntime)
	}
	return identifier
}

// Namespace/Node/Pod/ContainerName/ContainerId/containerRuntime
func ParseIdentifier(identifier string) ContainerObjectMeta {
	ss := strings.SplitN(identifier, "/", 6)
	meta := ContainerObjectMeta{}
	switch len(ss) {
	case 0:
		return meta
	case 1:
		meta.Namespace = ss[0]
	case 2:
		meta.Namespace = ss[0]
		meta.NodeName = ss[1]
	case 3:
		meta.Namespace = ss[0]
		meta.NodeName = ss[1]
		meta.PodName = ss[2]
	case 4:
		meta.Namespace = ss[0]
		meta.NodeName = ss[1]
		meta.PodName = ss[2]
		meta.ContainerName = ss[3]
	case 5:
		meta.Namespace = ss[0]
		meta.NodeName = ss[1]
		meta.PodName = ss[2]
		meta.ContainerName = ss[3]
		meta.ContainerId = ss[4]
	case 6:
		meta.Namespace = ss[0]
		meta.NodeName = ss[1]
		meta.PodName = ss[2]
		meta.ContainerName = ss[3]
		meta.ContainerId = ss[4]
		meta.ContainerRuntime = ss[5]
	}
	return meta
}

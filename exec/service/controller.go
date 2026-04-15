/*
 * Copyright 2025 The ChaosBlade Authors
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

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/sirupsen/logrus"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const ServiceMetaListKey = "ServiceMetaListKey"

type ServiceMeta struct {
	Id          string
	ServiceName string
	Namespace   string
}

type ServiceMetaList []ServiceMeta

func GetServiceMetaListFromContext(ctx context.Context) (ServiceMetaList, error) {
	val := ctx.Value(ServiceMetaListKey)
	if val == nil {
		return nil, fmt.Errorf("less service meta in context")
	}
	return val.(ServiceMetaList), nil
}

func SetServiceMetaListToContext(ctx context.Context, list ServiceMetaList) context.Context {
	return context.WithValue(ctx, ServiceMetaListKey, list)
}

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
	return "service"
}

func (e *ExpController) Create(ctx context.Context, expSpec v1alpha1.ExperimentSpec) *spec.Response {
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrus.WithField("experiment", experimentId).Infof("creating service experiment")
	return e.Exec(ctx, expModel)
}

func (e *ExpController) Destroy(ctx context.Context, expSpec v1alpha1.ExperimentSpec, oldExpStatus v1alpha1.ExperimentStatus) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrus.WithField("experiment", experimentId).Infoln("start to destroy service experiment")
	expModel := model.ExtractExpModelFromExperimentSpec(expSpec)
	statuses := oldExpStatus.ResStatuses
	if statuses == nil {
		return spec.ReturnSuccess(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{}))
	}
	serviceMetaList := ServiceMetaList{}
	for _, status := range statuses {
		if !status.Success {
			continue
		}
		meta := parseServiceIdentifier(status.Identifier)
		meta.Id = status.Id
		serviceMetaList = append(serviceMetaList, meta)
	}
	ctx = SetServiceMetaListToContext(ctx, serviceMetaList)
	return e.Exec(ctx, expModel)
}

// parseServiceIdentifier parses identifier in format "Namespace/ServiceName"
func parseServiceIdentifier(identifier string) ServiceMeta {
	parts := strings.SplitN(identifier, "/", 2)
	meta := ServiceMeta{}
	if len(parts) >= 1 {
		meta.Namespace = parts[0]
	}
	if len(parts) >= 2 {
		meta.ServiceName = parts[1]
	}
	return meta
}

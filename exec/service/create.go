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
	"strconv"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	NamePrefixFlag      = "name-prefix"
	ServiceCountFlag    = "service-count"
	PortsPerServiceFlag = "ports-per-service"
)

type CreateServiceActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewCreateServiceActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &CreateServiceActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name:     NamePrefixFlag,
					Desc:     "Name prefix for the created services",
					NoArgs:   false,
					Required: true,
				},
				&spec.ExpFlag{
					Name:     ServiceCountFlag,
					Desc:     "Number of services to create",
					NoArgs:   false,
					Required: true,
				},
				&spec.ExpFlag{
					Name:   PortsPerServiceFlag,
					Desc:   "Number of ports per service, min 1, max 100, default 10",
					NoArgs: false,
				},
			},
			ActionExecutor: &CreateServiceActionExecutor{client: client},
			ActionExample: `# Create 2000 services with prefix my-service in default namespace
blade create k8s service-self create --name-prefix my-service --namespace default --service-count 2000 --kubeconfig ~/.kube/config`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*CreateServiceActionSpec) Name() string {
	return "create"
}

func (*CreateServiceActionSpec) Aliases() []string {
	return []string{}
}

func (*CreateServiceActionSpec) ShortDesc() string {
	return "Create services in batch"
}

func (*CreateServiceActionSpec) LongDesc() string {
	return "Create the specified number of Kubernetes services with given name prefix, each containing configurable port mappings and no selector"
}

type CreateServiceActionExecutor struct {
	client *channel.Client
}

func (*CreateServiceActionExecutor) Name() string {
	return "create"
}

func (*CreateServiceActionExecutor) SetChannel(channel spec.Channel) {
}

func (d *CreateServiceActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *CreateServiceActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	logrusField := logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx))

	namePrefix := expModel.ActionFlags[NamePrefixFlag]
	if namePrefix == "" {
		util.Errorf(uid, util.GetRunFuncName(), "name-prefix is required")
		return spec.ResponseFailWithResult(spec.ParameterLess,
			v1alpha1.CreateFailExperimentStatus("name-prefix is required", []v1alpha1.ResourceStatus{}),
			NamePrefixFlag)
	}

	namespace := expModel.ActionFlags["namespace"]
	if namespace == "" {
		namespace = "default"
	}

	serviceCountStr := expModel.ActionFlags[ServiceCountFlag]
	if serviceCountStr == "" {
		util.Errorf(uid, util.GetRunFuncName(), "service-count is required")
		return spec.ResponseFailWithResult(spec.ParameterLess,
			v1alpha1.CreateFailExperimentStatus("service-count is required", []v1alpha1.ResourceStatus{}),
			ServiceCountFlag)
	}
	serviceCount, err := strconv.Atoi(serviceCountStr)
	if err != nil {
		return spec.ResponseFailWithResult(spec.ParameterIllegal,
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf("service-count is invalid: %v", err), []v1alpha1.ResourceStatus{}),
			ServiceCountFlag, serviceCountStr, err)
	}
	if serviceCount < 1 || serviceCount > 20000 {
		return spec.ResponseFailWithResult(spec.ParameterIllegal,
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf("service-count must be between 1 and 20000, got %d", serviceCount), []v1alpha1.ResourceStatus{}),
			ServiceCountFlag, strconv.Itoa(serviceCount), "must be between 1 and 20000")
	}

	portsPerService := 10
	if v := expModel.ActionFlags[PortsPerServiceFlag]; v != "" {
		portsPerService, err = strconv.Atoi(v)
		if err != nil {
			return spec.ResponseFailWithResult(spec.ParameterIllegal,
				v1alpha1.CreateFailExperimentStatus(fmt.Sprintf("ports-per-service is invalid: %v", err), []v1alpha1.ResourceStatus{}),
				PortsPerServiceFlag, v, err)
		}
	}
	if portsPerService < 1 || portsPerService > 100 {
		return spec.ResponseFailWithResult(spec.ParameterIllegal,
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf("ports-per-service must be between 1 and 100, got %d", portsPerService), []v1alpha1.ResourceStatus{}),
			PortsPerServiceFlag, strconv.Itoa(portsPerService), "must be between 1 and 100")
	}

	logrusField.Infof("creating %d services with prefix %s in namespace %s", serviceCount, namePrefix, namespace)

	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false
	for i := 0; i < serviceCount; i++ {
		serviceName := fmt.Sprintf("%s-%s-%d", namePrefix, uid, i)
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.ServiceKind,
			Identifier: fmt.Sprintf("%s/%s", namespace, serviceName),
		}

		svc := buildService(serviceName, namespace, portsPerService, uid)
		if err := d.client.Create(context.TODO(), svc); err != nil {
			logrusField.Warningf("create service %s err, %v", serviceName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
		} else {
			status = status.CreateSuccessResourceStatus()
			success = true
		}
		statuses = append(statuses, status)
	}

	var experimentStatus v1alpha1.ExperimentStatus
	if success {
		experimentStatus = v1alpha1.CreateSuccessExperimentStatus(statuses)
	} else {
		experimentStatus = v1alpha1.CreateFailExperimentStatus("see resStatuses for details", statuses)
	}
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

func (d *CreateServiceActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	logrusField := logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx))

	serviceMetaList, err := GetServiceMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus("cannot get service meta from context", []v1alpha1.ResourceStatus{}))
	}

	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false
	for _, meta := range serviceMetaList {
		status := v1alpha1.ResourceStatus{
			Id:         meta.Id,
			Kind:       v1alpha1.ServiceKind,
			Identifier: fmt.Sprintf("%s/%s", meta.Namespace, meta.ServiceName),
		}

		svc := &v1.Service{}
		objectKey := types.NamespacedName{Name: meta.ServiceName, Namespace: meta.Namespace}
		if err := d.client.Get(context.TODO(), objectKey, svc); err != nil {
			logrusField.Warningf("get service %s err, %v", meta.ServiceName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		if _, ok := svc.Annotations[ServiceAnnotation]; !ok {
			errMsg := fmt.Sprintf("service %s/%s is not created by chaosblade (missing annotation %s), skip delete",
				meta.Namespace, meta.ServiceName, ServiceAnnotation)
			logrusField.Warning(errMsg)
			status = status.CreateFailResourceStatus(errMsg, spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		objectMeta := metav1.ObjectMeta{Name: meta.ServiceName, Namespace: meta.Namespace}
		if err := d.client.Delete(context.TODO(), &v1.Service{ObjectMeta: objectMeta}); err != nil {
			logrusField.Warningf("delete service %s err, %v", meta.ServiceName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
		} else {
			status.State = v1alpha1.DestroyedState
			status.Success = true
			success = true
		}
		statuses = append(statuses, status)
	}

	var experimentStatus v1alpha1.ExperimentStatus
	if success {
		experimentStatus = v1alpha1.CreateDestroyedExperimentStatus(statuses)
	} else {
		experimentStatus = v1alpha1.CreateFailExperimentStatus("see resStatuses for details", statuses)
	}
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

func buildService(name, namespace string, portsPerService int, uid string) *v1.Service {
	const portBase = 8000
	ports := make([]v1.ServicePort, 0, portsPerService)
	for i := 0; i < portsPerService; i++ {
		port := int32(portBase + i)
		ports = append(ports, v1.ServicePort{
			Name:       fmt.Sprintf("p%d", port),
			Port:       port,
			TargetPort: intstr.FromInt32(port),
		})
	}

	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"chaosblade.io/service": fmt.Sprintf("create-%s", uid),
			},
		},
		Spec: v1.ServiceSpec{
			Ports: ports,
		},
	}
}

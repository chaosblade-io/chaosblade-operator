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
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	NamePrefixFlag = "name-prefix"
	CountFlag      = "count"
	PortBaseFlag   = "port-base"
	PortCountFlag  = "port-count"
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
					Name:     CountFlag,
					Desc:     "Number of services to create",
					NoArgs:   false,
					Required: true,
				},
				&spec.ExpFlag{
					Name:   PortBaseFlag,
					Desc:   "Base port number for service ports, default is 8000",
					NoArgs: false,
				},
				&spec.ExpFlag{
					Name:   PortCountFlag,
					Desc:   "Number of ports per service, default is 10",
					NoArgs: false,
				},
			},
			ActionExecutor: &CreateServiceActionExecutor{client: client},
			ActionExample: `# Create 1000 services with prefix my-service in default namespace
blade create k8s service-self create --name-prefix my-service --namespace default --count 1000 --kubeconfig ~/.kube/config`,
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

	countStr := expModel.ActionFlags[CountFlag]
	if countStr == "" {
		util.Errorf(uid, util.GetRunFuncName(), "count is required")
		return spec.ResponseFailWithResult(spec.ParameterLess,
			v1alpha1.CreateFailExperimentStatus("count is required", []v1alpha1.ResourceStatus{}),
			CountFlag)
	}
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return spec.ResponseFailWithResult(spec.ParameterIllegal,
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf("count is invalid: %v", err), []v1alpha1.ResourceStatus{}),
			CountFlag, countStr, err)
	}
	if count < 1 {
		return spec.ResponseFailWithResult(spec.ParameterIllegal,
			v1alpha1.CreateFailExperimentStatus("count is invalid: must be greater than or equal to 1", []v1alpha1.ResourceStatus{}),
			CountFlag, countStr)
	}

	portBase := 8000
	if v := expModel.ActionFlags[PortBaseFlag]; v != "" {
		portBase, err = strconv.Atoi(v)
		if err != nil {
			return spec.ResponseFailWithResult(spec.ParameterIllegal,
				v1alpha1.CreateFailExperimentStatus(fmt.Sprintf("port-base is invalid: %v", err), []v1alpha1.ResourceStatus{}),
				PortBaseFlag, v, err)
		}
	}

	portCount := 10
	if v := expModel.ActionFlags[PortCountFlag]; v != "" {
		portCount, err = strconv.Atoi(v)
		if err != nil {
			return spec.ResponseFailWithResult(spec.ParameterIllegal,
				v1alpha1.CreateFailExperimentStatus(fmt.Sprintf("port-count is invalid: %v", err), []v1alpha1.ResourceStatus{}),
				PortCountFlag, v, err)
		}
	}

	logrusField.Infof("creating %d services with prefix %s in namespace %s", count, namePrefix, namespace)

	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false
	for i := 0; i < count; i++ {
		serviceName := fmt.Sprintf("%s-%d", namePrefix, i)
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.ServiceKind,
			Identifier: fmt.Sprintf("%s/%s", namespace, serviceName),
		}

		svc := buildService(serviceName, namespace, portBase, portCount)
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

func buildService(name, namespace string, portBase, portCount int) *v1.Service {
	ports := make([]v1.ServicePort, 0, portCount)
	for i := 0; i < portCount; i++ {
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
		},
		Spec: v1.ServiceSpec{
			Ports: ports,
		},
	}
}

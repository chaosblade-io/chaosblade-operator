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

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	ServiceNameFlag            = "name"
	ExternalTrafficPolicyFlag  = "externalTrafficPolicy"
	InternalTrafficPolicyFlag  = "internalTrafficPolicy"
	OriginalPolicyAnnotationFn = "chaosblade.io/original-%s"
)

type ModifyServiceActionSpec struct {
	spec.BaseExpActionCommandSpec
}

func NewModifyServiceActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &ModifyServiceActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name:     ServiceNameFlag,
					Desc:     "Service name to modify",
					NoArgs:   false,
					Required: true,
				},
				&spec.ExpFlag{
					Name:   ExternalTrafficPolicyFlag,
					Desc:   "Set externalTrafficPolicy, values: Local or Cluster",
					NoArgs: false,
				},
				&spec.ExpFlag{
					Name:   InternalTrafficPolicyFlag,
					Desc:   "Set internalTrafficPolicy, values: Local or Cluster",
					NoArgs: false,
				},
			},
			ActionExecutor: &ModifyServiceActionExecutor{client: client},
			ActionExample: `# Modify externalTrafficPolicy to Local
blade create k8s service-self modify --name my-service --namespace default --externalTrafficPolicy Local --kubeconfig ~/.kube/config

# Modify internalTrafficPolicy to Local
blade create k8s service-self modify --name my-service --namespace default --internalTrafficPolicy Local --kubeconfig ~/.kube/config

# Modify both policies
blade create k8s service-self modify --name my-service --namespace default --externalTrafficPolicy Local --internalTrafficPolicy Cluster --kubeconfig ~/.kube/config`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*ModifyServiceActionSpec) Name() string {
	return "modify"
}

func (*ModifyServiceActionSpec) Aliases() []string {
	return []string{}
}

func (*ModifyServiceActionSpec) ShortDesc() string {
	return "Modify service traffic policy"
}

func (*ModifyServiceActionSpec) LongDesc() string {
	return "Modify existing Kubernetes service's externalTrafficPolicy or internalTrafficPolicy"
}

type ModifyServiceActionExecutor struct {
	client *channel.Client
}

func (*ModifyServiceActionExecutor) Name() string {
	return "modify"
}

func (*ModifyServiceActionExecutor) SetChannel(channel spec.Channel) {
}

func (d *ModifyServiceActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *ModifyServiceActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	logrusField := logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx))

	serviceName := expModel.ActionFlags[ServiceNameFlag]
	if serviceName == "" {
		util.Errorf(uid, util.GetRunFuncName(), "name is required")
		return spec.ResponseFailWithResult(spec.ParameterLess,
			v1alpha1.CreateFailExperimentStatus("name is required", []v1alpha1.ResourceStatus{}),
			ServiceNameFlag)
	}

	namespace := expModel.ActionFlags["namespace"]
	if namespace == "" {
		namespace = "default"
	}

	externalPolicy := expModel.ActionFlags[ExternalTrafficPolicyFlag]
	internalPolicy := expModel.ActionFlags[InternalTrafficPolicyFlag]
	if externalPolicy == "" && internalPolicy == "" {
		util.Errorf(uid, util.GetRunFuncName(), "at least one of externalTrafficPolicy or internalTrafficPolicy is required")
		return spec.ResponseFailWithResult(spec.ParameterLess,
			v1alpha1.CreateFailExperimentStatus("at least one of externalTrafficPolicy or internalTrafficPolicy is required", []v1alpha1.ResourceStatus{}),
			fmt.Sprintf("%s or %s", ExternalTrafficPolicyFlag, InternalTrafficPolicyFlag))
	}

	if externalPolicy != "" && !isValidPolicy(externalPolicy) {
		return spec.ResponseFailWithResult(spec.ParameterIllegal,
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf("invalid externalTrafficPolicy: %s, must be Local or Cluster", externalPolicy), []v1alpha1.ResourceStatus{}),
			ExternalTrafficPolicyFlag, externalPolicy, "must be Local or Cluster")
	}
	if internalPolicy != "" && !isValidPolicy(internalPolicy) {
		return spec.ResponseFailWithResult(spec.ParameterIllegal,
			v1alpha1.CreateFailExperimentStatus(fmt.Sprintf("invalid internalTrafficPolicy: %s, must be Local or Cluster", internalPolicy), []v1alpha1.ResourceStatus{}),
			InternalTrafficPolicyFlag, internalPolicy, "must be Local or Cluster")
	}

	logrusField.Infof("modifying service %s/%s, externalTrafficPolicy=%s, internalTrafficPolicy=%s",
		namespace, serviceName, externalPolicy, internalPolicy)

	status := v1alpha1.ResourceStatus{
		Kind:       v1alpha1.ServiceKind,
		Identifier: fmt.Sprintf("%s/%s", namespace, serviceName),
	}

	svc := &v1.Service{}
	objectKey := types.NamespacedName{Name: serviceName, Namespace: namespace}
	if err := d.client.Get(context.TODO(), objectKey, svc); err != nil {
		logrusField.Errorf("get service %s err, %v", serviceName, err)
		status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
		return spec.ReturnResultIgnoreCode(
			v1alpha1.CreateFailExperimentStatus(err.Error(), []v1alpha1.ResourceStatus{status}))
	}

	if svc.Annotations == nil {
		svc.Annotations = make(map[string]string)
	}

	if externalPolicy != "" {
		annotationKey := fmt.Sprintf(OriginalPolicyAnnotationFn, ExternalTrafficPolicyFlag)
		if _, exists := svc.Annotations[annotationKey]; !exists {
			svc.Annotations[annotationKey] = string(svc.Spec.ExternalTrafficPolicy)
		}
		switch externalPolicy {
		case string(v1.ServiceExternalTrafficPolicyTypeLocal):
			svc.Spec.ExternalTrafficPolicy = v1.ServiceExternalTrafficPolicyTypeLocal
		case string(v1.ServiceExternalTrafficPolicyTypeCluster):
			svc.Spec.ExternalTrafficPolicy = v1.ServiceExternalTrafficPolicyTypeCluster
		default:
			err := fmt.Errorf("invalid externalTrafficPolicy %q, must be %q or %q",
				externalPolicy,
				v1.ServiceExternalTrafficPolicyTypeLocal,
				v1.ServiceExternalTrafficPolicyTypeCluster)
			logrusField.Errorf("modify service %s err, %v", serviceName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(
				v1alpha1.CreateFailExperimentStatus(err.Error(), []v1alpha1.ResourceStatus{status}))
		}
	}

	if internalPolicy != "" {
		annotationKey := fmt.Sprintf(OriginalPolicyAnnotationFn, InternalTrafficPolicyFlag)
		if _, exists := svc.Annotations[annotationKey]; !exists {
			if svc.Spec.InternalTrafficPolicy != nil {
				svc.Annotations[annotationKey] = string(*svc.Spec.InternalTrafficPolicy)
			} else {
				svc.Annotations[annotationKey] = ""
			}
		}
		policy := v1.ServiceInternalTrafficPolicyType(internalPolicy)
		svc.Spec.InternalTrafficPolicy = &policy
	}

	if err := d.client.Update(context.TODO(), svc); err != nil {
		logrusField.Errorf("update service %s err, %v", serviceName, err)
		status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
		return spec.ReturnResultIgnoreCode(
			v1alpha1.CreateFailExperimentStatus(err.Error(), []v1alpha1.ResourceStatus{status}))
	}

	status = status.CreateSuccessResourceStatus()
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{status}))
}

func (d *ModifyServiceActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	logrusField := logrus.WithField("experiment", model.GetExperimentIdFromContext(ctx))

	serviceMetaList, err := GetServiceMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus("cannot get service meta from context", []v1alpha1.ResourceStatus{}))
	}

	statuses := make([]v1alpha1.ResourceStatus, 0)
	for _, meta := range serviceMetaList {
		status := v1alpha1.ResourceStatus{
			Id:         meta.Id,
			Kind:       v1alpha1.ServiceKind,
			Identifier: fmt.Sprintf("%s/%s", meta.Namespace, meta.ServiceName),
		}

		svc := &v1.Service{}
		objectKey := types.NamespacedName{Name: meta.ServiceName, Namespace: meta.Namespace}
		if err := d.client.Get(context.TODO(), objectKey, svc); err != nil {
			logrusField.Errorf("get service %s for restoring err, %v", meta.ServiceName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		restored := false
		extAnnotationKey := fmt.Sprintf(OriginalPolicyAnnotationFn, ExternalTrafficPolicyFlag)
		if original, ok := svc.Annotations[extAnnotationKey]; ok {
			if !isValidPolicy(original) {
				err := fmt.Errorf("invalid externalTrafficPolicy value %q for service %s/%s", original, meta.Namespace, meta.ServiceName)
				logrusField.Errorf("restore service %s err, %v", meta.ServiceName, err)
				status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				continue
			}
			svc.Spec.ExternalTrafficPolicy = v1.ServiceExternalTrafficPolicyType(original)
			delete(svc.Annotations, extAnnotationKey)
			restored = true
		}

		intAnnotationKey := fmt.Sprintf(OriginalPolicyAnnotationFn, InternalTrafficPolicyFlag)
		if original, ok := svc.Annotations[intAnnotationKey]; ok {
			if original == "" {
				svc.Spec.InternalTrafficPolicy = nil
			} else {
				policy := v1.ServiceInternalTrafficPolicyType(original)
				svc.Spec.InternalTrafficPolicy = &policy
			}
			delete(svc.Annotations, intAnnotationKey)
			restored = true
		}

		if restored {
			if err := d.client.Update(context.TODO(), svc); err != nil {
				logrusField.Errorf("restore service %s err, %v", meta.ServiceName, err)
				status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				continue
			}
		}

		status.State = v1alpha1.DestroyedState
		status.Success = true
		statuses = append(statuses, status)
	}
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus(statuses))
}

func isValidPolicy(policy string) bool {
	return policy == "Local" || policy == "Cluster"
}

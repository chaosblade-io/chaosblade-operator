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

package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	pkglabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	sigsclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	// DnsServiceFlag is the flag name for the cluster DNS service.
	DnsServiceFlag = "dns-service"
	// DnsServiceNamespaceFlag is the flag name for the cluster DNS service's namespace.
	DnsServiceNamespaceFlag = "dns-service-namespace"

	// DefaultDnsServiceName is the default cluster DNS service name (CoreDNS / kube-dns).
	DefaultDnsServiceName = "kube-dns"
	// DefaultDnsServiceNamespace is the default namespace where the cluster DNS service lives.
	DefaultDnsServiceNamespace = "kube-system"

	// ClusterDnsKind is the resource kind exposed in ChaosBlade status for cluster DNS faults.
	ClusterDnsKind = "cluster-dns"

	// ChaosBladeClusterDnsAnnotation indicates that the deployment was modified by a cluster DNS experiment.
	ChaosBladeClusterDnsAnnotation = "chaosblade.io/cluster-dns"
	// ChaosBladeOriginalReplicasAnnotation stores the original replica count of the DNS workload.
	ChaosBladeOriginalReplicasAnnotation = "chaosblade.io/original-replicas"
	// ChaosBladeExperimentAnnotation is the annotation key recording the experiment id.
	ChaosBladeExperimentAnnotation = "chaosblade.io/experiment"
	// ChaosBladeClusterDnsAction is the annotation value of the cluster DNS action.
	ChaosBladeClusterDnsAction = "dnsfailure"
)

// ClusterDnsFailureActionSpec implements DNS server outage at the cluster level.
//
// The action looks up the configured DNS Service, finds the underlying Deployment
// by matching the Service's selector against Deployment pod template labels, then
// scales the Deployment to zero replicas to make the cluster DNS completely
// unavailable. The original replica count is backed up in an annotation so the
// destroy flow can recover the workload.
type ClusterDnsFailureActionSpec struct {
	spec.BaseExpActionCommandSpec
	client *channel.Client
}

func NewClusterDnsFailureActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &ClusterDnsFailureActionSpec{
		BaseExpActionCommandSpec: spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name:    DnsServiceFlag,
					Desc:    "The cluster DNS service name. Default: kube-dns",
					Default: DefaultDnsServiceName,
				},
				&spec.ExpFlag{
					Name:    DnsServiceNamespaceFlag,
					Desc:    "The namespace of the cluster DNS service. Default: kube-system",
					Default: DefaultDnsServiceNamespace,
				},
			},
			ActionExecutor: &ClusterDnsFailureActionExecutor{client: client},
			ActionExample: `# Make the cluster DNS server completely unavailable by scaling the kube-dns / CoreDNS deployment to 0
blade create k8s cluster-dns dnsfailure --kubeconfig ~/.kube/config

# Specify a custom DNS service name and namespace
blade create k8s cluster-dns dnsfailure --dns-service coredns --dns-service-namespace kube-system --kubeconfig ~/.kube/config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
		client: client,
	}
}

func (*ClusterDnsFailureActionSpec) Name() string {
	return "dnsfailure"
}

func (*ClusterDnsFailureActionSpec) Aliases() []string {
	return []string{}
}

func (*ClusterDnsFailureActionSpec) ShortDesc() string {
	return "Make the cluster DNS server completely unavailable"
}

func (*ClusterDnsFailureActionSpec) LongDesc() string {
	return "Inject a complete unavailability fault to the cluster DNS server. " +
		"The action resolves the underlying DNS workload (Deployment) from the " +
		"DNS service's selector, backs up its replica count, and scales it to 0 " +
		"so that no DNS query can be answered cluster-wide. When the experiment " +
		"is destroyed, the original replica count is restored."
}

type ClusterDnsFailureActionExecutor struct {
	client *channel.Client
}

func (*ClusterDnsFailureActionExecutor) Name() string {
	return "dnsfailure"
}

func (*ClusterDnsFailureActionExecutor) SetChannel(channel spec.Channel) {}

func (d *ClusterDnsFailureActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *ClusterDnsFailureActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	dnsService, dnsNamespace := resolveDnsServiceFlags(expModel)

	status := v1alpha1.ResourceStatus{
		Kind:       ClusterDnsKind,
		Identifier: fmt.Sprintf("%s/%s", dnsNamespace, dnsService),
	}

	deployment, err := d.findDnsDeployment(ctx, dnsNamespace, dnsService)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		logrusField.Warningf("locate cluster DNS deployment failed: %v", err)
		status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
		return spec.ReturnResultIgnoreCode(
			v1alpha1.CreateFailExperimentStatus(err.Error(), []v1alpha1.ResourceStatus{status}),
		)
	}

	status.Identifier = fmt.Sprintf("%s/%s/%s", dnsNamespace, dnsService, deployment.Name)

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &appsv1.Deployment{}
		if err := d.client.Get(ctx, types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}, latest); err != nil {
			return err
		}
		return d.injectClusterDnsFailure(ctx, latest, experimentId)
	}); err != nil {
		logrusField.Warningf("inject cluster DNS failure to %s/%s failed: %v", deployment.Namespace, deployment.Name, err)
		status = status.CreateFailResourceStatus(fmt.Sprintf("inject cluster DNS failure failed: %v", err), spec.K8sExecFailed.Code)
		return spec.ReturnResultIgnoreCode(
			v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}),
		)
	}

	logrusField.Infof("scaled cluster DNS deployment %s/%s to 0 replicas", deployment.Namespace, deployment.Name)
	status = status.CreateSuccessResourceStatus()
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{status}))
}

func (d *ClusterDnsFailureActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	dnsService, dnsNamespace := resolveDnsServiceFlags(expModel)

	status := v1alpha1.ResourceStatus{
		Kind:       ClusterDnsKind,
		Identifier: fmt.Sprintf("%s/%s", dnsNamespace, dnsService),
	}

	deployment, err := d.findDnsDeployment(ctx, dnsNamespace, dnsService)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		logrusField.Warningf("locate cluster DNS deployment for restore failed: %v", err)
		status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
		return spec.ReturnResultIgnoreCode(
			v1alpha1.CreateFailExperimentStatus(err.Error(), []v1alpha1.ResourceStatus{status}),
		)
	}

	status.Identifier = fmt.Sprintf("%s/%s/%s", dnsNamespace, dnsService, deployment.Name)

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &appsv1.Deployment{}
		if err := d.client.Get(ctx, types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}, latest); err != nil {
			return err
		}
		return d.restoreClusterDnsDeployment(ctx, latest, experimentId)
	}); err != nil {
		logrusField.Warningf("restore cluster DNS deployment %s/%s failed: %v", deployment.Namespace, deployment.Name, err)
		status = status.CreateFailResourceStatus(fmt.Sprintf("restore cluster DNS deployment failed: %v", err), spec.K8sExecFailed.Code)
		return spec.ReturnResultIgnoreCode(
			v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}),
		)
	}

	logrusField.Infof("restored cluster DNS deployment %s/%s", deployment.Namespace, deployment.Name)
	status = status.CreateSuccessResourceStatus()
	status.State = v1alpha1.DestroyedState
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{status}))
}

// findDnsDeployment locates the Deployment that backs the given DNS service.
//
// It first reads the Service to discover its selector, then lists Deployments in
// the same namespace and returns the first one whose pod template labels match
// the Service selector. This works for the standard CoreDNS / kube-dns layout
// where a Service of type ClusterIP fronts a Deployment.
func (d *ClusterDnsFailureActionExecutor) findDnsDeployment(ctx context.Context, namespace, serviceName string) (*appsv1.Deployment, error) {
	svc := &v1.Service{}
	if err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: serviceName}, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("DNS service %s/%s not found", namespace, serviceName)
		}
		return nil, fmt.Errorf("get DNS service %s/%s failed: %v", namespace, serviceName, err)
	}

	if len(svc.Spec.Selector) == 0 {
		return nil, fmt.Errorf("DNS service %s/%s has no selector, cannot reverse-resolve workload", namespace, serviceName)
	}

	selector := pkglabels.SelectorFromSet(svc.Spec.Selector)
	deployments := &appsv1.DeploymentList{}
	if err := d.client.List(ctx, deployments, &sigsclient.ListOptions{Namespace: namespace}); err != nil {
		return nil, fmt.Errorf("list deployments in namespace %s failed: %v", namespace, err)
	}

	for i := range deployments.Items {
		dep := &deployments.Items[i]
		if selector.Matches(pkglabels.Set(dep.Spec.Template.Labels)) {
			return dep, nil
		}
	}
	return nil, fmt.Errorf("no Deployment in namespace %s matches DNS service %s selector %v",
		namespace, serviceName, svc.Spec.Selector)
}

// injectClusterDnsFailure backs up the deployment's replica count and scales it to 0.
// The function is idempotent: if the deployment is already modified by the same
// experiment id, no further changes are made.
func (d *ClusterDnsFailureActionExecutor) injectClusterDnsFailure(ctx context.Context, deployment *appsv1.Deployment, experimentId string) error {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}

	if existingId, ok := deployment.Annotations[ChaosBladeExperimentAnnotation]; ok && existingId != "" && existingId != experimentId {
		return fmt.Errorf("deployment is already modified by another chaosblade experiment: %s", existingId)
	}
	if deployment.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}

	originalReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		originalReplicas = *deployment.Spec.Replicas
	}

	originalBytes, err := json.Marshal(originalReplicas)
	if err != nil {
		return fmt.Errorf("marshal original replicas failed: %v", err)
	}
	deployment.Annotations[ChaosBladeOriginalReplicasAnnotation] = string(originalBytes)
	deployment.Annotations[ChaosBladeClusterDnsAnnotation] = ChaosBladeClusterDnsAction
	deployment.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	zero := int32(0)
	deployment.Spec.Replicas = &zero

	return d.client.Update(ctx, deployment)
}

// restoreClusterDnsDeployment restores the deployment's original replica count.
// If the deployment was not modified by this experiment id, the call is a no-op.
func (d *ClusterDnsFailureActionExecutor) restoreClusterDnsDeployment(ctx context.Context, deployment *appsv1.Deployment, experimentId string) error {
	if deployment.Annotations == nil {
		return nil
	}
	if deployment.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return nil
	}

	originalReplicas, err := readOriginalReplicas(deployment.Annotations[ChaosBladeOriginalReplicasAnnotation])
	if err != nil {
		return err
	}
	deployment.Spec.Replicas = &originalReplicas

	delete(deployment.Annotations, ChaosBladeOriginalReplicasAnnotation)
	delete(deployment.Annotations, ChaosBladeClusterDnsAnnotation)
	delete(deployment.Annotations, ChaosBladeExperimentAnnotation)

	return d.client.Update(ctx, deployment)
}

// readOriginalReplicas decodes the backed-up replica count, supporting both a
// raw integer string (e.g. "2") and a JSON-encoded integer (e.g. "2").
func readOriginalReplicas(raw string) (int32, error) {
	if raw == "" {
		return 1, nil
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return int32(n), nil
	}
	var n int32
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		return 0, fmt.Errorf("decode original replicas %q failed: %v", raw, err)
	}
	return n, nil
}

func resolveDnsServiceFlags(expModel *spec.ExpModel) (string, string) {
	dnsService := expModel.ActionFlags[DnsServiceFlag]
	if dnsService == "" {
		dnsService = DefaultDnsServiceName
	}
	dnsNamespace := expModel.ActionFlags[DnsServiceNamespaceFlag]
	if dnsNamespace == "" {
		dnsNamespace = DefaultDnsServiceNamespace
	}
	return dnsService, dnsNamespace
}

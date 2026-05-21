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

package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	sigsclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	// DnsFailureNameserverFlag is the optional flag specifying the nameserver IP
	// to be validated against the pod's DNS configuration. When provided, the
	// action requires the pod's resolved nameservers to contain this IP before
	// proceeding with the injection.
	DnsFailureNameserverFlag = "nameserver"

	// ChaosBladePodDnsFailureAction is the marker annotation value for the pod
	// DNS failure action.
	ChaosBladePodDnsFailureAction = "dnsfailure"
	// ChaosBladePodDnsFailureAnnotation marks workloads modified by the pod DNS
	// failure action.
	ChaosBladePodDnsFailureAnnotation = "chaosblade.io/pod-dnsfailure"
	// ChaosBladeOriginalDnsPolicyAnnotation stores the workload's original
	// PodSpec.DNSPolicy as a JSON string.
	ChaosBladeOriginalDnsPolicyAnnotation = "chaosblade.io/original-dnspolicy"
	// ChaosBladeOriginalDnsConfigAnnotation stores the workload's original
	// PodSpec.DNSConfig as a JSON string. An empty string means the original
	// DNSConfig was nil (i.e. the pod inherited from DNSPolicy).
	ChaosBladeOriginalDnsConfigAnnotation = "chaosblade.io/original-dnsconfig"

	// UnreachableDnsNameserver is the placeholder nameserver injected to the
	// workload's PodSpec to make all DNS queries originating from the pod fail.
	// 127.0.0.255 is in the loopback /8 and is guaranteed not to listen on UDP/53.
	UnreachableDnsNameserver = "127.0.0.255"
)

// supportedWorkloadKinds is the set of workload kinds that this action can
// inject into. Pods owned by workloads outside this set (e.g. naked pods, Jobs)
// are not supported because their PodSpec is not modifiable in a way that
// survives recreation.
var supportedWorkloadKinds = map[string]struct{}{
	"Deployment":  {},
	"DaemonSet":   {},
	"StatefulSet": {},
}

type PodDnsFailureActionSpec struct {
	spec.BaseExpActionCommandSpec
	client *channel.Client
}

func NewPodDnsFailureActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &PodDnsFailureActionSpec{
		BaseExpActionCommandSpec: spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name:     DnsFailureNameserverFlag,
					Desc:     "Nameserver IP to be validated against the pod's DNS configuration. When set, the pod must reference this nameserver, otherwise the injection fails. Optional",
					Required: false,
				},
			},
			ActionExecutor: &PodDnsFailureActionExecutor{client: client},
			ActionExample: `# Make all DNS queries from a pod fail by injecting an unreachable nameserver to its workload
blade create k8s pod-pod dnsfailure --names nginx-app-xxx --namespace default --kubeconfig ~/.kube/config

# Same as above but require that the pod is currently configured to use 10.96.0.10 as a nameserver
blade create k8s pod-pod dnsfailure --names nginx-app-xxx --namespace default --nameserver 10.96.0.10 --kubeconfig ~/.kube/config

# Inject DNS failure for pods selected by labels
blade create k8s pod-pod dnsfailure --labels app=nginx --namespace default --kubeconfig ~/.kube/config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
		client: client,
	}
}

func (*PodDnsFailureActionSpec) Name() string {
	return "dnsfailure"
}

func (*PodDnsFailureActionSpec) Aliases() []string {
	return []string{}
}

func (*PodDnsFailureActionSpec) ShortDesc() string {
	return "Make pod DNS resolution completely unavailable by injecting an unreachable nameserver"
}

func (*PodDnsFailureActionSpec) LongDesc() string {
	return "Resolve the target pod's DNS configuration (DNSPolicy + DNSConfig and the runtime resolv.conf), " +
		"then inject a complete DNS unavailability fault by overriding the owning workload's PodSpec to use " +
		"an unreachable nameserver. The original DNSPolicy/DNSConfig is backed up to workload annotations and " +
		"restored when the experiment is destroyed. The pod is deleted after injection so the controller " +
		"recreates it with the faulty DNS configuration."
}

type PodDnsFailureActionExecutor struct {
	client *channel.Client
}

func (*PodDnsFailureActionExecutor) Name() string {
	return "dnsfailure"
}

func (*PodDnsFailureActionExecutor) SetChannel(channel spec.Channel) {}

func (d *PodDnsFailureActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *PodDnsFailureActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}

	requiredNameserver := strings.TrimSpace(expModel.ActionFlags[DnsFailureNameserverFlag])

	// Track workloads already injected during this experiment to avoid double
	// processing when multiple matched pods belong to the same workload.
	processedWorkloads := make(map[string]bool)
	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false

	for _, meta := range containerObjectMetaList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: meta.GetIdentifier(),
		}

		pod := &v1.Pod{}
		if err := d.client.Get(ctx, types.NamespacedName{Name: meta.PodName, Namespace: meta.Namespace}, pod); err != nil {
			logrusField.Warningf("get pod %s/%s failed: %v", meta.Namespace, meta.PodName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("get pod failed: %v", err), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		if requiredNameserver != "" {
			if !podUsesNameserver(pod, requiredNameserver) {
				errMsg := fmt.Sprintf("pod %s/%s does not reference nameserver %s in its DNS config",
					meta.Namespace, meta.PodName, requiredNameserver)
				logrusField.Warning(errMsg)
				status = status.CreateFailResourceStatus(errMsg, spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				continue
			}
		}

		ownerKind, ownerName, err := resolveTopLevelWorkload(ctx, d.client, pod)
		if err != nil {
			logrusField.Warningf("resolve workload for pod %s/%s failed: %v", meta.Namespace, meta.PodName, err)
			status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		workloadKey := fmt.Sprintf("%s/%s/%s", pod.Namespace, ownerKind, ownerName)
		if !processedWorkloads[workloadKey] {
			if err := d.injectWorkloadDnsFailure(ctx, pod.Namespace, ownerKind, ownerName, experimentId); err != nil {
				logrusField.Warningf("inject DNS failure to %s %s/%s failed: %v", ownerKind, pod.Namespace, ownerName, err)
				status = status.CreateFailResourceStatus(
					fmt.Sprintf("inject DNS failure to %s %s/%s failed: %v", ownerKind, pod.Namespace, ownerName, err),
					spec.K8sExecFailed.Code,
				)
				statuses = append(statuses, status)
				continue
			}
			processedWorkloads[workloadKey] = true
			logrusField.Infof("injected DNS failure to %s %s/%s for pod %s",
				ownerKind, pod.Namespace, ownerName, meta.PodName)
		}

		// Delete the pod so the controller recreates it with the faulty DNS config.
		if err := d.client.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
			logrusField.Warningf("delete pod %s/%s failed: %v", meta.Namespace, meta.PodName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("delete pod failed: %v", err), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		status = status.CreateSuccessResourceStatus()
		statuses = append(statuses, status)
		success = true
	}

	if success {
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateSuccessExperimentStatus(statuses))
	}
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus("see resStatuses for details", statuses))
}

func (d *PodDnsFailureActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	containerObjectMetaList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}

	processedWorkloads := make(map[string]bool)
	statuses := make([]v1alpha1.ResourceStatus, 0)
	allSuccess := true

	for _, meta := range containerObjectMetaList {
		status := v1alpha1.ResourceStatus{
			Id:         meta.Id,
			Kind:       v1alpha1.PodKind,
			Identifier: meta.GetIdentifier(),
		}

		// During destroy, the pod referenced by the original injection is likely
		// already replaced. Still attempt to look up an owner via any pod that
		// currently has this name in the namespace; if the lookup fails we fall
		// back to using the original ResourceStatus identifier.
		pod := &v1.Pod{}
		if err := d.client.Get(ctx, types.NamespacedName{Name: meta.PodName, Namespace: meta.Namespace}, pod); err != nil {
			if !apierrors.IsNotFound(err) {
				logrusField.Warningf("get pod %s/%s for restore failed: %v", meta.Namespace, meta.PodName, err)
			}
			// If we cannot resolve the owner from the pod, the workload likely
			// still needs to be restored. We try to restore by enumerating
			// supported workloads in the namespace that carry our experiment
			// annotation.
			if err := d.restoreNamespaceWorkloads(ctx, meta.Namespace, experimentId, processedWorkloads); err != nil {
				logrusField.Warningf("restore namespace %s workloads failed: %v", meta.Namespace, err)
				status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				allSuccess = false
				continue
			}
			status = status.CreateSuccessResourceStatus()
			status.State = v1alpha1.DestroyedState
			statuses = append(statuses, status)
			continue
		}

		ownerKind, ownerName, err := resolveTopLevelWorkload(ctx, d.client, pod)
		if err != nil {
			logrusField.Warningf("resolve workload for pod %s/%s during restore failed: %v",
				meta.Namespace, meta.PodName, err)
			if err := d.restoreNamespaceWorkloads(ctx, meta.Namespace, experimentId, processedWorkloads); err != nil {
				logrusField.Warningf("restore namespace %s workloads failed: %v", meta.Namespace, err)
				status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				allSuccess = false
				continue
			}
			status = status.CreateSuccessResourceStatus()
			status.State = v1alpha1.DestroyedState
			statuses = append(statuses, status)
			continue
		}

		workloadKey := fmt.Sprintf("%s/%s/%s", pod.Namespace, ownerKind, ownerName)
		if !processedWorkloads[workloadKey] {
			if err := d.restoreWorkloadDnsFailure(ctx, pod.Namespace, ownerKind, ownerName, experimentId); err != nil {
				logrusField.Warningf("restore DNS failure on %s %s/%s failed: %v",
					ownerKind, pod.Namespace, ownerName, err)
				status = status.CreateFailResourceStatus(err.Error(), spec.K8sExecFailed.Code)
				statuses = append(statuses, status)
				allSuccess = false
				continue
			}
			processedWorkloads[workloadKey] = true
			logrusField.Infof("restored DNS configuration on %s %s/%s", ownerKind, pod.Namespace, ownerName)
		}

		status = status.CreateSuccessResourceStatus()
		status.State = v1alpha1.DestroyedState
		statuses = append(statuses, status)
	}

	if allSuccess {
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus(statuses))
	}
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus("see resStatuses for details", statuses))
}

// injectWorkloadDnsFailure overrides the workload's PodTemplate DNSPolicy/DNSConfig
// with an unreachable nameserver after backing up the original values.
func (d *PodDnsFailureActionExecutor) injectWorkloadDnsFailure(ctx context.Context, namespace, kind, name, experimentId string) error {
	if _, ok := supportedWorkloadKinds[kind]; !ok {
		return fmt.Errorf("unsupported owner kind %s for pod DNS failure (must be Deployment/DaemonSet/StatefulSet)", kind)
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		switch kind {
		case "Deployment":
			latest := &appsv1.Deployment{}
			if err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, latest); err != nil {
				return err
			}
			if latest.Annotations == nil {
				latest.Annotations = make(map[string]string)
			}
			if err := beginPodDnsFailure(latest.Annotations, &latest.Spec.Template.Spec, experimentId); err != nil {
				return err
			}
			return d.client.Update(ctx, latest)
		case "DaemonSet":
			latest := &appsv1.DaemonSet{}
			if err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, latest); err != nil {
				return err
			}
			if latest.Annotations == nil {
				latest.Annotations = make(map[string]string)
			}
			if err := beginPodDnsFailure(latest.Annotations, &latest.Spec.Template.Spec, experimentId); err != nil {
				return err
			}
			return d.client.Update(ctx, latest)
		case "StatefulSet":
			latest := &appsv1.StatefulSet{}
			if err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, latest); err != nil {
				return err
			}
			if latest.Annotations == nil {
				latest.Annotations = make(map[string]string)
			}
			if err := beginPodDnsFailure(latest.Annotations, &latest.Spec.Template.Spec, experimentId); err != nil {
				return err
			}
			return d.client.Update(ctx, latest)
		}
		return fmt.Errorf("unhandled workload kind %s", kind)
	})
}

func (d *PodDnsFailureActionExecutor) restoreWorkloadDnsFailure(ctx context.Context, namespace, kind, name, experimentId string) error {
	if _, ok := supportedWorkloadKinds[kind]; !ok {
		return nil
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		switch kind {
		case "Deployment":
			latest := &appsv1.Deployment{}
			if err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, latest); err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}
			if !endPodDnsFailure(latest.Annotations, &latest.Spec.Template.Spec, experimentId) {
				return nil
			}
			return d.client.Update(ctx, latest)
		case "DaemonSet":
			latest := &appsv1.DaemonSet{}
			if err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, latest); err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}
			if !endPodDnsFailure(latest.Annotations, &latest.Spec.Template.Spec, experimentId) {
				return nil
			}
			return d.client.Update(ctx, latest)
		case "StatefulSet":
			latest := &appsv1.StatefulSet{}
			if err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, latest); err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}
			if !endPodDnsFailure(latest.Annotations, &latest.Spec.Template.Spec, experimentId) {
				return nil
			}
			return d.client.Update(ctx, latest)
		}
		return fmt.Errorf("unhandled workload kind %s", kind)
	})
}

// restoreNamespaceWorkloads enumerates supported workloads in the given namespace
// and restores any that carry this experiment's annotation. Used as a fallback
// when the originally injected pod cannot be re-located on destroy.
func (d *PodDnsFailureActionExecutor) restoreNamespaceWorkloads(ctx context.Context, namespace, experimentId string, processed map[string]bool) error {
	listOpts := &sigsclient.ListOptions{Namespace: namespace}
	deployList := &appsv1.DeploymentList{}
	if err := d.client.List(ctx, deployList, listOpts); err != nil {
		return err
	}
	for i := range deployList.Items {
		dep := &deployList.Items[i]
		if dep.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
			continue
		}
		if dep.Annotations[ChaosBladePodDnsFailureAnnotation] != ChaosBladePodDnsFailureAction {
			continue
		}
		key := fmt.Sprintf("%s/Deployment/%s", namespace, dep.Name)
		if processed[key] {
			continue
		}
		if err := d.restoreWorkloadDnsFailure(ctx, namespace, "Deployment", dep.Name, experimentId); err != nil {
			return err
		}
		processed[key] = true
	}

	dsList := &appsv1.DaemonSetList{}
	if err := d.client.List(ctx, dsList, listOpts); err != nil {
		return err
	}
	for i := range dsList.Items {
		ds := &dsList.Items[i]
		if ds.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
			continue
		}
		if ds.Annotations[ChaosBladePodDnsFailureAnnotation] != ChaosBladePodDnsFailureAction {
			continue
		}
		key := fmt.Sprintf("%s/DaemonSet/%s", namespace, ds.Name)
		if processed[key] {
			continue
		}
		if err := d.restoreWorkloadDnsFailure(ctx, namespace, "DaemonSet", ds.Name, experimentId); err != nil {
			return err
		}
		processed[key] = true
	}

	stsList := &appsv1.StatefulSetList{}
	if err := d.client.List(ctx, stsList, listOpts); err != nil {
		return err
	}
	for i := range stsList.Items {
		sts := &stsList.Items[i]
		if sts.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
			continue
		}
		if sts.Annotations[ChaosBladePodDnsFailureAnnotation] != ChaosBladePodDnsFailureAction {
			continue
		}
		key := fmt.Sprintf("%s/StatefulSet/%s", namespace, sts.Name)
		if processed[key] {
			continue
		}
		if err := d.restoreWorkloadDnsFailure(ctx, namespace, "StatefulSet", sts.Name, experimentId); err != nil {
			return err
		}
		processed[key] = true
	}
	return nil
}

// beginPodDnsFailure is shared logic for all workload kinds: it checks for
// annotation conflicts, backs up the existing DNSPolicy/DNSConfig, then
// overrides the pod template DNS settings with an unreachable nameserver.
// Returns nil on success or when the annotations indicate the workload is
// already injected by the same experiment (idempotent).
func beginPodDnsFailure(annotations map[string]string, podSpec *v1.PodSpec, experimentId string) error {
	if existingId, ok := annotations[ChaosBladeExperimentAnnotation]; ok && existingId != "" && existingId != experimentId {
		return fmt.Errorf("workload is already modified by another chaosblade experiment: %s", existingId)
	}
	if annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}

	originalPolicyBytes, err := json.Marshal(podSpec.DNSPolicy)
	if err != nil {
		return fmt.Errorf("marshal original DNSPolicy failed: %v", err)
	}
	annotations[ChaosBladeOriginalDnsPolicyAnnotation] = string(originalPolicyBytes)

	if podSpec.DNSConfig != nil {
		originalCfgBytes, err := json.Marshal(podSpec.DNSConfig)
		if err != nil {
			return fmt.Errorf("marshal original DNSConfig failed: %v", err)
		}
		annotations[ChaosBladeOriginalDnsConfigAnnotation] = string(originalCfgBytes)
	} else {
		annotations[ChaosBladeOriginalDnsConfigAnnotation] = ""
	}

	annotations[ChaosBladePodDnsFailureAnnotation] = ChaosBladePodDnsFailureAction
	annotations[ChaosBladeExperimentAnnotation] = experimentId

	podSpec.DNSPolicy = v1.DNSNone
	podSpec.DNSConfig = &v1.PodDNSConfig{
		Nameservers: []string{UnreachableDnsNameserver},
	}
	return nil
}

// endPodDnsFailure restores the workload's DNS configuration. It returns true
// when an actual mutation was performed (so the caller should issue an Update).
func endPodDnsFailure(annotations map[string]string, podSpec *v1.PodSpec, experimentId string) bool {
	if annotations == nil {
		return false
	}
	if annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return false
	}

	if raw, ok := annotations[ChaosBladeOriginalDnsPolicyAnnotation]; ok {
		var original v1.DNSPolicy
		if err := json.Unmarshal([]byte(raw), &original); err == nil {
			podSpec.DNSPolicy = original
		}
	}

	if raw, ok := annotations[ChaosBladeOriginalDnsConfigAnnotation]; ok {
		if raw == "" {
			podSpec.DNSConfig = nil
		} else {
			var original v1.PodDNSConfig
			if err := json.Unmarshal([]byte(raw), &original); err == nil {
				podSpec.DNSConfig = &original
			}
		}
	}

	delete(annotations, ChaosBladeOriginalDnsPolicyAnnotation)
	delete(annotations, ChaosBladeOriginalDnsConfigAnnotation)
	delete(annotations, ChaosBladePodDnsFailureAnnotation)
	delete(annotations, ChaosBladeExperimentAnnotation)
	return true
}

// resolveTopLevelWorkload walks owner references upwards starting from a Pod
// and returns the kind/name of the top-level workload (Deployment / DaemonSet
// / StatefulSet). Returns an error if the pod has no supported owner.
func resolveTopLevelWorkload(ctx context.Context, client *channel.Client, pod *v1.Pod) (string, string, error) {
	owner := metav1.GetControllerOf(&pod.ObjectMeta)
	if owner == nil {
		return "", "", fmt.Errorf("pod %s/%s has no controller owner", pod.Namespace, pod.Name)
	}

	switch owner.Kind {
	case "ReplicaSet":
		rs := &appsv1.ReplicaSet{}
		if err := client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: owner.Name}, rs); err != nil {
			return "", "", fmt.Errorf("get replicaset %s/%s failed: %v", pod.Namespace, owner.Name, err)
		}
		rsOwner := metav1.GetControllerOf(&rs.ObjectMeta)
		if rsOwner == nil || rsOwner.Kind != "Deployment" {
			return "", "", fmt.Errorf("replicaset %s/%s is not owned by a Deployment", pod.Namespace, owner.Name)
		}
		return "Deployment", rsOwner.Name, nil
	case "DaemonSet":
		return "DaemonSet", owner.Name, nil
	case "StatefulSet":
		return "StatefulSet", owner.Name, nil
	default:
		return "", "", fmt.Errorf("unsupported pod owner kind %s for pod %s/%s",
			owner.Kind, pod.Namespace, pod.Name)
	}
}

// podUsesNameserver returns true when the pod's effective DNS configuration
// contains the given nameserver IP. The check is best-effort: if DNSConfig is
// not explicitly set, the pod inherits cluster DNS at runtime, so we cannot
// validate from the spec alone and we treat the precondition as satisfied.
func podUsesNameserver(pod *v1.Pod, nameserver string) bool {
	if pod.Spec.DNSConfig != nil {
		for _, ns := range pod.Spec.DNSConfig.Nameservers {
			if ns == nameserver {
				return true
			}
		}
		// HostNetwork pods with explicit DNSConfig require the nameserver to be listed.
		if pod.Spec.DNSPolicy == v1.DNSNone {
			return false
		}
	}
	// When DNSConfig is empty the pod uses ClusterFirst / ClusterFirstWithHostNet
	// or Default, which means the cluster DNS service is in use. Without runtime
	// access to /etc/resolv.conf we accept the nameserver flag as a hint.
	return true
}

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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

const (
	BadResourceSizeCPUFlag = "cpu"
	BadResourceSizeMemFlag = "mem"

	ChaosBladeOriginalResourcesAnnotation = "chaosblade.io/original-resources"
	ChaosBladeBadResourceSizeAction       = "badresourcesize"
)

type BadResourceSizeActionSpec struct {
	spec.BaseExpActionCommandSpec
	client *channel.Client
}

func NewBadResourceSizeActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &BadResourceSizeActionSpec{
		BaseExpActionCommandSpec: spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name:     "workload-type",
					Desc:     "Workload type: deployment, daemonset, statefulset. Default: deployment",
					Required: false,
					Default:  "deployment",
				},
				&spec.ExpFlag{
					Name:     "workload-name",
					Desc:     "Workload name to modify resource size",
					Required: true,
				},
				&spec.ExpFlag{
					Name:     BadResourceSizeCPUFlag,
					Desc:     "CPU resource limit to set, e.g. 1m, 5m",
					Required: false,
				},
				&spec.ExpFlag{
					Name:     BadResourceSizeMemFlag,
					Desc:     "Memory resource limit to set, e.g. 128m, 256m",
					Required: false,
				},
			},
			ActionExecutor: &BadResourceSizeActionExecutor{client: client},
			ActionExample: `# Set CPU resource limit for a deployment
blade create k8s pod-pod badresourcesize --namespace default --workload-type deployment --workload-name nginx-app --cpu 1m --kubeconfig ~/.kube/config

# Set memory resource limit for a deployment
blade create k8s pod-pod badresourcesize --namespace default --workload-type deployment --workload-name nginx-app --mem 128m --kubeconfig ~/.kube/config

# Set both CPU and memory resource limits for a deployment
blade create k8s pod-pod badresourcesize --namespace default --workload-type deployment --workload-name nginx-app --cpu 1m --mem 128m --kubeconfig ~/.kube/config

# Set resource limits for a statefulset
blade create k8s pod-pod badresourcesize --namespace default --workload-type statefulset --workload-name redis-app --cpu 1m --mem 128m --kubeconfig ~/.kube/config

# Set resource limits for a daemonset
blade create k8s pod-pod badresourcesize --namespace default --workload-type daemonset --workload-name fluentd --cpu 1m --mem 128m --kubeconfig ~/.kube/config
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
		client: client,
	}
}

func (*BadResourceSizeActionSpec) Name() string {
	return "badresourcesize"
}

func (*BadResourceSizeActionSpec) Aliases() []string {
	return []string{}
}

func (*BadResourceSizeActionSpec) ShortDesc() string {
	return "Modify workload pod resource limits to simulate bad resource sizing"
}

func (*BadResourceSizeActionSpec) LongDesc() string {
	return "Modify the CPU/Memory resource limits of a workload (Deployment/DaemonSet/StatefulSet) " +
		"to simulate incorrect resource sizing. The original resource configuration is backed up " +
		"in an annotation and restored when the experiment is destroyed. " +
		"Existing container-level and pod-level resource settings are removed, " +
		"and new pod-level resource limits are applied."
}

// PreCreate implements model.ActionPreProcessor.
func (a *BadResourceSizeActionSpec) PreCreate(ctx context.Context, expModel *spec.ExpModel, client *channel.Client) (context.Context, *spec.Response) {
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]

	if namespace == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if strings.Contains(namespace, ",") {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterInvalidNSNotOne, model.ResourceNamespaceFlag.Name)
	}
	if workloadName == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, "workload-name")
	}

	cpuVal := expModel.ActionFlags[BadResourceSizeCPUFlag]
	memVal := expModel.ActionFlags[BadResourceSizeMemFlag]
	if cpuVal == "" && memVal == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, "cpu or mem (at least one is required)")
	}

	containerObjectMetaList := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Namespace: namespace,
			PodName:   fmt.Sprintf("chaosblade-brs-%s-%s", workloadType, workloadName),
		},
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}

// PreDestroy implements model.ActionPreProcessor.
func (a *BadResourceSizeActionSpec) PreDestroy(ctx context.Context, expModel *spec.ExpModel, client *channel.Client, oldExpStatus v1alpha1.ExperimentStatus) (context.Context, *spec.Response) {
	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]

	if namespace == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if strings.Contains(namespace, ",") {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterInvalidNSNotOne, model.ResourceNamespaceFlag.Name)
	}
	if workloadName == "" {
		return ctx, spec.ResponseFailWithFlags(spec.ParameterLess, "workload-name")
	}

	containerObjectMetaList := model.ContainerMatchedList{
		model.ContainerObjectMeta{
			Namespace: namespace,
			PodName:   fmt.Sprintf("chaosblade-brs-%s-%s", workloadType, workloadName),
		},
	}

	ctx = model.SetContainerObjectMetaListToContext(ctx, containerObjectMetaList)
	return ctx, nil
}

type BadResourceSizeActionExecutor struct {
	client *channel.Client
}

func (*BadResourceSizeActionExecutor) Name() string {
	return "badresourcesize"
}

func (*BadResourceSizeActionExecutor) SetChannel(channel spec.Channel) {}

func (d *BadResourceSizeActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(uid, ctx, expModel)
	}
	return d.create(uid, ctx, expModel)
}

func (d *BadResourceSizeActionExecutor) create(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]
	cpuVal := expModel.ActionFlags[BadResourceSizeCPUFlag]
	memVal := expModel.ActionFlags[BadResourceSizeMemFlag]

	if namespace == "" {
		util.Errorf(uid, util.GetRunFuncName(), "namespace is required")
		return spec.ResponseFailWithFlags(spec.ParameterLess, model.ResourceNamespaceFlag.Name)
	}
	if workloadName == "" {
		util.Errorf(uid, util.GetRunFuncName(), "workload-name is required")
		return spec.ResponseFailWithFlags(spec.ParameterLess, "workload-name")
	}
	if cpuVal == "" && memVal == "" {
		util.Errorf(uid, util.GetRunFuncName(), "at least one of cpu or mem is required")
		return spec.ResponseFailWithFlags(spec.ParameterLess, "cpu or mem")
	}

	newLimits, err := buildResourceLimits(cpuVal, memVal)
	if err != nil {
		util.Errorf(uid, util.GetRunFuncName(), fmt.Sprintf("parse resource values failed: %v", err))
		return spec.ResponseFailWithFlags(spec.ParameterIllegal, "cpu/mem", err.Error())
	}

	status := v1alpha1.ResourceStatus{
		Kind:       v1alpha1.PodKind,
		Identifier: fmt.Sprintf("%s//%s//%s", namespace, workloadType, workloadName),
	}

	switch workloadType {
	case "deployment":
		deployment := &appsv1.Deployment{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, deployment)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Warningf("deployment %s/%s not found", namespace, workloadName)
				status = status.CreateFailResourceStatus(fmt.Sprintf("deployment not found: %v", err), spec.K8sExecFailed.Code)
			} else {
				logrusField.Warningf("get deployment %s/%s failed: %v", namespace, workloadName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("get deployment failed: %v", err), spec.K8sExecFailed.Code)
			}
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.injectDeploymentBadResourceSize(ctx, deployment, newLimits, experimentId); err != nil {
			logrusField.Warningf("inject bad resource size to deployment %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("inject bad resource size failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("injected bad resource size to deployment %s/%s", namespace, workloadName)

	case "daemonset":
		daemonset := &appsv1.DaemonSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, daemonset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Warningf("daemonset %s/%s not found", namespace, workloadName)
				status = status.CreateFailResourceStatus(fmt.Sprintf("daemonset not found: %v", err), spec.K8sExecFailed.Code)
			} else {
				logrusField.Warningf("get daemonset %s/%s failed: %v", namespace, workloadName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("get daemonset failed: %v", err), spec.K8sExecFailed.Code)
			}
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.injectDaemonSetBadResourceSize(ctx, daemonset, newLimits, experimentId); err != nil {
			logrusField.Warningf("inject bad resource size to daemonset %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("inject bad resource size failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("injected bad resource size to daemonset %s/%s", namespace, workloadName)

	case "statefulset":
		statefulset := &appsv1.StatefulSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, statefulset)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logrusField.Warningf("statefulset %s/%s not found", namespace, workloadName)
				status = status.CreateFailResourceStatus(fmt.Sprintf("statefulset not found: %v", err), spec.K8sExecFailed.Code)
			} else {
				logrusField.Warningf("get statefulset %s/%s failed: %v", namespace, workloadName, err)
				status = status.CreateFailResourceStatus(fmt.Sprintf("get statefulset failed: %v", err), spec.K8sExecFailed.Code)
			}
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}

		if err := d.injectStatefulSetBadResourceSize(ctx, statefulset, newLimits, experimentId); err != nil {
			logrusField.Warningf("inject bad resource size to statefulset %s/%s failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("inject bad resource size failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("injected bad resource size to statefulset %s/%s", namespace, workloadName)

	default:
		status = status.CreateFailResourceStatus(fmt.Sprintf("unsupported workload type: %s", workloadType), spec.ParameterIllegal.Code)
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
	}

	status = status.CreateSuccessResourceStatus()
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateSuccessExperimentStatus([]v1alpha1.ResourceStatus{status}))
}

func (d *BadResourceSizeActionExecutor) destroy(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)

	namespace := expModel.ActionFlags[model.ResourceNamespaceFlag.Name]
	workloadType := expModel.ActionFlags["workload-type"]
	if workloadType == "" {
		workloadType = "deployment"
	}
	workloadName := expModel.ActionFlags["workload-name"]

	status := v1alpha1.ResourceStatus{
		Kind:       v1alpha1.PodKind,
		Identifier: fmt.Sprintf("%s//%s//%s", namespace, workloadType, workloadName),
	}

	switch workloadType {
	case "deployment":
		deployment := &appsv1.Deployment{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, deployment)
		if resp, handled := handleGetError(err, namespace, workloadType, workloadName, &status, logrusField); handled {
			return resp
		}
		if err := d.restoreDeploymentResources(ctx, deployment, experimentId); err != nil {
			logrusField.Warningf("restore deployment %s/%s resources failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("restore deployment resources failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("restored deployment %s/%s resources", namespace, workloadName)

	case "daemonset":
		daemonset := &appsv1.DaemonSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, daemonset)
		if resp, handled := handleGetError(err, namespace, workloadType, workloadName, &status, logrusField); handled {
			return resp
		}
		if err := d.restoreDaemonSetResources(ctx, daemonset, experimentId); err != nil {
			logrusField.Warningf("restore daemonset %s/%s resources failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("restore daemonset resources failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("restored daemonset %s/%s resources", namespace, workloadName)

	case "statefulset":
		statefulset := &appsv1.StatefulSet{}
		err := d.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: workloadName}, statefulset)
		if resp, handled := handleGetError(err, namespace, workloadType, workloadName, &status, logrusField); handled {
			return resp
		}
		if err := d.restoreStatefulSetResources(ctx, statefulset, experimentId); err != nil {
			logrusField.Warningf("restore statefulset %s/%s resources failed: %v", namespace, workloadName, err)
			status = status.CreateFailResourceStatus(fmt.Sprintf("restore statefulset resources failed: %v", err), spec.K8sExecFailed.Code)
			return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
		}
		logrusField.Infof("restored statefulset %s/%s resources", namespace, workloadName)

	default:
		status = status.CreateFailResourceStatus(fmt.Sprintf("unsupported workload type: %s", workloadType), spec.ParameterIllegal.Code)
		return spec.ReturnResultIgnoreCode(v1alpha1.CreateFailExperimentStatus(status.Error, []v1alpha1.ResourceStatus{status}))
	}

	status = status.CreateSuccessResourceStatus()
	status.State = v1alpha1.DestroyedState
	return spec.ReturnResultIgnoreCode(v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{status}))
}

// containerResourcesBackup holds the original resource settings for all containers in a pod template,
// keyed by container name for resilience against container additions/removals during the experiment.
type containerResourcesBackup struct {
	// ResourcesByName maps container name to its original ResourceRequirements.
	ResourcesByName map[string]v1.ResourceRequirements `json:"resourcesByName"`
	// InitResourcesByName maps init container name to its original ResourceRequirements.
	InitResourcesByName map[string]v1.ResourceRequirements `json:"initResourcesByName,omitempty"`
}

// buildResourceLimits parses the cpu/mem flag values into a v1.ResourceList for Limits.
func buildResourceLimits(cpuVal, memVal string) (v1.ResourceList, error) {
	limits := v1.ResourceList{}
	if cpuVal != "" {
		q, err := resource.ParseQuantity(cpuVal)
		if err != nil {
			return nil, fmt.Errorf("invalid cpu value %q: %v", cpuVal, err)
		}
		limits[v1.ResourceCPU] = q
	}
	if memVal != "" {
		memQuantity := normalizeMemoryValue(memVal)
		q, err := resource.ParseQuantity(memQuantity)
		if err != nil {
			return nil, fmt.Errorf("invalid mem value %q: %v", memVal, err)
		}
		limits[v1.ResourceMemory] = q
	}
	return limits, nil
}

// normalizeMemoryValue converts shorthand like "128m" to the Kubernetes-compatible "128Mi".
// Kubernetes uses binary suffixes (Ki, Mi, Gi) for memory.
// If the value already uses a standard suffix or is purely numeric, it is returned as-is.
func normalizeMemoryValue(val string) string {
	if len(val) == 0 {
		return val
	}
	lastChar := val[len(val)-1]
	if lastChar == 'm' || lastChar == 'M' {
		prefix := val[:len(val)-1]
		if _, err := resource.ParseQuantity(prefix); err == nil {
			return prefix + "Mi"
		}
	}
	if lastChar == 'g' || lastChar == 'G' {
		prefix := val[:len(val)-1]
		if _, err := resource.ParseQuantity(prefix); err == nil {
			return prefix + "Gi"
		}
	}
	if lastChar == 'k' || lastChar == 'K' {
		prefix := val[:len(val)-1]
		if _, err := resource.ParseQuantity(prefix); err == nil {
			return prefix + "Ki"
		}
	}
	return val
}

// backupAndInjectResources backs up original container resources (keyed by container name)
// for both regular and init containers, then sets new resource limits on each container.
func backupAndInjectResources(podSpec *v1.PodSpec, annotations map[string]string, newLimits v1.ResourceList) error {
	backup := containerResourcesBackup{
		ResourcesByName: make(map[string]v1.ResourceRequirements, len(podSpec.Containers)),
	}
	for _, c := range podSpec.Containers {
		backup.ResourcesByName[c.Name] = *c.Resources.DeepCopy()
	}
	if len(podSpec.InitContainers) > 0 {
		backup.InitResourcesByName = make(map[string]v1.ResourceRequirements, len(podSpec.InitContainers))
		for _, c := range podSpec.InitContainers {
			backup.InitResourcesByName[c.Name] = *c.Resources.DeepCopy()
		}
	}

	backupBytes, err := json.Marshal(backup)
	if err != nil {
		return fmt.Errorf("marshal original resources failed: %v", err)
	}
	annotations[ChaosBladeOriginalResourcesAnnotation] = string(backupBytes)

	for i := range podSpec.Containers {
		podSpec.Containers[i].Resources = v1.ResourceRequirements{
			Limits: newLimits.DeepCopy(),
		}
	}
	for i := range podSpec.InitContainers {
		podSpec.InitContainers[i].Resources = v1.ResourceRequirements{
			Limits: newLimits.DeepCopy(),
		}
	}

	return nil
}

// restoreResources restores original container resources from the backup annotation.
// It uses best-effort matching by container name: containers that exist in both the
// backup and the current spec are restored; new containers (not in backup) are left
// untouched; removed containers (in backup but not in spec) are logged as warnings.
func restoreResources(podSpec *v1.PodSpec, annotations map[string]string) error {
	backupStr, ok := annotations[ChaosBladeOriginalResourcesAnnotation]
	if !ok || backupStr == "" {
		return fmt.Errorf("original resources backup annotation not found")
	}

	var backup containerResourcesBackup
	if err := json.Unmarshal([]byte(backupStr), &backup); err != nil {
		return fmt.Errorf("unmarshal original resources failed: %v", err)
	}

	if len(backup.ResourcesByName) == 0 {
		return fmt.Errorf("backup contains no container resources")
	}

	restored := make(map[string]bool, len(backup.ResourcesByName))
	for i := range podSpec.Containers {
		name := podSpec.Containers[i].Name
		if orig, found := backup.ResourcesByName[name]; found {
			podSpec.Containers[i].Resources = orig
			restored[name] = true
		} else {
			logrus.Warnf("container %q not found in backup, leaving its resources unchanged", name)
		}
	}
	for name := range backup.ResourcesByName {
		if !restored[name] {
			logrus.Warnf("backed-up container %q no longer exists in pod spec, skipping restore", name)
		}
	}

	if len(backup.InitResourcesByName) > 0 {
		restoredInit := make(map[string]bool, len(backup.InitResourcesByName))
		for i := range podSpec.InitContainers {
			name := podSpec.InitContainers[i].Name
			if orig, found := backup.InitResourcesByName[name]; found {
				podSpec.InitContainers[i].Resources = orig
				restoredInit[name] = true
			} else {
				logrus.Warnf("init container %q not found in backup, leaving its resources unchanged", name)
			}
		}
		for name := range backup.InitResourcesByName {
			if !restoredInit[name] {
				logrus.Warnf("backed-up init container %q no longer exists in pod spec, skipping restore", name)
			}
		}
	}

	return nil
}

func (d *BadResourceSizeActionExecutor) injectDeploymentBadResourceSize(ctx context.Context, deployment *appsv1.Deployment, newLimits v1.ResourceList, experimentId string) error {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	if err := ensureNoConflictingExperiment(deployment.Annotations, experimentId); err != nil {
		return err
	}
	if deployment.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}
	deployment.Annotations[ChaosBladeDeploymentAnnotation] = ChaosBladeBadResourceSizeAction
	deployment.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	if err := backupAndInjectResources(&deployment.Spec.Template.Spec, deployment.Annotations, newLimits); err != nil {
		return err
	}

	return d.client.Update(ctx, deployment)
}

func (d *BadResourceSizeActionExecutor) injectDaemonSetBadResourceSize(ctx context.Context, daemonset *appsv1.DaemonSet, newLimits v1.ResourceList, experimentId string) error {
	if daemonset.Annotations == nil {
		daemonset.Annotations = make(map[string]string)
	}
	if err := ensureNoConflictingExperiment(daemonset.Annotations, experimentId); err != nil {
		return err
	}
	if daemonset.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}
	daemonset.Annotations[ChaosBladeDaemonSetAnnotation] = ChaosBladeBadResourceSizeAction
	daemonset.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	if err := backupAndInjectResources(&daemonset.Spec.Template.Spec, daemonset.Annotations, newLimits); err != nil {
		return err
	}

	return d.client.Update(ctx, daemonset)
}

func (d *BadResourceSizeActionExecutor) injectStatefulSetBadResourceSize(ctx context.Context, statefulset *appsv1.StatefulSet, newLimits v1.ResourceList, experimentId string) error {
	if statefulset.Annotations == nil {
		statefulset.Annotations = make(map[string]string)
	}
	if err := ensureNoConflictingExperiment(statefulset.Annotations, experimentId); err != nil {
		return err
	}
	if statefulset.Annotations[ChaosBladeExperimentAnnotation] == experimentId {
		return nil
	}
	statefulset.Annotations[ChaosBladeStatefulSetAnnotation] = ChaosBladeBadResourceSizeAction
	statefulset.Annotations[ChaosBladeExperimentAnnotation] = experimentId

	if err := backupAndInjectResources(&statefulset.Spec.Template.Spec, statefulset.Annotations, newLimits); err != nil {
		return err
	}

	return d.client.Update(ctx, statefulset)
}

func (d *BadResourceSizeActionExecutor) restoreDeploymentResources(ctx context.Context, deployment *appsv1.Deployment, experimentId string) error {
	if deployment.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return fmt.Errorf("deployment was not modified by experiment %s", experimentId)
	}

	if err := restoreResources(&deployment.Spec.Template.Spec, deployment.Annotations); err != nil {
		return err
	}

	delete(deployment.Annotations, ChaosBladeDeploymentAnnotation)
	delete(deployment.Annotations, ChaosBladeExperimentAnnotation)
	delete(deployment.Annotations, ChaosBladeOriginalResourcesAnnotation)

	return d.client.Update(ctx, deployment)
}

func (d *BadResourceSizeActionExecutor) restoreDaemonSetResources(ctx context.Context, daemonset *appsv1.DaemonSet, experimentId string) error {
	if daemonset.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return fmt.Errorf("daemonset was not modified by experiment %s", experimentId)
	}

	if err := restoreResources(&daemonset.Spec.Template.Spec, daemonset.Annotations); err != nil {
		return err
	}

	delete(daemonset.Annotations, ChaosBladeDaemonSetAnnotation)
	delete(daemonset.Annotations, ChaosBladeExperimentAnnotation)
	delete(daemonset.Annotations, ChaosBladeOriginalResourcesAnnotation)

	return d.client.Update(ctx, daemonset)
}

func (d *BadResourceSizeActionExecutor) restoreStatefulSetResources(ctx context.Context, statefulset *appsv1.StatefulSet, experimentId string) error {
	if statefulset.Annotations[ChaosBladeExperimentAnnotation] != experimentId {
		return fmt.Errorf("statefulset was not modified by experiment %s", experimentId)
	}

	if err := restoreResources(&statefulset.Spec.Template.Spec, statefulset.Annotations); err != nil {
		return err
	}

	delete(statefulset.Annotations, ChaosBladeStatefulSetAnnotation)
	delete(statefulset.Annotations, ChaosBladeExperimentAnnotation)
	delete(statefulset.Annotations, ChaosBladeOriginalResourcesAnnotation)

	return d.client.Update(ctx, statefulset)
}

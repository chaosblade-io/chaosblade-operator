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

package chaosblade

import (
	"context"
	"fmt"

	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
)

// Deploy the chaosblade tool with daemonset mode
func deployChaosBladeTool(rcb *ReconcileChaosBlade) error {
	references, err := createOwnerReferences(rcb)
	if err != nil {
		return err
	}
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            chaosblade.DaemonsetPodName,
			Namespace:       chaosblade.DaemonsetPodNamespace,
			Labels:          chaosblade.DaemonsetPodLabels,
			OwnerReferences: references,
		},
		Spec: createDaemonsetSpec(),
	}

	if err := rcb.client.Create(context.TODO(), daemonSet); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logrus.Info("chaosblade tool exits, skip to deploy")
			return nil
		}
		return err
	}
	return nil
}

func createOwnerReferences(rcb *ReconcileChaosBlade) ([]metav1.OwnerReference, error) {
	// get chaosblade operator deployment object
	// Using a unstructured object.
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})
	namespace, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return nil, err
	}
	err = rcb.client.Get(context.TODO(), types.NamespacedName{
		Namespace: namespace,
		Name:      "chaosblade-operator",
	}, u)
	if err != nil {
		logrus.WithError(err).Error("cannot get chaosblade-operator deployment from apps/v1")
		return nil, err
	}
	trueVar := true
	return []metav1.OwnerReference{
		{
			APIVersion: u.GetAPIVersion(),
			Kind:       u.GetKind(),
			Name:       u.GetName(),
			UID:        u.GetUID(),
			Controller: &trueVar,
		},
	}, nil
}

// createDaemonsetSpec
func createDaemonsetSpec() appsv1.DaemonSetSpec {
	return appsv1.DaemonSetSpec{
		Selector:        &metav1.LabelSelector{MatchLabels: chaosblade.DaemonsetPodLabels},
		Template:        createPodTemplateSpec(),
		MinReadySeconds: 5,
		UpdateStrategy:  appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
	}
}

// createPodTemplateSpec
func createPodTemplateSpec() corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   chaosblade.DaemonsetPodName,
			Labels: chaosblade.DaemonsetPodLabels,
		},
		Spec: createPodSpec(),
	}
}

func createPodSpec() corev1.PodSpec {
	pathType := corev1.HostPathFileOrCreate
	periodSeconds := int64(30)
	return corev1.PodSpec{
		Containers:                    []corev1.Container{createContainer()},
		Affinity:                      createAffinity(),
		DNSPolicy:                     corev1.DNSClusterFirstWithHostNet,
		HostNetwork:                   true,
		HostPID:                       true,
		Tolerations:                   []corev1.Toleration{{Effect: corev1.TaintEffectNoSchedule, Operator: corev1.TolerationOpExists}},
		TerminationGracePeriodSeconds: &periodSeconds,
		SchedulerName:                 corev1.DefaultSchedulerName,
		RestartPolicy:                 corev1.RestartPolicyAlways,
		Volumes: []corev1.Volume{
			{
				Name:         "docker-socket",
				VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/docker.sock"}},
			},
			{
				Name: "chaosblade-db-volume",
				VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/chaosblade.dat",
					Type: &pathType,
				}},
			},
			{
				Name:         "hosts",
				VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/etc/hosts"}},
			},
		},
	}
}

func createAffinity() *corev1.Affinity {
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "type",
							Operator: corev1.NodeSelectorOpNotIn,
							Values:   []string{"virtual-kubelet"},
						}},
					},
				},
			},
		},
	}
}

func createContainer() corev1.Container {
	trueVar := true
	return corev1.Container{
		Name:            chaosblade.DaemonsetPodName,
		Image:           fmt.Sprintf("%s:%s", chaosblade.Constant.ImageRepoFunc(), chaosblade.Version),
		ImagePullPolicy: corev1.PullPolicy(chaosblade.PullPolicy),
		VolumeMounts: []corev1.VolumeMount{
			{Name: "docker-socket", MountPath: "/var/run/docker.sock"},
			{Name: "chaosblade-db-volume", MountPath: "/opt/chaosblade/chaosblade.dat"},
			{Name: "hosts", MountPath: "/etc/hosts"},
		},
		SecurityContext: &corev1.SecurityContext{Privileged: &trueVar},
	}
}

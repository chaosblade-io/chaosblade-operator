/*
 * Copyright 1999-2019 Alibaba Group Holding Ltd.
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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/meta"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

// Deploy the chaosblade tool with daemonset mode
func deployChaosBladeAgent(rcb *ReconcileChaosBlade, cb *v1alpha1.ChaosBlade) error {
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      meta.Constant.PodName,
			Namespace: meta.GetNamespace(),
			Labels:    meta.Constant.PodLabels,
		},
		Spec: createDaemonsetSpec(),
	}

	if err := rcb.client.Create(context.TODO(), daemonSet); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// createDaemonsetSpec
func createDaemonsetSpec() appsv1.DaemonSetSpec {
	return appsv1.DaemonSetSpec{
		Selector:        &metav1.LabelSelector{MatchLabels: meta.Constant.PodLabels},
		Template:        createPodTemplateSpec(),
		MinReadySeconds: 5,
		UpdateStrategy:  appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
	}
}

// createPodTemplateSpec
func createPodTemplateSpec() corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   meta.Constant.PodName,
			Labels: meta.Constant.PodLabels,
		},
		Spec: createPodSpec(),
	}
}

func createPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
		Containers:  []corev1.Container{createContainer()},
		Affinity:    createAffinity(),
		DNSPolicy:   corev1.DNSClusterFirstWithHostNet,
		HostNetwork: true,
		HostPID:     true,
		Tolerations: []corev1.Toleration{{Effect: corev1.TaintEffectNoSchedule, Operator: corev1.TolerationOpExists}},
		Volumes: []corev1.Volume{
			{
				Name:         "docker-socket",
				VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/docker.sock"}},
			},
		},
	}
}

func createAffinity() *corev1.Affinity {
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key:      "type",
						Operator: corev1.NodeSelectorOpNotIn,
						Values:   []string{"virtual-kubelet"},
					}}},
				},
			},
		},
	}
}

func createContainer() corev1.Container {
	trueVar := true
	return corev1.Container{
		Name:            meta.Constant.PodName,
		Image:           fmt.Sprintf("%s:%s", meta.Constant.ImageRepoFunc(), meta.GetChaosBladeVersion()),
		ImagePullPolicy: corev1.PullPolicy(meta.GetPullImagePolicy()),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "docker-socket",
				MountPath: "/var/run/docker.sock",
			},
		},
		SecurityContext: &corev1.SecurityContext{Privileged: &trueVar},
	}
}

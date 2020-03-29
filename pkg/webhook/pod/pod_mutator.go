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

package pod

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/builder"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

var (
	log            = logf.Log.WithName("webhook_chaosblade")
	FuseServerPort int32
	SidecarImage   string
)

const (
	SidecarName        = "chaosblade-fuse"
	FuseServerPortName = "fuse-port"
)

func AddMutator(server *webhook.Server, mgr manager.Manager) error {
	webhook, err := builder.NewWebhookBuilder().
		Name("admission-webhook.chaosblade.com").
		Mutating().
		Path("/mutating-pods").
		Operations(admissionregistrationv1beta1.Create, admissionregistrationv1beta1.Update).
		ForType(&corev1.Pod{}).
		WithManager(mgr).
		Handlers(&podMutator{}).
		Build()
	if err != nil {
		return err
	}
	server.Register(webhook)
	return nil
}

// podMutator set default values for pod
type podMutator struct {
	client  client.Client
	decoder types.Decoder
}

// Implement admission.Handler so the controller can handle admission request.
var _ admission.Handler = &podMutator{}

func (a *podMutator) Handle(ctx context.Context, req types.Request) types.Response {
	pod := &corev1.Pod{}
	err := a.decoder.Decode(req, pod)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	patchPod := pod.DeepCopy()
	err = a.mutatePodsFn(patchPod)
	if err != nil {
		log.Info("mutate pod failed: %s", err)
		return types.Response{
			Response: &v1beta1.AdmissionResponse{
				Allowed: true,
			},
		}
	}
	return admission.PatchResponse(pod, patchPod)
}

// podMutator set default values for pod
func (a *podMutator) mutatePodsFn(pod *corev1.Pod) error {
	if pod.Annotations == nil {
		return nil
	}
	injectVolumeName, ok := pod.Annotations["chaosblade/inject-volume"]
	if !ok {
		log.Info("pod has no chaosblade/inject-volume annotation")
		return nil
	}
	injectSubPath, ok := pod.Annotations["chaosblade/inject-volume-subpath"]
	if !ok {
		log.Info("pod has no chaosblade/inject-volume annotation")
		return nil
	}

	for _, container := range pod.Spec.Containers {
		if container.Name == SidecarName {
			log.Info("sidecar has been injected")
			return nil
		}
	}

	var targetVolumeMount corev1.VolumeMount
	//inject sidecar for the first container
	for _, volumeMount := range pod.Spec.Containers[0].VolumeMounts {
		if volumeMount.Name == injectVolumeName {
			if volumeMount.MountPropagation == nil {
				return fmt.Errorf("target volume mount propagation must be HostToContainer or Bidirectional")
			}
			if *(volumeMount.MountPropagation) != corev1.MountPropagationHostToContainer &&
				*(volumeMount.MountPropagation) != corev1.MountPropagationBidirectional {
				return fmt.Errorf("target volume mount propagation is not support")
			}
			targetVolumeMount = volumeMount
			mountPropagation := corev1.MountPropagationBidirectional
			targetVolumeMount.MountPropagation = &mountPropagation
		}
	}

	if targetVolumeMount.Name == "" {
		return fmt.Errorf("pod has no volume mount %s", injectVolumeName)
	}

	privileged := true
	runAsUser := int64(0) //root
	sidecar := corev1.Container{
		Name:  SidecarName,
		Image: SidecarImage,
		Command: []string{
			"/opt/chaosblade/bin/chaos_fuse",
		},
		Args: []string{
			fmt.Sprintf("--addr=:%d", FuseServerPort),
			fmt.Sprintf("--mountpoint=%s/%s", targetVolumeMount.MountPath, injectSubPath),
			fmt.Sprintf("--original=%s/fuse-%s", targetVolumeMount.MountPath, injectSubPath),
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          FuseServerPortName,
				ContainerPort: FuseServerPort,
			},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
			RunAsUser:  &runAsUser,
		},
		VolumeMounts: []corev1.VolumeMount{
			targetVolumeMount,
		},
	}
	containers := []corev1.Container{}
	containers = append(containers, sidecar, pod.Spec.Containers[0])
	pod.Spec.Containers = containers
	return nil
}

// podMutator implements inject.Client.
// A client will be automatically injected.
var _ inject.Client = &podMutator{}

// InjectClient injects the client.
func (v *podMutator) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

// podMutator implements inject.Decoder.
// A decoder will be automatically injected.
var _ inject.Decoder = &podMutator{}

// InjectDecoder injects the decoder.
func (v *podMutator) InjectDecoder(d types.Decoder) error {
	v.decoder = d
	return nil
}

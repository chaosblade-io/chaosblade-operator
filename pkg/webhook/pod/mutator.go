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
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/chaosblade-io/chaosblade-operator/pkg/runtime/chaosblade"
	"github.com/chaosblade-io/chaosblade-operator/version"
)

var (
	FuseServerPort int32
	SidecarImage   string
)

const (
	SidecarName        = "chaosblade-fuse"
	FuseServerPortName = "fuse-port"
)

// PodMutator set default values for pod
type Mutator struct {
	client  client.Client
	decoder *admission.Decoder
}

func (v *Mutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}
	err := v.decoder.Decode(req, pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	patchPod := pod.DeepCopy()
	err = v.mutatePodsFn(patchPod)
	if err != nil {
		logrus.WithError(err).Errorln("mutate pod failed")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	originalBytes, err := json.Marshal(pod)
	if err != nil {
		logrus.WithError(err).Errorln("Marshal original pod err")
		return admission.Allowed("")
	}
	expectedBytes, err := json.Marshal(patchPod)
	if err != nil {
		logrus.WithError(err).Errorln("Marshal patched pod err")
	}
	return admission.PatchResponseFromRaw(originalBytes, expectedBytes)
}

// PodMutator set default values for pod
func (v *Mutator) mutatePodsFn(pod *corev1.Pod) error {
	if pod.Annotations == nil {
		return nil
	}
	injectVolumeName, ok := pod.Annotations["chaosblade/inject-volume"]
	if !ok {
		logrus.WithField("name", pod.Name).Infoln("pod has no chaosblade/inject-volume annotation")
		return nil
	}
	injectSubPath, ok := pod.Annotations["chaosblade/inject-volume-subpath"]
	if !ok {
		logrus.WithField("name", pod.Name).Infoln("pod has no chaosblade/inject-volume annotation")
		return nil
	}

	for _, container := range pod.Spec.Containers {
		if container.Name == SidecarName {
			logrus.WithField("name", pod.Name).Infoln("sidecar has been injected")
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
	mountPoint := path.Join(targetVolumeMount.MountPath, injectSubPath)
	original := path.Join(targetVolumeMount.MountPath, fmt.Sprintf("fuse-%s", injectSubPath))
	logrus.WithFields(logrus.Fields{
		"mountPoint": mountPoint,
		"mountPath":  targetVolumeMount.MountPath,
		"podName":    pod.Name,
	}).Infof("Get matched pod")
	if mountPoint == targetVolumeMount.MountPath {
		original = path.Join(path.Dir(targetVolumeMount.MountPath),
			fmt.Sprintf("fuse-%s", path.Base(targetVolumeMount.MountPath)))
	}
	sidecar := corev1.Container{
		Name:            SidecarName,
		Image:           GetSidecarImage(),
		ImagePullPolicy: corev1.PullAlways,
		Command: []string{
			"/opt/chaosblade/bin/chaos_fuse",
		},

		Args: []string{
			fmt.Sprintf("--address=:%d", FuseServerPort),
			fmt.Sprintf("--mountpoint=%s", mountPoint),
			fmt.Sprintf("--original=%s", original),
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

// InjectClient injects the client.
func (v *Mutator) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

// InjectDecoder injects the decoder.
func (v *Mutator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

func GetSidecarImage() string {
	if SidecarImage != "" {
		return SidecarImage
	}
	if chaosblade.Constant != nil {
		return fmt.Sprintf("%s:%s", chaosblade.Constant.ImageRepoFunc(), version.Version)
	}
	// Fallback for testing when chaosblade.Constant is not initialized
	return fmt.Sprintf("%s:%s", chaosblade.ImageRepository, version.Version)
}

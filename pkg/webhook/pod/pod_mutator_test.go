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
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_mutatePodsFn(t *testing.T) {
	bidirectional := v1.MountPropagationBidirectional
	//hostToContainer := v1.MountPropagationHostToContainer
	None := v1.MountPropagationNone
	tests := []struct {
		pod *v1.Pod
		err error
	}{
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-0",
					Annotations: map[string]string{
						"chaosblade/inject-volume":         "fuse-test",
						"chaosblade/inject-volume-subpath": "data",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "test-0",
							Image: "test-0",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:             "fuse-test",
									MountPath:        "/data",
									MountPropagation: &bidirectional,
								},
							},
						},
					},
				},
			},
			err: nil,
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-1",
					Annotations: map[string]string{
						"chaosblade/inject-volume":         "fuse-test",
						"chaosblade/inject-volume-subpath": "/data",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "test-1",
							Image: "test-1",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:             "data",
									MountPath:        "/data",
									MountPropagation: &bidirectional,
								},
							},
						},
					},
				},
			},
			err: fmt.Errorf("pod has no volume mount fuse-test"),
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-2",
					Annotations: map[string]string{
						"chaosblade/inject-volume":         "data",
						"chaosblade/inject-volume-subpath": "/data",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "test-2",
							Image: "test-2",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
						},
					},
				},
			},
			err: fmt.Errorf("target volume mount propagation must be HostToContainer or Bidirectional"),
		},
		{
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-3",
					Annotations: map[string]string{
						"chaosblade/inject-volume":         "data",
						"chaosblade/inject-volume-subpath": "/data",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "test-3",
							Image: "test-3",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:             "data",
									MountPath:        "/data",
									MountPropagation: &None,
								},
							},
						},
					},
				},
			},
			err: fmt.Errorf("target volume mount propagation is not support"),
		},
	}

	mutator := &PodMutator{}
	for _, test := range tests {
		err := mutator.mutatePodsFn(test.pod)
		if err != nil && err.Error() != test.err.Error() {
			t.Errorf("unexpected result %v, expected result: %v", err, test.err)
		}
	}
}

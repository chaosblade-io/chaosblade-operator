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
	"fmt"
	"strings"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/chaosblade-io/chaosblade-operator/channel"
	"github.com/chaosblade-io/chaosblade-operator/exec/model"
	"github.com/chaosblade-io/chaosblade-operator/pkg/apis/chaosblade/v1alpha1"
)

type ImageConfigActionSpec struct {
	spec.BaseExpActionCommandSpec
}

const (
	ImageNameFlag = "image-name"
	ImageTagFlag  = "image-tag"
)

func NewImageConfigActionSpec(client *channel.Client) spec.ExpActionCommandSpec {
	return &ImageConfigActionSpec{
		spec.BaseExpActionCommandSpec{
			ActionMatchers: []spec.ExpFlagSpec{},
			ActionFlags: []spec.ExpFlagSpec{
				&spec.ExpFlag{
					Name: ImageNameFlag,
					Desc: "The image name to replace the original image name, e.g. nginx-not-exist",
				},
				&spec.ExpFlag{
					Name: ImageTagFlag,
					Desc: "The image tag to replace the original image tag, e.g. non-existent-tag",
				},
			},
			ActionExecutor: &ImageConfigActionExecutor{client: client},
			ActionExample: `# Inject image config error to pods with default behavior
blade create k8s pod-pod imageconfig --labels "app=test" --namespace default

# Inject image config error with custom image name
blade create k8s pod-pod imageconfig --labels "app=test" --namespace default --image-name nginx-not-exist

# Inject image config error with custom image tag
blade create k8s pod-pod imageconfig --labels "app=test" --namespace default --image-tag non-existent-tag

# Inject image config error with both custom image name and tag
blade create k8s pod-pod imageconfig --labels "app=test" --namespace default --image-name nginx-not-exist --image-tag non-existent-tag
`,
			ActionCategories: []string{model.CategorySystemContainer},
		},
	}
}

func (*ImageConfigActionSpec) Name() string {
	return "imageconfig"
}

func (*ImageConfigActionSpec) Aliases() []string {
	return []string{}
}

func (*ImageConfigActionSpec) ShortDesc() string {
	return "Inject image config error to pods"
}

func (*ImageConfigActionSpec) LongDesc() string {
	return "Modify pod container image to a non-existent image to simulate image config error"
}

type ImageConfigActionExecutor struct {
	client *channel.Client
}

func (*ImageConfigActionExecutor) Name() string {
	return "imageconfig"
}

func (*ImageConfigActionExecutor) SetChannel(channel spec.Channel) {}

func (d *ImageConfigActionExecutor) Exec(uid string, ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	if _, ok := spec.IsDestroy(ctx); ok {
		return d.destroy(ctx, expModel)
	}
	return d.create(ctx, expModel)
}

func (d *ImageConfigActionExecutor) create(ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	imageName := expModel.ActionFlags[ImageNameFlag]
	imageTag := expModel.ActionFlags[ImageTagFlag]
	experimentId := model.GetExperimentIdFromContext(ctx)
	logrusField := logrus.WithField("experiment", experimentId)
	containerMatchedList, err := model.GetContainerObjectMetaListFromContext(ctx)
	if err != nil {
		util.Errorf(experimentId, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}
	statuses := make([]v1alpha1.ResourceStatus, 0)
	success := false
	for _, c := range containerMatchedList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: c.GetIdentifier(),
		}
		objectMeta := types.NamespacedName{Name: c.PodName, Namespace: c.Namespace}
		pod := &v1.Pod{}
		err := d.client.Get(ctx, objectMeta, pod)
		if err != nil {
			logrusField.Errorf("get pod %s err, %v", c.PodName, err)
			status = status.CreateFailResourceStatus(spec.K8sExecFailed.Sprintf("get", err), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		if !isImageConfigPodReady(pod) {
			logrusField.Infof("pod %s is not ready", c.PodName)
			statuses = append(statuses, status.CreateFailResourceStatus(spec.PodNotReady.Sprintf(c.PodName),
				spec.PodNotReady.Code))
			continue
		}

		if err := d.modifyPodImage(ctx, pod, imageName, imageTag); err != nil {
			logrusField.Warningf("modify pod %s image err, %v", c.PodName, err)
			status = status.CreateFailResourceStatus(spec.K8sExecFailed.Sprintf("update", err), spec.K8sExecFailed.Code)
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

func (d *ImageConfigActionExecutor) destroy(ctx context.Context, expModel *spec.ExpModel) *spec.Response {
	containerMatchedList, err := model.GetContainerObjectMetaListFromContext(ctx)
	experimentId := model.GetExperimentIdFromContext(ctx)
	if err != nil {
		util.Errorf(experimentId, util.GetRunFuncName(), err.Error())
		return spec.ResponseFailWithResult(spec.ContainerInContextNotFound,
			v1alpha1.CreateFailExperimentStatus(spec.ContainerInContextNotFound.Msg, []v1alpha1.ResourceStatus{}))
	}
	logrusField := logrus.WithField("experiment", experimentId)
	experimentStatus := v1alpha1.CreateDestroyedExperimentStatus([]v1alpha1.ResourceStatus{})
	statuses := experimentStatus.ResStatuses
	for _, c := range containerMatchedList {
		status := v1alpha1.ResourceStatus{
			Kind:       v1alpha1.PodKind,
			Identifier: c.GetIdentifier(),
		}
		objectMeta := types.NamespacedName{Name: c.PodName, Namespace: c.Namespace}
		pod := &v1.Pod{}
		err := d.client.Get(ctx, objectMeta, pod)
		if err != nil {
			logrusField.Errorf("get pod %s err, %v", c.PodName, err)
			status = status.CreateFailResourceStatus(spec.K8sExecFailed.Sprintf("get", err), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}

		err = d.client.Delete(ctx, pod)
		if err != nil {
			logrusField.Errorf("delete pod %s err, %v", c.PodName, err)
			status = status.CreateFailResourceStatus(spec.K8sExecFailed.Sprintf("delete", err), spec.K8sExecFailed.Code)
			statuses = append(statuses, status)
			continue
		}
		status = status.CreateSuccessResourceStatus()
		statuses = append(statuses, status)
	}
	experimentStatus.ResStatuses = statuses
	return spec.ReturnResultIgnoreCode(experimentStatus)
}

// modifyPodImage modifies the pod container images to simulate image config error.
// If imageName and imageTag are both empty, it appends "-image-config-error" to the original image (default behavior).
// If imageName is provided, the image name portion is replaced.
// If imageTag is provided, the image tag portion is replaced.
func (d *ImageConfigActionExecutor) modifyPodImage(ctx context.Context, pod *v1.Pod, imageName, imageTag string) error {
	modified := false
	for i, container := range pod.Spec.Containers {
		key := fmt.Sprintf("%s-%s", "chaosblade.io/imageconfig", container.Name)
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		if isImageConfigAnnotationExist(pod.Annotations, key) {
			continue
		}
		pod.Annotations[key] = container.Image
		pod.Spec.Containers[i].Image = buildNewImage(container.Image, imageName, imageTag)
		modified = true
	}
	if !modified {
		return nil
	}
	return d.client.Update(ctx, pod)
}

// buildNewImage constructs the new image string based on provided imageName and imageTag.
// If both are empty, returns "{original}-image-config-error" for backward compatibility.
// The original image format can be: "name", "name:tag", "registry/name", "registry/name:tag".
func buildNewImage(originalImage, imageName, imageTag string) string {
	if imageName == "" && imageTag == "" {
		return fmt.Sprintf("%s-image-config-error", originalImage)
	}

	// Parse the original image into name and tag parts
	origName, origTag := parseImage(originalImage)

	if imageName != "" {
		origName = imageName
	}
	if imageTag != "" {
		origTag = imageTag
	}

	if origTag == "" {
		return origName
	}
	return fmt.Sprintf("%s:%s", origName, origTag)
}

// parseImage splits an image reference into name and tag.
// Handles formats like "nginx", "nginx:latest", "registry.example.com/nginx:v1.0".
func parseImage(image string) (name, tag string) {
	// Find the last colon that is not part of a registry port (after the last /)
	slashIdx := strings.LastIndex(image, "/")
	colonIdx := strings.LastIndex(image, ":")

	// If colon exists and is after the last slash, it's a tag separator
	if colonIdx > slashIdx {
		return image[:colonIdx], image[colonIdx+1:]
	}
	return image, ""
}

// isImageConfigAnnotationExist checks if the annotation already exists
func isImageConfigAnnotationExist(annotation map[string]string, key string) bool {
	_, ok := annotation[key]
	if !ok {
		return false
	}
	return true
}

// isImageConfigPodReady checks if the pod is ready
func isImageConfigPodReady(pod *v1.Pod) bool {
	if pod.ObjectMeta.DeletionTimestamp != nil {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == v1.PodReady &&
			condition.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

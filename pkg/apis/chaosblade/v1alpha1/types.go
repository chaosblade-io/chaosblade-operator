/*
 * Copyright 1999-2020 Alibaba Group Holding Ltd.
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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ClusterPhase string

const (
	ClusterPhaseInitial     ClusterPhase = ""
	ClusterPhaseInitialized ClusterPhase = "Initialized"
	ClusterPhaseRunning     ClusterPhase = "Running"
	ClusterPhaseUpdating    ClusterPhase = "Updating"
	ClusterPhaseDestroying  ClusterPhase = "Destroying"
	ClusterPhaseDestroyed   ClusterPhase = "Destroyed"
	ClusterPhaseError       ClusterPhase = "Error"
)

// ChaosBladeSpec defines the desired state of ChaosBlade
// +k8s:openapi-gen=true
type ChaosBladeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Experiments []ExperimentSpec `json:"experiments"`
}

type ExperimentSpec struct {
	// Scope is the area of the experiments, currently support node, pod and container
	Scope string `json:"scope"`
	// Target is the experiment target, such as cpu, network
	Target string `json:"target"`
	// Action is the experiment scenario of the target, such as delay, load
	Action string `json:"action"`
	// Desc is the experiment description
	Desc string `json:"desc,omitempty"`
	// Matchers is the experiment rules
	Matchers []FlagSpec `json:"matchers,omitempty"`
}

type FlagSpec struct {
	// Name is the name of flag
	Name string `json:"name"`
	// TODO: Temporarily defined as an array for all flags
	// Value is the value of flag
	Value []string `json:"value"`
}

// ChaosBladeStatus defines the observed state of ChaosBlade
// +k8s:openapi-gen=true
type ChaosBladeStatus struct {
	// Phase indicates the state of the experiment
	//   Initial -> Running -> Updating -> Destroying -> Destroyed
	Phase ClusterPhase `json:"phase,omitempty"`

	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	ExpStatuses []ExperimentStatus `json:"expStatuses"`
}

func (in *ResourceStatus) CreateFailResourceStatus(err string, code int32) ResourceStatus {
	in.State = ErrorState
	in.Error = err
	in.Success = false
	in.Code = code
	return *in
}

func (in *ResourceStatus) CreateSuccessResourceStatus() ResourceStatus {
	in.State = SuccessState
	in.Success = true
	return *in
}

const (
	PodKind       = "pod"
	ContainerKind = "container"
	NodeKind      = "node"
)

type ResourceStatus struct {
	// experiment uid in chaosblade
	Id string `json:"id,omitempty"`
	// experiment state
	State string `json:"state"`
	// experiment code
	Code int32 `json:"code,omitempty"`
	// experiment error
	Error string `json:"error,omitempty"`
	// success
	Success bool `json:"success"`

	// Kind
	Kind string `json:"kind"`
	// Resource identifier, rules as following:
	// container: Namespace/NodeName/PodName/ContainerName
	// pod： Namespace/NodeName/PodName
	Identifier string `json:"identifier,omitempty"`
}

const (
	SuccessState   = "Success"
	ErrorState     = "Error"
	DestroyedState = "Destroyed"
)

func CreateFailExperimentStatus(err string, ResStatuses []ResourceStatus) ExperimentStatus {
	return ExperimentStatus{Success: false, State: ErrorState, Error: err, ResStatuses: ResStatuses}
}

func CreateSuccessExperimentStatus(ResStatuses []ResourceStatus) ExperimentStatus {
	return ExperimentStatus{Success: true, State: SuccessState, ResStatuses: ResStatuses}
}

func CreateDestroyedExperimentStatus(ResStatuses []ResourceStatus) ExperimentStatus {
	return ExperimentStatus{Success: true, State: DestroyedState, ResStatuses: ResStatuses}
}

func CreateFailResStatuses(code int32, err, uid string) []ResourceStatus {
	statuses := make([]ResourceStatus, 0)
	statuses = append(statuses, ResourceStatus{
		Error:   err,
		Code:    code,
		Id:      uid,
		Success: false,
	})
	return statuses
}

type ExperimentStatus struct {
	// experiment scope for cache
	Scope  string `json:"scope"`
	Target string `json:"target"`
	Action string `json:"action"`
	// Success is used to judge the experiment result
	Success bool `json:"success"`
	// State is used to describe the experiment result
	State string `json:"state"`
	Error string `json:"error,omitempty"`
	// ResStatuses is the details of the experiment
	ResStatuses []ResourceStatus `json:"resStatuses,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ChaosBlade is the Schema for the chaosblades API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type ChaosBlade struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChaosBladeSpec   `json:"spec,omitempty"`
	Status ChaosBladeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ChaosBladeList contains a list of ChaosBlade
type ChaosBladeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChaosBlade `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChaosBlade{}, &ChaosBladeList{})
}

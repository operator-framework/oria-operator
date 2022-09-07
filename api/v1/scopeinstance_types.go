/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ScopeInstanceSpec defines the desired state of ScopeInstance
type ScopeInstanceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ScopeInstance. Edit scopeinstance_types.go to remove/update
	ScopeTemplateName string   `json:"scopeTemplateName,omitempty"`
	Namespaces        []string `json:"namespaces,omitempty"`
}

// ScopeInstanceStatus defines the observed state of ScopeInstance
type ScopeInstanceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// TODO(everettraven): Add Condition Types and Reasons as part of the API
const (
	ScopeInstanceSucceededType = "Succeeded"

	ScopeTemplateNotFoundReason         = "ScopeTemplateNotFound"
	RoleBindingDeleteFailureReason      = "RoleBindingDeleteFailure"
	RoleBindingCreateFailureReason      = "RoleBindingCreateFailure"
	ScopeInstanceReconcileSuccessReason = "ScopeInstanceReconcileSuccess"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// ScopeInstance is the Schema for the scopeinstances API
type ScopeInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScopeInstanceSpec   `json:"spec,omitempty"`
	Status ScopeInstanceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ScopeInstanceList contains a list of ScopeInstance
type ScopeInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScopeInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScopeInstance{}, &ScopeInstanceList{})
}

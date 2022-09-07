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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ScopeTemplateSpec defines the desired state of ScopeTemplate
type ScopeTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ScopeTemplate. Edit scopetemplate_types.go to remove/update
	ClusterRoles []ClusterRoleTemplate `json:"clusterRoles,omitempty"`
}

type ClusterRoleTemplate struct {
	GenerateName string              `json:"generateName"`
	Rules        []rbacv1.PolicyRule `json:"rules"`
	Subjects     []rbacv1.Subject    `json:"subjects"`
}

// ScopeTemplateStatus defines the observed state of ScopeTemplate
type ScopeTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// ScopeTemplate is the Schema for the scopetemplates API
type ScopeTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScopeTemplateSpec   `json:"spec,omitempty"`
	Status ScopeTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ScopeTemplateList contains a list of ScopeTemplate
type ScopeTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScopeTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScopeTemplate{}, &ScopeTemplateList{})
}

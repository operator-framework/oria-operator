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

package controllers

import (
	"fmt"

	// . "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorsv1 "awgreene/scope-operator/api/v1alpha1"
)

func verifyRoleBindings(existingRB *rbacv1.RoleBinding, si *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate) {
	// verify cluster role bindings with ownerference, subjects, and role reference.
	Expect(len(existingRB.OwnerReferences)).To(Equal(1))
	Expect(existingRB.OwnerReferences).Should(ContainElement(metav1.OwnerReference{
		APIVersion:         "operators.io.operator-framework/v1alpha1",
		Kind:               "ScopeInstance",
		Name:               si.GetObjectMeta().GetName(),
		UID:                si.GetObjectMeta().GetUID(),
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
	}))

	Expect(len(existingRB.Subjects)).To(Equal(1))
	Expect(existingRB.Subjects).Should(ContainElement(rbacv1.Subject{
		Kind:     "Group",
		Name:     "manager",
		APIGroup: "rbac.authorization.k8s.io",
	}))
	Expect(existingRB.RoleRef).To(Equal(rbacv1.RoleRef{
		Kind:     "ClusterRole",
		Name:     "test",
		APIGroup: "rbac.authorization.k8s.io",
	}))
}

func listRoleBinding(namespace string, numberOfExpectedRoleBindings int, labels map[string]string) *rbacv1.RoleBindingList {
	roleBindingList := &rbacv1.RoleBindingList{}
	Eventually(func() error {
		if err := k8sClient.List(ctx, roleBindingList, &client.ListOptions{
			Namespace: namespace,
		}, client.MatchingLabels(labels)); err != nil {
			return err
		}

		if len(roleBindingList.Items) != numberOfExpectedRoleBindings {
			return fmt.Errorf("Expected %d roleBinding, found %d", numberOfExpectedRoleBindings, len(roleBindingList.Items))
		}

		return nil
	}, timeout, interval).Should(BeNil())

	return roleBindingList
}

func listClusterRole(numberOfExpectedRoleBindings int, labels map[string]string) *rbacv1.ClusterRoleList {
	clusterRoleList := &rbacv1.ClusterRoleList{}
	Eventually(func() error {
		// TODO: scopeTemplate should check for status that no clusterRole exists.
		// Until then, just make sure no clusterRole exists
		if err := k8sClient.List(ctx, clusterRoleList, client.MatchingLabels(labels)); err != nil {
			return err
		}

		if len(clusterRoleList.Items) != numberOfExpectedRoleBindings {
			return fmt.Errorf("Expected %d clusterRoles, found %d", numberOfExpectedRoleBindings, len(clusterRoleList.Items))
		}

		return nil
	}, timeout, interval).Should(BeNil())

	return clusterRoleList
}

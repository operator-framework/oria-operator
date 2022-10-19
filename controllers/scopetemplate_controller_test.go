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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	operatorsv1 "operator-framework/oria-operator/api/v1alpha1"
)

const (
	scopeInstanceName = "scopeinstance-sample"
)

var _ = Describe("ScopeTemplate", func() {
	var (
		namespace     *corev1.Namespace
		scopeTemplate *operatorsv1.ScopeTemplate
		scopeInstance *operatorsv1.ScopeInstance
		// Use this to track the test iteration
		iteration int
	)

	BeforeEach(func() {
		iteration += 1
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, namespace)).NotTo(HaveOccurred())
	})

	When("a scopeTemplate is created that specifies an array of ClusterRoles", func() {
		BeforeEach(func() {
			scopeTemplate = &operatorsv1.ScopeTemplate{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ScopeTemplate",
					APIVersion: "operators.io.operator-framework/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "scopetemplate-sample",
				},
				Spec: operatorsv1.ScopeTemplateSpec{
					ClusterRoles: []operatorsv1.ClusterRoleTemplate{
						{
							// Create a new ClusterRole for each test iteration so that there are no conflicts with a ClusterRole already existing.
							// We can deliberately test this scenario in specific tests in the future.
							GenerateName: fmt.Sprintf("test-%d", iteration),
							Rules: []rbacv1.PolicyRule{
								{
									APIGroups: []string{
										"",
									},
									Verbs: []string{
										"get", "watch", "list",
									},
									Resources: []string{
										"secrets",
									},
								},
							},
							Subjects: []rbacv1.Subject{
								{
									Kind:     "Group",
									APIGroup: "rbac.authorization.k8s.io",
									Name:     "manager",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, scopeTemplate)).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, scopeTemplate)).NotTo(HaveOccurred())
		})

		It("should not create a clusterRole", func() {
			// scope instances are what trigger the creation of the clusterRole
			labels := map[string]string{scopeTemplateUIDKey: string(scopeTemplate.GetUID()),
				clusterRoleGenerateKey: "test"}

			clusterRoleList := listClusterRole(0, labels)
			Expect(clusterRoleList.Items).Should(BeNil())
		})

		When("a scopeInstance is created that references the scopeTemplate", func() {
			BeforeEach(func() {
				scopeInstance = &operatorsv1.ScopeInstance{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ScopeInstance",
						APIVersion: "operators.io.operator-framework/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: scopeInstanceName,
					},
					Spec: operatorsv1.ScopeInstanceSpec{
						ScopeTemplateName: scopeTemplate.GetName(),
						Namespaces:        []string{namespace.GetName()},
					},
				}
				err := k8sClient.Create(ctx, scopeInstance)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, scopeInstance)).NotTo(HaveOccurred())
			})

			It("should create the expected clusterRole", func() {
				labels := map[string]string{scopeTemplateUIDKey: string(scopeTemplate.GetUID()),
					clusterRoleGenerateKey: scopeTemplate.Spec.ClusterRoles[0].GenerateName}

				clusterRoleList := listClusterRole(1, labels)

				existingRole := clusterRoleList.Items[0]

				//TODO: Non-blocking: Let's create functions to check verify the clusterRole and clusterRoleBindings in a followup PR.
				Expect(len(existingRole.OwnerReferences)).Should(Equal(1))
				Expect(existingRole.OwnerReferences).Should(ContainElement(metav1.OwnerReference{
					APIVersion:         "operators.io.operator-framework/v1alpha1",
					Kind:               "ScopeTemplate",
					Name:               scopeTemplate.GetName(),
					UID:                scopeTemplate.GetUID(),
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				}))
				Expect(existingRole.Rules).Should(Equal([]rbacv1.PolicyRule{
					{
						Verbs:     []string{"get", "watch", "list"},
						APIGroups: []string{""},
						Resources: []string{"secrets"},
					},
				}))
			})
		})
	})
})

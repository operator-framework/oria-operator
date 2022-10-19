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
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorsv1 "operator-framework/oria-operator/api/v1alpha1"
	"operator-framework/oria-operator/util"
)

var _ = Describe("ScopeInstanceReconciler", func() {

	Describe("hashScopeInstanceAndTemplate", func() {
		var (
			si *operatorsv1.ScopeInstance
			st *operatorsv1.ScopeTemplate
		)
		BeforeEach(func() {
			si = &operatorsv1.ScopeInstance{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ScopeInstance",
					APIVersion: "operators.io.operator-framework/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "scopeinstance-name",
				},
				Spec: operatorsv1.ScopeInstanceSpec{
					ScopeTemplateName: "scopetemplate-name",
					Namespaces:        []string{"test-ns"},
				},
			}
			st = &operatorsv1.ScopeTemplate{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ScopeTemplate",
					APIVersion: "operators.io.operator-framework/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "scopetemplate-hash",
				},
				Spec: operatorsv1.ScopeTemplateSpec{
					ClusterRoles: []operatorsv1.ClusterRoleTemplate{
						{
							GenerateName: "test",
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
		})
		It("should return the hash from the two objects", func() {
			expected := util.HashObject(&referenceHash{
				ScopeInstanceSpec: &si.Spec,
				ScopeTemplateSpec: &st.Spec,
			})
			thehash := hashScopeInstanceAndTemplate(si, st)
			Expect(expected).To(Equal(thehash))
		})
		It("should return a different hash if the scopeinstance changes", func() {
			notexpected := util.HashObject(&referenceHash{
				ScopeInstanceSpec: &si.Spec,
				ScopeTemplateSpec: &st.Spec,
			})
			si.Spec = operatorsv1.ScopeInstanceSpec{}
			thehash := hashScopeInstanceAndTemplate(si, st)
			Expect(notexpected).ToNot(Equal(thehash))
		})
		It("should return a different hash if the scopetemplate changes", func() {
			notexpected := util.HashObject(&referenceHash{
				ScopeInstanceSpec: &si.Spec,
				ScopeTemplateSpec: &st.Spec,
			})
			st.Spec = operatorsv1.ScopeTemplateSpec{}
			thehash := hashScopeInstanceAndTemplate(si, st)
			Expect(notexpected).ToNot(Equal(thehash))
		})
	})

	// Test the controller
	When("a ScopeInstance is created", func() {

		var (
			scopeInstance *operatorsv1.ScopeInstance
			scopeTemplate *operatorsv1.ScopeTemplate
			namespace     *corev1.Namespace
		)
		BeforeEach(func() {

			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
			}
			Expect(k8sClient.Create(ctx, namespace)).NotTo(HaveOccurred())

			scopeTemplate = &operatorsv1.ScopeTemplate{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ScopeTemplate",
					APIVersion: "operators.io.operator-framework/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "scopetemplate-test",
				},
				Spec: operatorsv1.ScopeTemplateSpec{
					ClusterRoles: []operatorsv1.ClusterRoleTemplate{
						{
							GenerateName: "test",
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

			scopeInstance = &operatorsv1.ScopeInstance{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ScopeInstance",
					APIVersion: "operators.io.operator-framework/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "scopeinstance-test",
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
			Expect(k8sClient.Delete(ctx, scopeTemplate)).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, namespace)).NotTo(HaveOccurred())

			// cleanup ClusterRoles since OwnerReferences do not work in envtest
			labels := map[string]string{clusterRoleGenerateKey: "test"}
			clusterRoles := &rbacv1.ClusterRoleList{}

			Expect(k8sClient.List(ctx, clusterRoles, client.MatchingLabels(labels))).NotTo(HaveOccurred())

			for _, crb := range clusterRoles.Items {
				if err := k8sClient.Delete(ctx, &crb); err != nil && !k8sapierrors.IsNotFound(err) {
					Fail("problem deleting clusterrole")
				}
			}
		})

		When("a scopeInstance references a non existent scopetemplate", func() {
			var si *operatorsv1.ScopeInstance
			BeforeEach(func() {
				/*
					create a scopeInstance for the reconciler
				*/
				si = &operatorsv1.ScopeInstance{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ScopeInstance",
						APIVersion: "operators.io.operator-framework/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "scopeinstance-notemplate",
					},
				}
				err := k8sClient.Create(ctx, si)
				Expect(err).NotTo(HaveOccurred())
			})
			AfterEach(func() {
				// delete the scope instance
				Expect(k8sClient.Delete(ctx, si)).NotTo(HaveOccurred())
			})

			It("should not create a clusterRole", func() {
				labels := map[string]string{scopeTemplateUIDKey: "nonexistent-st",
					clusterRoleGenerateKey: "test"}

				clusterRoleList := listClusterRole(0, labels)
				Expect(clusterRoleList.Items).Should(BeNil())
			})
			It("should delete the bindings that match the instance uid", func() {
				// setup:
				// create bindings with instance uids, and they should get
				// deleted.
			})
		})
		When("it references a scopetemplate with no namespace", func() {
			It("should create the clusterrolebinding references by the scopetemplate", func() {
			})
		})
		When("it references a scopetemplate with a namespace", func() {
			It("should create the rolebinding references by the scopetemplate in the namespace", func() {
				// scopetepmlate to scopeinstance or create a new one
			})
		})

		When("a scopeInstance is created that references the scopeTemplate with a single namespace", func() {

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

			It("should create the expected RoleBinding within the test namespace", func() {
				labels := map[string]string{scopeInstanceUIDKey: string(scopeInstance.GetUID()),
					clusterRoleBindingGenerateKey: "test"}

				roleBindingList := listRoleBinding(namespace.GetName(), 1, labels)

				existingRB := &roleBindingList.Items[0]

				verifyRoleBindings(existingRB, scopeInstance, scopeTemplate)
			})

			When("a scopeInstance is updated to include another namespace", func() {
				var namespace2 *corev1.Namespace
				BeforeEach(func() {
					namespace2 = &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "test-",
						},
					}
					Expect(k8sClient.Create(ctx, namespace2)).NotTo(HaveOccurred())
				})
				AfterEach(func() {
					Expect(k8sClient.Delete(ctx, namespace2)).NotTo(HaveOccurred())
				})

				It("Should create new roleBindings in the expected namespaces", func() {
					Eventually(func() error {
						scopeInstance := &operatorsv1.ScopeInstance{}
						if err := k8sClient.Get(ctx, types.NamespacedName{Name: "scopeinstance-test"}, scopeInstance); err != nil {
							return err
						}
						scopeInstance.Spec.Namespaces = []string{namespace.GetName(), namespace2.GetName()}
						if err := k8sClient.Update(ctx, scopeInstance); err != nil {
							return err
						}
						return nil
					}, timeout, interval).Should(BeNil())

					labels := map[string]string{scopeInstanceUIDKey: string(scopeInstance.GetUID()),
						clusterRoleBindingGenerateKey: "test"}

					roleBindingList := listRoleBinding(namespace2.GetName(), 1, labels)

					existingRB := &roleBindingList.Items[0]

					verifyRoleBindings(existingRB, scopeInstance, scopeTemplate)

					roleBindingList = listRoleBinding(namespace.GetName(), 1, labels)

					existingRB = &roleBindingList.Items[0]
					verifyRoleBindings(existingRB, scopeInstance, scopeTemplate)
				})

				When("a scopeInstance is updated to remove one of the namespace", func() {
					It("should remove respective roleBindings in the expected namespaces", func() {
						Eventually(func() error {
							scopeInstance := &operatorsv1.ScopeInstance{}
							if err := k8sClient.Get(ctx, types.NamespacedName{Name: "scopeinstance-test"}, scopeInstance); err != nil {
								return err
							}
							scopeInstance.Spec.Namespaces = []string{namespace2.GetName()}
							if err := k8sClient.Update(ctx, scopeInstance); err != nil {
								return err
							}
							return nil
						}, timeout, interval).Should(BeNil())

						labels := map[string]string{scopeInstanceUIDKey: string(scopeInstance.GetUID()),
							clusterRoleBindingGenerateKey: "test"}

						roleBindingList := listRoleBinding(namespace2.GetName(), 1, labels)

						existingRB := &roleBindingList.Items[0]

						verifyRoleBindings(existingRB, scopeInstance, scopeTemplate)

						roleBindingList = listRoleBinding(namespace.GetName(), 0, labels)

					})

					When("a scopeInstance is updated to remove all namespaces", func() {
						It("Should create new clusterRoleBindings in the expected namespaces", func() {
							Eventually(func() error {
								scopeInstance := &operatorsv1.ScopeInstance{}
								if err := k8sClient.Get(ctx, types.NamespacedName{Name: "scopeinstance-test"}, scopeInstance); err != nil {
									return err
								}
								scopeInstance.Spec.Namespaces = []string{}
								if err := k8sClient.Update(ctx, scopeInstance); err != nil {
									return err
								}
								return nil
							}, timeout, interval).Should(BeNil())

							clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
							Eventually(func() error {
								err := k8sClient.List(ctx, clusterRoleBindingList,
									client.MatchingLabels{
										scopeInstanceUIDKey:           string(scopeInstance.GetUID()),
										clusterRoleBindingGenerateKey: "test",
									})
								if err != nil {
									return err
								}

								if len(clusterRoleBindingList.Items) != 1 {
									return fmt.Errorf("Expected 1 roleBinding, found %d", len(clusterRoleBindingList.Items))
								}

								return nil
							}, timeout, interval).Should(BeNil())

							existingCRB := &clusterRoleBindingList.Items[0]

							Expect(len(existingCRB.OwnerReferences)).To(Equal(1))
							Expect(existingCRB.OwnerReferences).Should(ContainElement(metav1.OwnerReference{
								APIVersion:         "operators.io.operator-framework/v1alpha1",
								Kind:               "ScopeInstance",
								Name:               scopeInstance.GetObjectMeta().GetName(),
								UID:                scopeInstance.GetObjectMeta().GetUID(),
								Controller:         pointer.Bool(true),
								BlockOwnerDeletion: pointer.Bool(true),
							}))

							Expect(len(existingCRB.Subjects)).To(Equal(1))
							Expect(existingCRB.Subjects).Should(ContainElement(rbacv1.Subject{
								Kind:     "Group",
								Name:     "manager",
								APIGroup: "rbac.authorization.k8s.io",
							}))
							Expect(existingCRB.RoleRef).To(Equal(rbacv1.RoleRef{
								Kind:     "ClusterRole",
								Name:     "test",
								APIGroup: "rbac.authorization.k8s.io",
							}))

							labels := map[string]string{scopeInstanceUIDKey: string(scopeInstance.GetUID()),
								scopeTemplateUIDKey:           string(scopeTemplate.GetUID()),
								clusterRoleBindingGenerateKey: "test"}

							roleBindingList := listRoleBinding(namespace.GetName(), 0, labels)
							Expect(len(roleBindingList.Items)).To(Equal(0))

							roleBindingList = listRoleBinding(namespace2.GetName(), 0, labels)
							Expect(len(roleBindingList.Items)).To(Equal(0))
						})
					})
				})
			})
		})
	})
})

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

package e2e

import (
	"context"
	"fmt"
	operatorsv1 "operator-framework/oria-operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Oria-Operator e2e", func() {
	testScenario("Scoping to a single namespace", []string{"oria-e2e-singlens"}, false)
	testScenario("Scoping to multiple namespaces", []string{"oria-e2e-namespace1", "oria-e2e-namespace2", "oria-e2e-namespace3"}, false)
	testScenario("Scoping to all namespaces (cluster-wide scoping)", nil, true)
})

// testScenario is a generalized test suite that can be used to test different scenarios.
// It accepts a description of the scenario, a list of namespaces, and a bool to indicate
// whether or not the permissions should be cluster-scoped. If the permissions are
// set to be cluster-scoped any namespaces provided will be ignored and will use a
// default namespace value of 'oria-e2e-namespace'.
func testScenario(description string, namespaces []string, clusterScoped bool) {
	Context(description, func() {
		var (
			// store Namespaces and ServiceAccounts in a list
			// so we can keep track of them for use in later tests
			// and clean up logic.
			namespaceList      []*corev1.Namespace
			serviceAccountList []*corev1.ServiceAccount

			// helper function for ensuring the creation
			// of a resource will eventually succeed
			createResource = func(res client.Object) {
				createRes := func() error {
					return c.Create(context.Background(), res)
				}
				Eventually(createRes).Should(Succeed())
			}
		)

		BeforeEach(func() {
			namespaceList = []*corev1.Namespace{}
			serviceAccountList = []*corev1.ServiceAccount{}

			// If the test scenario is not cluster scoped create
			// namespaces and service accounts based on the provided
			// namespace list. Otherwise, use the default namespace
			// 'oria-e2e-namespace'.
			if !clusterScoped {
				for _, ns := range namespaces {
					namespace := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: ns,
						},
					}
					createResource(namespace)
					namespaceList = append(namespaceList, namespace.DeepCopy())

					serviceAccount := &corev1.ServiceAccount{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "oria-e2e-serviceaccount",
							Namespace: namespace.Name,
						},
						AutomountServiceAccountToken: pointer.Bool(true),
					}
					createResource(serviceAccount)
					serviceAccountList = append(serviceAccountList, serviceAccount.DeepCopy())
				}
			} else {
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "oria-e2e-namespace",
					},
				}
				createResource(namespace)
				namespaceList = append(namespaceList, namespace.DeepCopy())

				serviceAccount := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "oria-e2e-serviceaccount",
						Namespace: namespace.Name,
					},
					AutomountServiceAccountToken: pointer.Bool(true),
				}
				createResource(serviceAccount)
				serviceAccountList = append(serviceAccountList, serviceAccount.DeepCopy())
			}
		})

		AfterEach(func() {
			for _, namespace := range namespaceList {
				Expect(c.Delete(context.Background(), namespace)).Should(Succeed())
			}

			for _, serviceAccount := range serviceAccountList {
				Expect(c.Delete(context.Background(), serviceAccount)).Should(Succeed())
			}
		})

		When("Creating a ScopeTemplate", func() {
			var scopeTemplate *operatorsv1.ScopeTemplate
			BeforeEach(func() {
				// Create the list of subjects based on the
				// ServiceAccounts we created previously
				subjects := []rbacv1.Subject{}
				for _, serviceAccount := range serviceAccountList {
					subjects = append(subjects, rbacv1.Subject{
						Kind:      "ServiceAccount",
						APIGroup:  corev1.GroupName,
						Name:      serviceAccount.Name,
						Namespace: serviceAccount.Namespace,
					})
				}

				scopeTemplate = &operatorsv1.ScopeTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name: "oria-e2e-scopetemplate",
					},
					Spec: operatorsv1.ScopeTemplateSpec{
						ClusterRoles: []operatorsv1.ClusterRoleTemplate{
							{
								GenerateName: "oria-e2e-clusterrole",
								Rules: []rbacv1.PolicyRule{
									{
										Verbs:     []string{"get", "list"},
										Resources: []string{"pods"},
										APIGroups: []string{corev1.GroupName},
									},
								},
								Subjects: subjects,
							},
						},
					},
				}
				createResource(scopeTemplate)
			})

			AfterEach(func() {
				Expect(c.Delete(context.Background(), scopeTemplate)).Should(Succeed())
			})

			It("Should not create the defined ClusterRole", func() {
				err := c.Get(context.Background(), types.NamespacedName{Name: scopeTemplate.Spec.ClusterRoles[0].GenerateName}, &rbacv1.ClusterRole{})
				Expect(err).Should(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).Should(BeTrue())
			})

			It("Should have a successful status condition", func() {
				st := &operatorsv1.ScopeTemplate{}
				Eventually(func() bool {
					_ = c.Get(context.Background(), client.ObjectKeyFromObject(scopeTemplate), st)
					return len(st.Status.Conditions) > 0
				}).Should(BeTrue())

				cond := meta.FindStatusCondition(st.Status.Conditions, operatorsv1.TypeTemplated)
				Expect(cond).ShouldNot(BeNil())
				Expect(cond.Reason).Should(Equal(operatorsv1.ReasonTemplatingSuccessful))
				Expect(cond.Status).Should(Equal(metav1.ConditionTrue))
			})

			When("Creating a ScopeInstances that references a nonexistent ScopeTemplate", func() {
				var scopeInstance *operatorsv1.ScopeInstance

				BeforeEach(func() {
					scopeInstance = &operatorsv1.ScopeInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name: "oria-e2e-scopeinstance-ne",
						},
						Spec: operatorsv1.ScopeInstanceSpec{
							ScopeTemplateName: "nonexistent",
						},
					}
					// If the test scenario is not cluster scoped then
					// add the namespaces we created to the ScopeInstance
					if !clusterScoped {
						for _, namespace := range namespaceList {
							scopeInstance.Spec.Namespaces = append(scopeInstance.Spec.Namespaces, namespace.Name)
						}
					}
					createResource(scopeInstance)
				})

				AfterEach(func() {
					Expect(c.Delete(context.Background(), scopeInstance)).Should(Succeed())
				})

				It("Should have a failed status condition", func() {
					si := &operatorsv1.ScopeInstance{}
					Eventually(func() bool {
						_ = c.Get(context.Background(), client.ObjectKeyFromObject(scopeInstance), si)
						return len(si.Status.Conditions) > 0
					}).Should(BeTrue())

					cond := meta.FindStatusCondition(si.Status.Conditions, operatorsv1.TypeScoped)
					Expect(cond).ShouldNot(BeNil())
					Expect(cond.Reason).Should(Equal(operatorsv1.ReasonScopeTemplateNotFound))
					Expect(cond.Status).Should(Equal(metav1.ConditionFalse))
				})
			})

			When("Creating a ScopeInstance that references an existing ScopeTemplate", func() {
				var scopeInstance *operatorsv1.ScopeInstance

				BeforeEach(func() {
					scopeInstance = &operatorsv1.ScopeInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name: "oria-e2e-scopeinstance",
						},
						Spec: operatorsv1.ScopeInstanceSpec{
							ScopeTemplateName: scopeTemplate.Name,
						},
					}
					// If the test scenario is not cluster scoped then
					// add the namespaces we created to the ScopeInstance
					if !clusterScoped {
						for _, namespace := range namespaceList {
							scopeInstance.Spec.Namespaces = append(scopeInstance.Spec.Namespaces, namespace.Name)
						}
					}
					createResource(scopeInstance)
				})

				AfterEach(func() {
					Expect(c.Delete(context.Background(), scopeInstance)).Should(Succeed())
				})

				It("Should have a successful status condition", func() {
					si := &operatorsv1.ScopeInstance{}
					Eventually(func() bool {
						_ = c.Get(context.Background(), client.ObjectKeyFromObject(scopeInstance), si)
						return len(si.Status.Conditions) > 0
					}).Should(BeTrue())

					cond := meta.FindStatusCondition(si.Status.Conditions, operatorsv1.TypeScoped)
					Expect(cond).ShouldNot(BeNil())
					Expect(cond.Reason).Should(Equal(operatorsv1.ReasonScopingSuccessful))
					Expect(cond.Status).Should(Equal(metav1.ConditionTrue))
				})

				It("Should create the ClusterRole defined in the ScopeTemplate", func() {
					cr := &rbacv1.ClusterRole{}
					fetchCr := func() error {
						return c.Get(context.Background(), types.NamespacedName{Name: scopeTemplate.Spec.ClusterRoles[0].GenerateName}, cr)
					}
					Eventually(fetchCr).Should(Succeed())
					Expect(cr.Rules).Should(Equal(scopeTemplate.Spec.ClusterRoles[0].Rules))
					Expect(cr.Labels).Should(HaveKeyWithValue("operators.coreos.io/scopeTemplateUID", string(scopeTemplate.GetUID())))
					Expect(cr.Labels).Should(HaveKeyWithValue("operators.coreos.io/generateName", scopeTemplate.Spec.ClusterRoles[0].GenerateName))
					Expect(cr.Labels).Should(HaveKey("operators.coreos.io/scopeTemplateHash"))
				})

				It("Should create (Cluster)RoleBinding(s) as specified by the ScopeInstance", func() {
					// If the test scenario is cluster scoped then we
					// want to ensure a ClusterRoleBinding is created.
					// Otherwise we check to ensure that a RoleBinding
					// is created in each namespace.
					if clusterScoped {
						crb := &rbacv1.ClusterRoleBinding{}
						fetchCrb := func() error {
							crbList := &rbacv1.ClusterRoleBindingList{}
							err := c.List(context.Background(), crbList)
							if err != nil {
								return err
							}

							// We know we found the proper ClusterRoleBinding if
							// the RoleRef.Name is equivalent to the name of the
							// ClusterRole that should have been generated.
							for _, crbItem := range crbList.Items {
								if crbItem.RoleRef.Name == scopeTemplate.Spec.ClusterRoles[0].GenerateName {
									crb = &crbItem
									break
								}
							}

							if crb.Name == "" {
								return fmt.Errorf("haven't found ClusterRoleBinding that matches expectations")
							}

							return nil
						}
						Eventually(fetchCrb).Should(Succeed())

						Expect(crb.Subjects).Should(Equal(scopeTemplate.Spec.ClusterRoles[0].Subjects))
						Expect(crb.Name).Should(ContainSubstring(scopeTemplate.Spec.ClusterRoles[0].GenerateName))
						Expect(crb.Labels).Should(HaveKeyWithValue("operators.coreos.io/scopeInstanceUID", string(scopeInstance.GetUID())))
						Expect(crb.Labels).Should(HaveKeyWithValue("operators.coreos.io/generateName", scopeTemplate.Spec.ClusterRoles[0].GenerateName))
						Expect(crb.Labels).Should(HaveKey("operators.coreos.io/scopeInstanceAndTemplateHash"))
					} else {
						for _, namespace := range namespaceList {
							rb := &rbacv1.RoleBinding{}
							fetchRb := func() error {
								rbList := &rbacv1.RoleBindingList{}
								err := c.List(context.Background(), rbList, &client.ListOptions{Namespace: namespace.Name})
								if err != nil {
									return err
								}

								// We know we found the proper RoleBinding if
								// the RoleRef.Name is equivalent to the name of the
								// ClusterRole that should have been generated.
								for _, rbItem := range rbList.Items {
									if rbItem.RoleRef.Name == scopeTemplate.Spec.ClusterRoles[0].GenerateName {
										rb = &rbItem
										break
									}
								}

								if rb.Name == "" {
									return fmt.Errorf("haven't found RoleBinding that matches expectations")
								}

								return nil
							}
							Eventually(fetchRb).Should(Succeed())

							Expect(rb.Subjects).Should(Equal(scopeTemplate.Spec.ClusterRoles[0].Subjects))
							Expect(rb.Name).Should(ContainSubstring(scopeTemplate.Spec.ClusterRoles[0].GenerateName))
							Expect(rb.Labels).Should(HaveKeyWithValue("operators.coreos.io/scopeInstanceUID", string(scopeInstance.GetUID())))
							Expect(rb.Labels).Should(HaveKeyWithValue("operators.coreos.io/generateName", scopeTemplate.Spec.ClusterRoles[0].GenerateName))
							Expect(rb.Labels).Should(HaveKey("operators.coreos.io/scopeInstanceAndTemplateHash"))
						}
					}
				})

				When("Making requests through scoped ServiceAccount", func() {
					var kctlPod *corev1.Pod
					var kctlPods []*corev1.Pod

					// helper function to make sure a given pod eventually reaches the desired phase
					checkStatus := func(key types.NamespacedName, expectedPhase corev1.PodPhase) func() error {
						return func() error {
							p := &corev1.Pod{}
							err := c.Get(context.Background(), key, p)
							if err != nil {
								return err
							}

							if p.Status.Phase != expectedPhase {
								return fmt.Errorf("pod has not reached phase %q - pod.Status.Phase=%q", expectedPhase, p.Status.Phase)
							}

							return nil
						}
					}

					BeforeEach(func() {
						kctlPods = []*corev1.Pod{}
					})

					AfterEach(func() {
						for _, pod := range kctlPods {
							Expect(c.Delete(context.Background(), pod)).Should(Succeed())
						}
					})

					It("Should be allowed to GET pods", func() {
						for _, serviceAccount := range serviceAccountList {
							kctlPod = &corev1.Pod{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "oria-e2e-kctlpod-get",
									Namespace: serviceAccount.Namespace,
								},
								Spec: corev1.PodSpec{
									ServiceAccountName: serviceAccount.Name,
									Containers: []corev1.Container{
										{
											Image:   "bitnami/kubectl",
											Command: []string{"kubectl"},
											Args:    []string{"get", "pods"},
											Name:    "kubectl-get-pods",
										},
									},
									RestartPolicy: corev1.RestartPolicyNever,
								},
							}
							createResource(kctlPod)
							Eventually(checkStatus(types.NamespacedName{Namespace: kctlPod.Namespace, Name: kctlPod.Name}, corev1.PodSucceeded)).Should(Succeed())
							kctlPods = append(kctlPods, kctlPod.DeepCopy())
						}
					})

					It("Should not be allowed to DELETE Pods", func() {
						for _, serviceAccount := range serviceAccountList {
							podToDelete := &corev1.Pod{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "oria-e2e-pod-delete",
									Namespace: serviceAccount.Namespace,
								},
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Image:   "bitnami/kubectl",
											Command: []string{"kubectl"},
											Name:    "to-be-deleted",
										},
									},
									RestartPolicy: corev1.RestartPolicyNever,
								},
							}
							createResource(podToDelete)
							kctlPods = append(kctlPods, podToDelete.DeepCopy())

							kctlPod = &corev1.Pod{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "oria-e2e-kctlpod-delete",
									Namespace: serviceAccount.Namespace,
								},
								Spec: corev1.PodSpec{
									ServiceAccountName: serviceAccount.Name,
									Containers: []corev1.Container{
										{
											Image:   "bitnami/kubectl",
											Command: []string{"kubectl"},
											Args:    []string{"delete", "pods", podToDelete.Name},
											Name:    "kubectl-delete-pods",
										},
									},
									RestartPolicy: corev1.RestartPolicyNever,
								},
							}
							createResource(kctlPod)
							Eventually(checkStatus(types.NamespacedName{Namespace: kctlPod.Namespace, Name: kctlPod.Name}, corev1.PodFailed)).Should(Succeed())
							kctlPods = append(kctlPods, kctlPod.DeepCopy())
						}
					})

					It("Should not be allowed to CREATE pods", func() {
						for _, serviceAccount := range serviceAccountList {
							kctlPod = &corev1.Pod{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "oria-e2e-kctlpod-create",
									Namespace: serviceAccount.Namespace,
								},
								Spec: corev1.PodSpec{
									ServiceAccountName: serviceAccount.Name,
									Containers: []corev1.Container{
										{
											Image:   "bitnami/kubectl",
											Command: []string{"kubectl"},
											Args:    []string{"run", "kubectl-pod", "--image=bitnami/kubectl", "--", "kubectl"},
											Name:    "kubectl-create-pods",
										},
									},
									RestartPolicy: corev1.RestartPolicyNever,
								},
							}
							createResource(kctlPod)
							Eventually(checkStatus(types.NamespacedName{Namespace: kctlPod.Namespace, Name: kctlPod.Name}, corev1.PodFailed)).Should(Succeed())
							kctlPods = append(kctlPods, kctlPod.DeepCopy())
						}
					})

					It("Should not be allowed to GET deployments", func() {
						for _, serviceAccount := range serviceAccountList {
							kctlPod = &corev1.Pod{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "oria-e2e-kctlpod-get-deploy",
									Namespace: serviceAccount.Namespace,
								},
								Spec: corev1.PodSpec{
									ServiceAccountName: serviceAccount.Name,
									Containers: []corev1.Container{
										{
											Image:   "bitnami/kubectl",
											Command: []string{"kubectl"},
											Args:    []string{"get", "deployments"},
											Name:    "kubectl-get-deploys",
										},
									},
									RestartPolicy: corev1.RestartPolicyNever,
								},
							}
							createResource(kctlPod)
							Eventually(checkStatus(types.NamespacedName{Namespace: kctlPod.Namespace, Name: kctlPod.Name}, corev1.PodFailed)).Should(Succeed())
							kctlPods = append(kctlPods, kctlPod.DeepCopy())
						}
					})

					It("Should only be able to GET pods from allowed namespaces", func() {
						for _, serviceAccount := range serviceAccountList {
							kctlPod = &corev1.Pod{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "oria-e2e-kctlpod-get-allowed-ns",
									Namespace: serviceAccount.Namespace,
								},
								Spec: corev1.PodSpec{
									ServiceAccountName: serviceAccount.Name,
									Containers: []corev1.Container{
										{
											Image:   "bitnami/kubectl",
											Command: []string{"kubectl"},
											Args:    []string{"-n", "kube-system", "get", "pods"},
											Name:    "kubectl-get-pods-allowed-ns",
										},
									},
									RestartPolicy: corev1.RestartPolicyNever,
								},
							}
							createResource(kctlPod)

							// if cluster-scoped it should be able to list Pods in a different namespace, otherwise it shouldn't
							if clusterScoped {
								Eventually(checkStatus(types.NamespacedName{Namespace: kctlPod.Namespace, Name: kctlPod.Name}, corev1.PodSucceeded)).Should(Succeed())
							} else {
								Eventually(checkStatus(types.NamespacedName{Namespace: kctlPod.Namespace, Name: kctlPod.Name}, corev1.PodFailed)).Should(Succeed())
							}
							kctlPods = append(kctlPods, kctlPod.DeepCopy())
						}
					})
				})
			})
		})
	})
}

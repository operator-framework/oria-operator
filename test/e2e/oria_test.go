package e2e

import (
	operatorsv1 "awgreene/scope-operator/api/v1alpha1"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Oria-Operator e2e", func() {
	var (
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "oria-e2e-namespace",
			},
		}

		serviceAccount = &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "oria-e2e-serviceaccount",
				Namespace: namespace.Name,
			},
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
						Subjects: []rbacv1.Subject{
							{
								Kind:      "ServiceAccount",
								APIGroup:  corev1.GroupName,
								Name:      serviceAccount.Name,
								Namespace: namespace.Name,
							},
						},
					},
				},
			},
		}

		scopeInstance = &operatorsv1.ScopeInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name: "oria-e2e-scopeinstance",
			},
			Spec: operatorsv1.ScopeInstanceSpec{
				ScopeTemplateName: scopeTemplate.Name,
				Namespaces:        []string{namespace.Name},
			},
		}
	)

	BeforeEach(func() {
		Expect(c.Create(context.Background(), namespace)).Should(Succeed())
		Expect(c.Create(context.Background(), serviceAccount)).Should(Succeed())
	})

	AfterEach(func() {
		Expect(c.Delete(context.Background(), namespace)).Should(Succeed())
		Expect(c.Delete(context.Background(), serviceAccount)).Should(Succeed())
	})

	Context("Limiting permissions", func() {
		When("Creating a ScopeTemplate", func() {
			BeforeEach(func() {
				Expect(c.Create(context.Background(), scopeTemplate)).Should(Succeed())
			})

			AfterEach(func() {
				Expect(c.Delete(context.Background(), scopeTemplate)).Should(Succeed())
			})
			It("Should not create the defined ClusterRole(s)", func() {
				err := c.Get(context.Background(), types.NamespacedName{Name: scopeTemplate.Spec.ClusterRoles[0].GenerateName}, &rbacv1.ClusterRole{})
				Expect(err).Should(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).Should(BeTrue())
			})

			When("Creating a ScopeInstance that references a ScopeTemplate", func() {
				BeforeEach(func() {
					Expect(c.Create(context.Background(), scopeInstance)).Should(Succeed())
				})

				AfterEach(func() {
					Expect(c.Delete(context.Background(), scopeInstance)).Should(Succeed())
				})
			})
		})
	})
})

package util

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorsv1 "awgreene/scope-operator/api/v1"
)

var _ = Describe("Util", func() {

	Describe("IsOwnedByLabel", func() {
	})
	Describe("GetOwnerByLabel", func() {
		var (
			owner *operatorsv1.ScopeInstance
			rb    rbacv1.RoleBinding
		)
		BeforeEach(func() {
			// create owner and rb here
			owner = &operatorsv1.ScopeInstance{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ScopeInstance",
					APIVersion: "operators.io.operator-framework/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "scopeinstance-name",
				},
				Spec: operatorsv1.ScopeInstanceSpec{
					ScopeTemplateName: "scopeinstance-name",
					Namespaces:        []string{"test-foo"},
				},
			}

			rb = rbacv1.RoleBinding{
				TypeMeta: metav1.TypeMeta{
					Kind: "RoleBinding",
					// APIVersion: "operators.io.operator-framework/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-rb",
				},
			}
		})
		It("should return true if owner label matches", func() {
			// set a specific UID for this owner
			owner.SetUID("uid")

			// add label to match the owner's uid
			rb.SetLabels(map[string]string{
				OwnerLabel: "uid", // should match the uid of the owner
			})

			Expect(GetOwnerByLabel(rb.DeepCopy(), owner)).To(Equal(true))
		})
		It("should return false if owner label does not match", func() {
			// set a specific UID for this owner
			owner.SetUID("uid")

			// add label to match the owner's uid
			rb.SetLabels(map[string]string{
				OwnerLabel: "anotheruid", // should be different than owner
			})

			Expect(GetOwnerByLabel(rb.DeepCopy(), owner)).To(Equal(false))
		})
		It("should return false if owner label does not exist", func() {
			Expect(GetOwnerByLabel(rb.DeepCopy(), owner)).To(Equal(false))
		})
		It("should return false if either option is nil", func() {
			Expect(GetOwnerByLabel(nil, nil)).To(Equal(false))
			Expect(GetOwnerByLabel(rb.DeepCopy(), nil)).To(Equal(false))
			Expect(GetOwnerByLabel(nil, owner)).To(Equal(false))
		})
	})
	Describe("GetOwnerByRef", func() {
	})

	Describe("HashObject", func() {
		It("should return a hash for a complex object", func() {
			si := &operatorsv1.ScopeInstance{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ScopeInstance",
					APIVersion: "operators.io.operator-framework/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "scopeinstance-name",
				},
				Spec: operatorsv1.ScopeInstanceSpec{
					ScopeTemplateName: "scopeinstance-name",
					Namespaces:        []string{"test-foo"},
				},
			}

			hash := HashObject(si.Spec)
			Expect(hash).Should(Equal("5b568d69c6"))
		})
		It("should return a hash for an empty string", func() {
			hash := HashObject("")
			Expect(hash).Should(Equal("54766fc949"))
		})
		It("should return a hash for nil", func() {
			hash := HashObject(nil)
			Expect(hash).Should(Equal("5cb59f8c64"))
		})
	})

	Describe("deepHashObject", func() {
	})
})

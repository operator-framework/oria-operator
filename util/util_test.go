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
					UID:  "uid",
				},
				Spec: operatorsv1.ScopeInstanceSpec{
					ScopeTemplateName: "scopeinstance-name",
					Namespaces:        []string{"test-foo"},
				},
			}

			rb = rbacv1.RoleBinding{
				TypeMeta: metav1.TypeMeta{
					Kind: "RoleBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-rb",
				},
			}
		})
		It("should return true if no owner label matches but reference does", func() {
			// ensure labels are empty
			rb.SetLabels(map[string]string{})

			// reference owner from object
			rb.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: owner.APIVersion,
				Kind:       owner.Kind,
				Name:       "scopeinstance-name",
				UID:        "uid",
			}})

			Expect(IsOwnedByLabel(rb.DeepCopy(), owner)).To(Equal(true))
		})
		It("should return true if owner label matches", func() {
			// set a specific UID for this owner
			owner.SetUID("uid")

			// add label to match the owner's uid
			rb.SetLabels(map[string]string{
				OwnerLabel: "uid", // should match the uid of the owner
			})

			Expect(IsOwnedByLabel(rb.DeepCopy(), owner)).To(Equal(true))
		})
		It("should return false if no label and no reference", func() {
			Expect(IsOwnedByLabel(rb.DeepCopy(), owner)).To(Equal(false))
		})
		It("should return false if either option is nil", func() {
			Expect(IsOwnedByLabel(nil, nil)).To(Equal(false))
			Expect(IsOwnedByLabel(rb.DeepCopy(), nil)).To(Equal(false))
			Expect(IsOwnedByLabel(nil, owner)).To(Equal(false))
		})
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
					UID:  "uid",
				},
				Spec: operatorsv1.ScopeInstanceSpec{
					ScopeTemplateName: "scopeinstance-name",
					Namespaces:        []string{"test-foo"},
				},
			}

			rb = rbacv1.RoleBinding{
				TypeMeta: metav1.TypeMeta{
					Kind: "RoleBinding",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-rb",
				},
			}
		})
		It("should return false if the owner is not referenced by the object", func() {
			// not referenced at all
			Expect(GetOwnerByRef(rb.DeepCopy(), owner)).To(Equal(false))

			// reference a different owner from object
			rb.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: owner.APIVersion,
				Kind:       owner.Kind,
				Name:       "scopeinstance-name",
				UID:        "anotheruid",
			}})

			Expect(GetOwnerByRef(rb.DeepCopy(), owner)).To(Equal(false))
		})
		It("should return true if the owner is referenced by the object", func() {
			// reference owner from object
			rb.SetOwnerReferences([]metav1.OwnerReference{{
				APIVersion: owner.APIVersion,
				Kind:       owner.Kind,
				Name:       "scopeinstance-name",
				UID:        "uid",
			}})

			Expect(GetOwnerByRef(rb.DeepCopy(), owner)).To(Equal(true))
		})
		It("should return false if either option is nil", func() {
			Expect(GetOwnerByRef(nil, nil)).To(Equal(false))
			Expect(GetOwnerByRef(rb.DeepCopy(), nil)).To(Equal(false))
			Expect(GetOwnerByRef(nil, owner)).To(Equal(false))
		})
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
})

package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Owner is used to build an OwnerReference, and we need type and object metadata
type Owner interface {
	metav1.Object
	runtime.Object
}

const (
	OwnerLabel = "operators.coreos.io/scopeInstance"
)

func IsOwnedByLabel(object metav1.Object, owner Owner) bool {
	ok := GetOwnerByLabel(object, owner)
	if !ok {
		return false
	}

	ownerref := GetOwnerByRef(object, owner)
	if !ownerref {
		return false
	}

	return true
}

func GetOwnerByLabel(object metav1.Object, owner Owner) (ok bool) {
	return object.GetLabels()[OwnerLabel] == string(owner.GetUID())
}

func GetOwnerByRef(object metav1.Object, owner Owner) (ok bool) {
	for _, oref := range object.GetOwnerReferences() {
		if string(oref.UID) == string(owner.GetUID()) {
			return true
		}
	}
	return false
}

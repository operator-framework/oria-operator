package util

import (
	"fmt"
	"hash"
	"hash/fnv"

	"github.com/davecgh/go-spew/spew"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
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

// HashObject calculates a hash from an object
func HashObject(obj interface{}) string {
	hasher := fnv.New32a()
	deepHashObject(hasher, &obj)
	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
}

// DeepHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func deepHashObject(hasher hash.Hash, objectToWrite interface{}) {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	printer.Fprintf(hasher, "%#v", objectToWrite)
}

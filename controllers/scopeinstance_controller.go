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
	"context"
	"fmt"
	"reflect"

	operatorsv1 "awgreene/scope-operator/api/v1alpha1"
	"awgreene/scope-operator/util"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ScopeInstanceReconciler reconciles a ScopeInstance object
type ScopeInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	// scopeInstanceUIDKey is used to track "owners" of bindings we create.
	scopeInstanceUIDKey = "operators.coreos.io/scopeInstanceUID"

	// referenceHashKey is used to track "abandoned" bindings we created.
	referenceHashKey = "operators.coreos.io/scopeInstanceAndTemplateHash"

	// generateNames are used to track each binding we create for a single scopeTemplate
	clusterRoleBindingGenerateKey = "operators.coreos.io/generateName"
	siCtrlFieldOwner              = "scopeinstance-controller"
)

//+kubebuilder:rbac:groups=operators.io.operator-framework,resources=scopeinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operators.io.operator-framework,resources=scopeinstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operators.io.operator-framework,resources=scopeinstances/finalizers,verbs=update
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;rolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ScopeInstance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *ScopeInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	log.Log.V(2).Info("Reconciling ScopeInstance", "namespaceName", req.NamespacedName)

	existingIn := &operatorsv1.ScopeInstance{}
	if err := r.Client.Get(ctx, req.NamespacedName, existingIn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reconciledIn := existingIn.DeepCopy()
	res, reconcileErr := r.reconcile(ctx, reconciledIn)

	// Update the status subresource before updating the main object. This is
	// necessary because, in many cases, the main object update will remove the
	// finalizer, which will cause the core Kubernetes deletion logic to
	// complete. Therefore, we need to make the status update prior to the main
	// object update to ensure that the status update can be processed before
	// a potential deletion.
	if !equality.Semantic.DeepEqual(existingIn.Status, reconciledIn.Status) {
		if updateErr := r.Client.Status().Update(ctx, reconciledIn); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	existingIn.Status, reconciledIn.Status = operatorsv1.ScopeInstanceStatus{}, operatorsv1.ScopeInstanceStatus{}
	if !equality.Semantic.DeepEqual(existingIn, reconciledIn) {
		if updateErr := r.Client.Update(ctx, reconciledIn); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	return res, reconcileErr
}

func (r *ScopeInstanceReconciler) reconcile(ctx context.Context, in *operatorsv1.ScopeInstance) (ctrl.Result, error) {
	// Get the ScopeTemplate referenced by the ScopeInstance
	st := &operatorsv1.ScopeTemplate{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: in.Spec.ScopeTemplateName}, st); err != nil {
		if !k8sapierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		// Delete anything owned by the scopeInstance if the scopeTemplate is gone.
		listOption := client.MatchingLabels{
			scopeInstanceUIDKey: string(in.GetUID()),
		}

		if err := r.deleteBindings(ctx, listOption); err != nil {
			log.Log.V(2).Error(err, "in deleting (Cluster)RoleBindings")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// create required roleBindings and clusterRoleBindings.
	if err := r.ensureBindings(ctx, in, st); err != nil {
		log.Log.V(2).Error(err, "in creating (Cluster)RoleBindings")
		return ctrl.Result{}, err
	}

	// delete out of date (Cluster)RoleBindings
	if err := r.deleteOldBindings(ctx, in, st); err != nil {
		log.Log.V(2).Error(err, "in deleting (Cluster)RoleBindings")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// ensureBindings will ensure that the proper bindings are created for a
// given ScopeInstance and ScopeTemplate. If the ScopeInstance.Spec.Namespaces
// is empty it will create a ClusterRoleBinding. If the
// ScopeInstance.Spec.Namespaces is not empty it will create a RoleBinding
// in each provided namespace. A separate (Cluster)RoleBinding will be created
// for each ClusterRole specified in the ScopeTemplate
func (r *ScopeInstanceReconciler) ensureBindings(ctx context.Context, in *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate) error {
	for _, cr := range st.Spec.ClusterRoles {
		if len(in.Spec.Namespaces) == 0 {
			err := r.createOrUpdateClusterRoleBinding(ctx, &cr, in, st)
			if err != nil {
				return err
			}
		} else {
			for _, ns := range in.Spec.Namespaces {
				err := r.createOrUpdateRoleBinding(ctx, &cr, in, st, ns)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (r *ScopeInstanceReconciler) createOrUpdateClusterRoleBinding(ctx context.Context, cr *operatorsv1.ClusterRoleTemplate, in *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate) error {
	crb := r.clusterRoleBindingManifest(cr, in, st)
	crbList := &rbacv1.ClusterRoleBindingList{}
	if err := r.Client.List(ctx, crbList, client.MatchingLabels{
		scopeInstanceUIDKey:           string(in.GetUID()),
		clusterRoleBindingGenerateKey: cr.GenerateName,
	}); err != nil {
		return err
	}

	if len(crbList.Items) > 1 {
		return fmt.Errorf("more than one ClusterRoleBinding found for ClusterRole %s", cr.GenerateName)
	}

	// Create the ClusterRoleBinding if one doesn't already exist
	if len(crbList.Items) == 0 {
		return r.Client.Create(ctx, crb)
	}

	existingCRB := &crbList.Items[0]
	if util.IsOwnedByLabel(existingCRB.DeepCopy(), in) &&
		reflect.DeepEqual(existingCRB.Subjects, crb.Subjects) &&
		reflect.DeepEqual(existingCRB.Labels, crb.Labels) {
		log.Log.V(2).Info("existing ClusterRoleBinding does not need to be updated", "UID", existingCRB.GetUID())
		return nil
	}

	patchObj := r.clusterRoleBindingPatchObj(existingCRB, crb)

	// server-side apply patch
	if err := r.patchBinding(ctx, patchObj); err != nil {
		return err
	}

	return nil
}

func (r *ScopeInstanceReconciler) clusterRoleBindingPatchObj(oldCrb *rbacv1.ClusterRoleBinding, crb *rbacv1.ClusterRoleBinding) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": rbacv1.SchemeGroupVersion.String(),
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"name":            oldCrb.Name,
				"ownerReferences": crb.OwnerReferences,
				"labels":          crb.Labels,
			},
			"subjects": crb.Subjects,
		},
	}
}

func (r *ScopeInstanceReconciler) createOrUpdateRoleBinding(ctx context.Context, cr *operatorsv1.ClusterRoleTemplate, in *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate, namespace string) error {
	rb := r.roleBindingManifest(cr, in, st, namespace)
	rbList := &rbacv1.RoleBindingList{}
	if err := r.Client.List(ctx, rbList, &client.ListOptions{
		Namespace: namespace,
	}, client.MatchingLabels{
		scopeInstanceUIDKey:           string(in.GetUID()),
		clusterRoleBindingGenerateKey: cr.GenerateName,
	}); err != nil {
		return err
	}

	if len(rbList.Items) > 1 {
		return fmt.Errorf("more than one RoleBinding found for ClusterRole %s", cr.GenerateName)
	}

	// Create the RoleBinding if one doesn't already exist
	if len(rbList.Items) == 0 {
		return r.Client.Create(ctx, rb)
	}

	log.Log.V(2).Info("Updating existing rb", "namespaced", rbList.Items[0].GetNamespace(), "name", rbList.Items[0].GetName())

	existingRB := &rbList.Items[0]

	if util.IsOwnedByLabel(existingRB.DeepCopy(), in) &&
		reflect.DeepEqual(existingRB.Subjects, rb.Subjects) &&
		reflect.DeepEqual(existingRB.Labels, rb.Labels) {
		log.Log.V(2).Info("existing RoleBinding does not need to be updated", "UID", existingRB.GetUID())
		return nil
	}

	patchObj := r.roleBindingPatchObj(existingRB, rb)

	// server-side apply patch
	if err := r.patchBinding(ctx, patchObj); err != nil {
		return err
	}

	return nil
}

func (r *ScopeInstanceReconciler) roleBindingPatchObj(oldRb *rbacv1.RoleBinding, rb *rbacv1.RoleBinding) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": rbacv1.SchemeGroupVersion.String(),
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name":            oldRb.Name,
				"namespace":       oldRb.Namespace,
				"ownerReferences": rb.OwnerReferences,
				"labels":          rb.Labels,
			},
			"subjects": rb.Subjects,
		},
	}
}

func (r *ScopeInstanceReconciler) patchBinding(ctx context.Context, binding client.Object) error {
	return r.Client.Patch(ctx,
		binding,
		client.Apply,
		client.FieldOwner(siCtrlFieldOwner),
		client.ForceOwnership)
}

// TODO: use a client.DeleteAllOf instead of a client.List -> delete
func (r *ScopeInstanceReconciler) deleteBindings(ctx context.Context, listOptions ...client.ListOption) error {
	clusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
	if err := r.Client.List(ctx, clusterRoleBindings, listOptions...); err != nil {
		// TODO: Aggregate errors
		return err
	}

	for _, crb := range clusterRoleBindings.Items {
		// TODO: Aggregate errors
		if err := r.Client.Delete(ctx, &crb); err != nil && !k8sapierrors.IsNotFound(err) {
			return err
		}
	}

	roleBindings := &rbacv1.RoleBindingList{}
	if err := r.Client.List(ctx, roleBindings, listOptions...); err != nil {
		// TODO: Aggregate errors
		return err
	}

	for _, rb := range roleBindings.Items {
		// TODO: Aggregate errors
		if err := r.Client.Delete(ctx, &rb); err != nil && !k8sapierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// deleteOldBindings will delete any (Cluster)RoleBindings that are owned by
// the given ScopeInstance and are no longer up to date.Being out of date
// means the combined hash of ScopeInstance.Spec and ScopeTemplate.Spec is different
func (r *ScopeInstanceReconciler) deleteOldBindings(ctx context.Context, in *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate) error {
	combinedHash := hashScopeInstanceAndTemplate(in, st)
	hashReq, err := labels.NewRequirement(referenceHashKey, selection.NotEquals, []string{combinedHash})
	if err != nil {
		return err
	}

	siUIDReq, err := labels.NewRequirement(scopeInstanceUIDKey, selection.Equals, []string{string(in.GetUID())})
	if err != nil {
		return err
	}

	listOptions := &client.ListOptions{
		LabelSelector: labels.NewSelector().Add(*hashReq, *siUIDReq),
	}

	if err := r.deleteBindings(ctx, listOptions); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScopeInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorsv1.ScopeInstance{}).
		Watches(&source.Kind{Type: &operatorsv1.ScopeTemplate{}}, handler.EnqueueRequestsFromMapFunc(r.mapToScopeInstance)).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}

func (r *ScopeInstanceReconciler) mapToScopeInstance(obj client.Object) (requests []reconcile.Request) {
	if obj == nil || obj.GetName() == "" {
		return nil
	}

	// Requeue all Scope Instance in the resource namespace
	ctx := context.TODO()
	scopeInstanceList := &operatorsv1.ScopeInstanceList{}

	if err := r.Client.List(ctx, scopeInstanceList); err != nil {
		log.Log.Error(err, "error listing scopeinstances")
		return nil
	}

	for _, si := range scopeInstanceList.Items {
		if si.Spec.ScopeTemplateName != obj.GetName() {
			continue
		}

		request := reconcile.Request{
			NamespacedName: types.NamespacedName{Namespace: si.GetNamespace(), Name: si.GetName()},
		}
		requests = append(requests, request)
	}

	return
}

// clusterRoleBindingManifest will create a ClusterRoleBinding from a
// ClusterRoleTemplate, ScopeInstance, and ScopeTemplate
func (r *ScopeInstanceReconciler) clusterRoleBindingManifest(cr *operatorsv1.ClusterRoleTemplate, in *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate) *rbacv1.ClusterRoleBinding {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cr.GenerateName + "-",
			Labels: map[string]string{
				scopeInstanceUIDKey:           string(in.GetUID()),
				referenceHashKey:              hashScopeInstanceAndTemplate(in, st),
				clusterRoleBindingGenerateKey: cr.GenerateName,
			},
		},
		Subjects: cr.Subjects,
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     cr.GenerateName,
			APIGroup: rbacv1.GroupName,
		},
	}

	err := ctrl.SetControllerReference(in, crb, r.Scheme)
	if err != nil {
		log.Log.Error(err, "setting controller reference for ClusterRoleBinding")
	}
	return crb
}

// roleBindingManifest will create a RoleBinding from a
// ClusterRoleTemplate, ScopeInstance, ScopeTemplate, and namespace
func (r *ScopeInstanceReconciler) roleBindingManifest(cr *operatorsv1.ClusterRoleTemplate, in *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate, namespace string) *rbacv1.RoleBinding {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cr.GenerateName + "-",
			Namespace:    namespace,
			Labels: map[string]string{
				scopeInstanceUIDKey:           string(in.GetUID()),
				referenceHashKey:              hashScopeInstanceAndTemplate(in, st),
				clusterRoleBindingGenerateKey: cr.GenerateName,
			},
		},
		Subjects: cr.Subjects,
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     cr.GenerateName,
			APIGroup: rbacv1.GroupName,
		},
	}

	err := ctrl.SetControllerReference(in, rb, r.Scheme)
	if err != nil {
		log.Log.Error(err, "setting controller reference for ClusterRoleBinding")
	}
	return rb
}

// referenceHash is used to store a ScopeInstance.Spec
// and ScopeTemplate.Spec. This object is used for getting
// the combined hash of both specs.
type referenceHash struct {
	ScopeInstanceSpec *operatorsv1.ScopeInstanceSpec
	ScopeTemplateSpec *operatorsv1.ScopeTemplateSpec
}

// hashScopeInstanceAndTemplate will take in a
// ScopeInstance and ScopeTemplate and return
// a combined hash of the ScopeInstance.Spec and
// ScopeTemplate.Spec fields
func hashScopeInstanceAndTemplate(si *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate) string {
	hashObj := &referenceHash{
		ScopeInstanceSpec: &si.Spec,
		ScopeTemplateSpec: &st.Spec,
	}

	return util.HashObject(hashObj)
}

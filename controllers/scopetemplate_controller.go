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
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	rbacv1ac "k8s.io/client-go/applyconfigurations/rbac/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ScopeTemplateReconciler reconciles a ScopeTemplate object
type ScopeTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	// generateNames are used to track each binding we create for a single scopeTemplate
	clusterRoleGenerateKey = "operators.coreos.io/generateName"
	stCtrlFieldOwner       = "scopetemplate-controller"
)

//+kubebuilder:rbac:groups=operators.io.operator-framework,resources=scopetemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operators.io.operator-framework,resources=scopetemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operators.io.operator-framework,resources=scopetemplates/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ScopeTemplate object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *ScopeTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	log.Log.Info("Reconciling ScopeTemplate")

	// get the scope template
	existingSt := &operatorsv1.ScopeTemplate{}
	if err := r.Client.Get(ctx, req.NamespacedName, existingSt); err != nil {
		return ctrl.Result{}, err
	}

	// Perform reconciliation
	reconciledSt := existingSt.DeepCopy()
	res, reconcileErr := r.reconcile(ctx, reconciledSt)

	// Update the status subresource before updating the main object. This is
	// necessary because, in many cases, the main object update will remove the
	// finalizer, which will cause the core Kubernetes deletion logic to
	// complete. Therefore, we need to make the status update prior to the main
	// object update to ensure that the status update can be processed before
	// a potential deletion.
	if !equality.Semantic.DeepEqual(existingSt.Status, reconciledSt.Status) {
		if updateErr := r.Client.Status().Update(ctx, reconciledSt); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	existingSt.Status, reconciledSt.Status = operatorsv1.ScopeTemplateStatus{}, operatorsv1.ScopeTemplateStatus{}
	if !equality.Semantic.DeepEqual(existingSt, reconciledSt) {
		if updateErr := r.Client.Update(ctx, reconciledSt); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	return res, reconcileErr
}

func (r *ScopeTemplateReconciler) reconcile(ctx context.Context, st *operatorsv1.ScopeTemplate) (ctrl.Result, error) {
	scopeinstances := operatorsv1.ScopeInstanceList{}
	if err := r.Client.List(ctx, &scopeinstances, &client.ListOptions{}); err != nil {
		return ctrl.Result{}, err
	}

	for _, sInstance := range scopeinstances.Items {
		if sInstance.Spec.ScopeTemplateName != st.Name {
			continue
		}
		// create ClusterRoles based on the ScopeTemplate
		log.Log.Info("ScopeInstance found that references ScopeTemplate", "name", st.Name)
		if err := r.ensureClusterRoles(ctx, st); err != nil {
			return ctrl.Result{}, fmt.Errorf("creating ClusterRoles: %v", err)
		}
	}

	// Add requirement to delete old hashes
	stHashReq, err := labels.NewRequirement(scopeTemplateHashKey, selection.NotEquals, []string{util.HashObject(st.Spec)})
	if err != nil {
		return ctrl.Result{}, err
	}

	// Only look for old clusterroles that map to this ScopeTemplate UID
	stUIDReq, err := labels.NewRequirement(scopeTemplateUIDKey, selection.Equals, []string{string(st.GetUID())})
	if err != nil {
		return ctrl.Result{}, err
	}

	listOptions := &client.ListOptions{
		LabelSelector: labels.NewSelector().Add(*stHashReq, *stUIDReq),
	}

	if err := r.deleteClusterRoles(ctx, listOptions); err != nil {
		return ctrl.Result{}, err
	}

	log.Log.Info("No ScopeTemplate error")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScopeTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorsv1.ScopeTemplate{}).
		// Set up a watch for ScopeInstance to handle requeuing of requests for ScopeTemplate
		Watches(&source.Kind{Type: &operatorsv1.ScopeInstance{}}, handler.EnqueueRequestsFromMapFunc(r.mapToScopeTemplate)).
		Complete(r)
}

func (r *ScopeTemplateReconciler) mapToScopeTemplate(obj client.Object) (requests []reconcile.Request) {
	if obj == nil || obj.GetName() == "" {
		return
	}

	ctx := context.TODO()
	//(todo): Check if obj can be converted into a scope instance.
	scopeInstance := &operatorsv1.ScopeInstance{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: obj.GetName()}, scopeInstance); err != nil {
		return nil
	}

	// Exit early if scopeInstance doesn't reference a scopeTemplate
	if scopeInstance.Spec.ScopeTemplateName == "" {
		return nil
	}

	// enqueue requests for ScopeTemplate based on Name and Namespace
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: obj.GetNamespace(), Name: scopeInstance.Spec.ScopeTemplateName},
	}
	requests = append(requests, request)

	return requests
}

func (r *ScopeTemplateReconciler) ensureClusterRoles(ctx context.Context, st *operatorsv1.ScopeTemplate) error {
	for _, cr := range st.Spec.ClusterRoles {
		clusterRole := r.getClusterRole(&cr, st)

		crList := &rbacv1.ClusterRoleList{}
		if err := r.Client.List(ctx, crList, client.MatchingLabels{
			scopeTemplateUIDKey:    string(st.GetUID()),
			clusterRoleGenerateKey: cr.GenerateName,
		}); err != nil {
			return err
		}

		// TODO: here to compare the clusterRoles against the expected values.
		if len(crList.Items) > 1 {
			return fmt.Errorf("more than one ClusterRole found %s", cr.GenerateName)
		}

		// GenerateName is immutable, so create the object if it has changed
		if len(crList.Items) == 0 {
			if err := r.Client.Create(ctx, clusterRole); err != nil {
				return err
			}
			continue
		}

		existingCR := &crList.Items[0]

		if util.IsOwnedByLabel(existingCR.DeepCopy(), st) &&
			reflect.DeepEqual(existingCR.Rules, clusterRole.Rules) &&
			reflect.DeepEqual(existingCR.Labels, clusterRole.Labels) {
			log.Log.Info("existing ClusterRole does not need to be updated")
			return nil
		}

		u, err := r.patchConfigForClusterRole(existingCR, clusterRole)
		if err != nil {
			return err
		}

		// server-side apply patch
		if err := r.Client.Patch(ctx,
			u,
			client.Apply,
			client.FieldOwner(stCtrlFieldOwner),
			client.ForceOwnership); err != nil {
			return err
		}
	}
	return nil
}

func (r *ScopeTemplateReconciler) patchConfigForClusterRole(oldCr *rbacv1.ClusterRole, cr *rbacv1.ClusterRole) (*unstructured.Unstructured, error) {
	crAc := rbacv1ac.ClusterRole(oldCr.Name).WithLabels(cr.Labels)
	rulesAc := []rbacv1ac.PolicyRuleApplyConfiguration{}
	orAcs := []metav1ac.OwnerReferenceApplyConfiguration{}

	for _, rule := range cr.Rules {
		ruleAc := *rbacv1ac.PolicyRule().WithAPIGroups(rule.APIGroups...).WithNonResourceURLs(rule.NonResourceURLs...).WithResourceNames(rule.ResourceNames...).WithResources(rule.Resources...).WithVerbs(rule.Verbs...)
		rulesAc = append(rulesAc, ruleAc)
	}

	for _, own := range cr.OwnerReferences {
		ownAc := *metav1ac.OwnerReference().WithAPIVersion(own.APIVersion).WithKind(own.Kind).WithName(own.Name).WithUID(own.UID)
		orAcs = append(orAcs, ownAc)
	}

	crAc.Rules = rulesAc
	crAc.OwnerReferences = orAcs

	uMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(crAc)
	if err != nil {
		return nil, err
	}

	return &unstructured.Unstructured{Object: uMap}, nil
}

func (r *ScopeTemplateReconciler) deleteClusterRoles(ctx context.Context, listOptions ...client.ListOption) error {
	clusterRoles := &rbacv1.ClusterRoleList{}
	if err := r.Client.List(ctx, clusterRoles, listOptions...); err != nil {
		// TODO: Aggregate errors
		return err
	}

	for _, crb := range clusterRoles.Items {
		// TODO: Aggregate errors
		if err := r.Client.Delete(ctx, &crb); err != nil && !k8sapierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *ScopeTemplateReconciler) getClusterRole(crt *operatorsv1.ClusterRoleTemplate, st *operatorsv1.ScopeTemplate) *rbacv1.ClusterRole {
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: crt.GenerateName,
			Labels: map[string]string{
				scopeTemplateUIDKey:    string(st.GetUID()),
				scopeTemplateHashKey:   util.HashObject(st.Spec),
				clusterRoleGenerateKey: crt.GenerateName,
			},
		},
		Rules: crt.Rules,
	}

	ctrl.SetControllerReference(st, cr, r.Scheme)
	return cr
}

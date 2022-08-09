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

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorsv1 "awgreene/scope-operator/api/v1"
)

// ScopeTemplateReconciler reconciles a ScopeTemplate object
type ScopeTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

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
	in := &operatorsv1.ScopeTemplate{}
	if err := r.Client.Get(ctx, req.NamespacedName, in); err != nil {
		return ctrl.Result{}, err
	}

	log.Log.Info("Getting ScopeTemplate", "name", in.Name)

	scopeinstances := operatorsv1.ScopeInstanceList{}

	if err := r.Client.List(ctx, &scopeinstances, client.InNamespace(req.Namespace)); err != nil {
		return ctrl.Result{}, fmt.Errorf("list scope instances: %v", err)
	}

	for i := range scopeinstances.Items {
		sInstance := scopeinstances.Items[i]
		if sInstance.Spec.ScopeTemplateName == in.Name {
			log.Log.Info("ScopeInstance found that references ScopeTemplate", "name", in.Name)
			// (todo): only create the associated cluster roles if a ScopeInstance exists that references ScopeTemplate
		}
	}

	// siKey := types.NamespacedName{
	// 	Namespace: req.Namespace,
	// 	Name:      "scopeinstance-sample",
	// }

	// get the scope instance
	// sInstance := &operatorsv1.ScopeInstance{}
	// if err := r.Client.Get(ctx, siKey, sInstance); err != nil {
	// 	return ctrl.Result{}, err
	// }

	// log.Log.Info("Getting ScopeInstance", "name", sInstance.Name)

	var clusterRole *rbacv1.ClusterRole
	// Create all the clusterRoles
	for _, cr := range in.Spec.ClusterRoles {
		clusterRole = &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: cr.GenerateName,
			},
			Rules: cr.Rules,
		}

		// make scope template controller the owner of cluster role object
		if err := controllerutil.SetOwnerReference(in, clusterRole, r.Scheme); err != nil {
			return ctrl.Result{}, fmt.Errorf("error setting owner reference: %w", err)
		}

		log.Log.Info("Creating ClusterRole", "name", clusterRole.Name)
		if err := r.Client.Create(ctx, clusterRole); err != nil {
			log.Log.Error(err, "Failed to create ClusterRole", "name", clusterRole.GenerateName)
			if errors.IsAlreadyExists(err) {
				log.Log.Info("ClusterRole already exists", "name", clusterRole.Name)
			}
		}
	}

	crKey := types.NamespacedName{
		Namespace: req.Namespace,
		Name:      clusterRole.Name,
	}

	// get cluster role associated with the scope template
	cRole := &rbacv1.ClusterRole{}
	if err := r.Client.Get(ctx, crKey, cRole); err != nil {
		return ctrl.Result{}, fmt.Errorf("get %q: %v", cRole.Name, err)
	}
	log.Log.Info("Getting ClusterRole", "name", cRole.Name)

	// (todo): I think setting OwnerReference will take care of deleting associated cluster roles when ScopeTemplate gets deleted, not sure if we need to add delete call explicitly.
	// delete cluster role associated with the scope template
	log.Log.Info("Deleting ClusterRole associated with the ScopeTemplate", "name", cRole.Name)
	if err := r.Client.Delete(ctx, cRole); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("delete %q: %v", cRole.Name, err)
	}

	log.Log.Info("No ScopeTemplate error")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScopeTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueueScopeTemplate := handler.EnqueueRequestsFromMapFunc(r.mapToScopeTemplate)
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorsv1.ScopeTemplate{}).
		// Watch for changes to cluster roles created by Scope Template and trigger a Reconcile for the owner ScopeTemplate
		// Watches(&source.Kind{Type: &operatorsv1.ScopeTemplate{}}, &handler.EnqueueRequestForOwner{
		// 	OwnerType:    &operatorsv1.ScopeTemplate{},
		// 	IsController: true}).
		Watches(&source.Kind{Type: &operatorsv1.ScopeInstance{}}, enqueueScopeTemplate).
		Complete(r)
}

func (r *ScopeTemplateReconciler) mapToScopeTemplate(obj client.Object) (requests []reconcile.Request) {
	if obj == nil || obj.GetName() == "" {
		return
	}

	// (todo): Check if obj can be converted into a scope instance.
	// sInstance, ok := obj.(*operatorsv1.ScopeInstance)
	// if !ok {
	// 	return nil
	// }

	ctx := context.TODO()
	scopeInstance := &operatorsv1.ScopeInstance{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: scopeInstance.Spec.ScopeTemplateName}, scopeInstance); err != nil {
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

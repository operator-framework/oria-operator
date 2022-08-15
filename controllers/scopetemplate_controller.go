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
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
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

	// create ClusterRoles based on the ScopeTemplate
	if err := r.createClusterRoles(ctx, in); err != nil {
		return ctrl.Result{}, fmt.Errorf("create ClusterRoles: %v", err)
	}

	// get the scope template to make sure it doesn't exist before deleting all of its associated ClusterRoles.
	sTemplate := &operatorsv1.ScopeTemplate{}
	if err := r.Client.Get(ctx, req.NamespacedName, sTemplate); err != nil {
		if !k8sapierrors.IsNotFound(err) {
			// Delete ClusterRoles associated with ScopeTemplate if/when the ScopeTemplate gets deleted
			if err := r.deleteClusterRoles(ctx, in); err != nil {
				return ctrl.Result{}, fmt.Errorf("delete ClusterRoles: %v", err)
			}
		}
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
	// (todo): Check if obj can be converted into a scope instance.
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

func (r *ScopeTemplateReconciler) createClusterRoles(ctx context.Context, sTemplate *operatorsv1.ScopeTemplate) error {
	scopeinstances := operatorsv1.ScopeInstanceList{}

	if err := r.Client.List(ctx, &scopeinstances, client.InNamespace(sTemplate.Namespace)); err != nil {
		return fmt.Errorf("list scope instances: %v", err)
	}

	var clusterRole *rbacv1.ClusterRole
	// Create all the ClusterRoles
	for i := range scopeinstances.Items {
		sInstance := scopeinstances.Items[i]
		if sInstance.Spec.ScopeTemplateName == sTemplate.Name {
			log.Log.Info("ScopeInstance found that references ScopeTemplate", "name", sTemplate.Name)
			for _, cr := range sTemplate.Spec.ClusterRoles {
				clusterRole = &rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: cr.GenerateName,
					},
					Rules: cr.Rules,
				}

				// make scope template controller the owner of cluster role object
				if err := controllerutil.SetOwnerReference(sTemplate, clusterRole, r.Scheme); err != nil {
					return fmt.Errorf("error setting owner reference: %w", err)
				}

				// (todo): If ClusterRole already exists, then don't create it, just update it.
				log.Log.Info("Creating ClusterRole", "name", clusterRole.Name)
				if err := r.Client.Create(ctx, clusterRole); err != nil {
					log.Log.Error(err, "Failed to create ClusterRole", "name", clusterRole.GenerateName)
					if errors.IsAlreadyExists(err) {
						log.Log.Info("ClusterRole already exists", "name", clusterRole.Name)
					}
				}
			}
		}
	}
	return nil
}

func (r *ScopeTemplateReconciler) deleteClusterRoles(ctx context.Context, sTemplate *operatorsv1.ScopeTemplate) error {
	scopeinstances := &operatorsv1.ScopeInstanceList{}
	if err := r.Client.List(ctx, scopeinstances); err != nil {
		return fmt.Errorf("list scope instances: %v", err)
	}

	for _, sInstance := range scopeinstances.Items {
		if sInstance.Spec.ScopeTemplateName == sTemplate.Name {
			log.Log.Info("ScopeInstance found that references ScopeTemplate", "name", sTemplate.Name)
			continue
		}

		for _, cr := range sTemplate.Spec.ClusterRoles {
			crKey := types.NamespacedName{
				Namespace: sTemplate.Namespace,
				Name:      cr.GenerateName,
			}

			// get ClusterRole associated with the ScopeTemplate.
			cRole := &rbacv1.ClusterRole{}
			if err := r.Client.Get(ctx, crKey, cRole); err != nil {
				return fmt.Errorf("get ClusterRole %q: %v", cRole.Name, err)
			}
			log.Log.Info("Got ClusterRole", "name", cRole.Name)

			// delete ClusterRole associated with the ScopeTemplate.
			log.Log.Info("Deleting ClusterRole associated with the ScopeTemplate", "name", cRole.Name)
			if err := r.Client.Delete(ctx, cRole); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete %q: %v", cRole.Name, err)
			}
		}
	}

	return nil
}

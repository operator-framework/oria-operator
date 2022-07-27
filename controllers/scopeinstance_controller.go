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

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorsv1 "awgreene/scope-operator/api/v1"
)

// ScopeInstanceReconciler reconciles a ScopeInstance object
type ScopeInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=operators.io.operator-framework,resources=scopeinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operators.io.operator-framework,resources=scopeinstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operators.io.operator-framework,resources=scopeinstances/finalizers,verbs=update

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

	log.Log.Info("Reconciling ScopeInstance")

	in := &operatorsv1.ScopeInstance{}
	if err := r.Client.Get(ctx, req.NamespacedName, in); err != nil {
		return ctrl.Result{}, err
	}

	// get 	the scope template
	st := &operatorsv1.ScopeTemplate{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: in.Spec.ScopeTemplateName}, st); err != nil {
		return ctrl.Result{}, err
	}

	log.Log.Info("ScopeTemplate found", "name", st.Name)

	objects := []client.Object{}
	if len(in.Spec.Namespaces) == 0 {
		for _, cr := range st.Spec.ClusterRoles {
			objects = append(objects, &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: cr.GenerateName + "-",
				},
				Subjects: cr.Subjects,
				RoleRef: rbacv1.RoleRef{
					Kind: "ClusterRole",
					Name: cr.GenerateName,
				},
			})
		}
	} else {
		for _, namespace := range in.Spec.Namespaces {
			for _, cr := range st.Spec.ClusterRoles {
				objects = append(objects, &rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: cr.GenerateName + "-",
						Namespace:    namespace,
					},
					Subjects: cr.Subjects,
					RoleRef: rbacv1.RoleRef{
						Kind: "ClusterRole",
						Name: cr.GenerateName,
					},
				})
			}
		}

	}

	for _, object := range objects {
		log.Log.Info("Creating object", "object", object)
		if err := r.Client.Create(ctx, object); err != nil {
			log.Log.Error(err, "Error creating object", "object", object)
			/// TODO: try to create all the (cluster)roleBindings before returning an aggregate error.
		}
	}

	log.Log.Info("No ScopeInstance error")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScopeInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorsv1.ScopeInstance{}).
		Complete(r)
}

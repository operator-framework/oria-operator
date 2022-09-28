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
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
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
	// UID keys are used to track "owners" of bindings we create.
	scopeInstanceUIDKey = "operators.coreos.io/scopeInstanceUID"
	scopeTemplateUIDKey = "operators.coreos.io/scopeTemplateUID"

	// Hash keys are used to track "abandoned" bindings we created.
	scopeInstanceHashKey = "operators.coreos.io/scopeInstanceHash"
	scopeTemplateHashKey = "operators.coreos.io/scopeTemplateHash"

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

	log.Log.Info("Reconciling ScopeInstance", "namespaceName", req.NamespacedName)

	in := &operatorsv1.ScopeInstance{}
	if err := r.Client.Get(ctx, req.NamespacedName, in); err != nil {
		return ctrl.Result{}, err
	}

	st := &operatorsv1.ScopeTemplate{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: in.Spec.ScopeTemplateName}, st); err != nil {
		if !k8sapierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		cErr := r.updateScopeInstanceCondition(ctx, in, metav1.Condition{
			Type:               operatorsv1.TypeScoped,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: in.Generation,
			Reason:             operatorsv1.ReasonScopeTemplateNotFound,
			Message:            fmt.Sprintf("getting ScopeTemplate %q: %s", in.Spec.ScopeTemplateName, err),
		})
		if cErr != nil {
			return ctrl.Result{Requeue: true}, cErr
		}

		// Delete anything owned by the scopeInstance if the scopeTemplate is gone.
		listOption := client.MatchingLabels{
			scopeInstanceUIDKey: string(in.GetUID()),
		}

		if err := r.deleteBindings(ctx, listOption); err != nil {
			cErr := r.updateScopeInstanceCondition(ctx, in, metav1.Condition{
				Type:               operatorsv1.TypeScoped,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: in.Generation,
				Reason:             operatorsv1.ReasonScopingFailed,
				Message:            fmt.Sprintf("deleting RoleBindings associated with ScopeInstance %q: %v", in.Name, err),
			})
			if cErr != nil {
				return ctrl.Result{Requeue: true}, cErr
			}
			log.Log.Error(err, "in deleting Role Bindings")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// create required roleBindings and clusterRoleBindings.
	if err := r.ensureBindings(ctx, in, st); err != nil {
		cErr := r.updateScopeInstanceCondition(ctx, in, metav1.Condition{
			Type:               operatorsv1.TypeScoped,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: in.Generation,
			Reason:             operatorsv1.ReasonScopingFailed,
			Message:            fmt.Sprintf("creating RoleBindings for ScopeInstance %q: %v", in.Name, err),
		})
		if cErr != nil {
			return ctrl.Result{Requeue: true}, cErr
		}
		log.Log.Error(err, "in creating Role Bindings")
		return ctrl.Result{}, err
	}

	listOption := client.MatchingLabels{
		scopeInstanceUIDKey: string(in.GetUID()),
	}

	requirement, err := labels.NewRequirement(scopeInstanceHashKey, selection.NotEquals, []string{util.HashObject(in.Spec)})
	if err != nil {
		cErr := r.updateScopeInstanceCondition(ctx, in, metav1.Condition{
			Type:               operatorsv1.TypeScoped,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: in.Generation,
			Reason:             operatorsv1.ReasonScopingFailed,
			Message:            fmt.Sprintf("creating a new requirement label associated with ScopeInstance %q: %v", in.Name, err),
		})
		if cErr != nil {
			return ctrl.Result{Requeue: true}, cErr
		}
		return ctrl.Result{}, err
	}

	listOptions := &client.ListOptions{
		LabelSelector: labels.NewSelector().Add(*requirement),
	}

	if err := r.deleteBindings(ctx, listOption, listOptions); err != nil {
		cErr := r.updateScopeInstanceCondition(ctx, in, metav1.Condition{
			Type:               operatorsv1.TypeScoped,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: in.Generation,
			Reason:             operatorsv1.ReasonScopingFailed,
			Message:            fmt.Sprintf("deleting RoleBindings associated with ScopeInstance %q: %v", in.Name, err),
		})
		if cErr != nil {
			return ctrl.Result{Requeue: true}, cErr
		}
		log.Log.Error(err, "in deleting Role Bindings")
		return ctrl.Result{}, err
	}

	// TODO: Find out how to merge with the above delete
	listOption = client.MatchingLabels{
		scopeInstanceUIDKey: string(in.GetUID()),
		scopeTemplateUIDKey: string(st.GetUID()),
	}

	requirement, err = labels.NewRequirement(scopeTemplateHashKey, selection.NotEquals, []string{util.HashObject(st.Spec)})
	if err != nil {
		cErr := r.updateScopeInstanceCondition(ctx, in, metav1.Condition{
			Type:               operatorsv1.TypeScoped,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: in.Generation,
			Reason:             operatorsv1.ReasonScopingFailed,
			Message:            fmt.Sprintf("creating a new requirement label associated with ScopeInstance %q: %v", in.Name, err),
		})
		if cErr != nil {
			return ctrl.Result{Requeue: true}, cErr
		}
		return ctrl.Result{}, err
	}

	listOptions = &client.ListOptions{
		LabelSelector: labels.NewSelector().Add(*requirement),
	}

	if err := r.deleteBindings(ctx, listOption, listOptions); err != nil {
		cErr := r.updateScopeInstanceCondition(ctx, in, metav1.Condition{
			Type:               operatorsv1.TypeScoped,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: in.Generation,
			Reason:             operatorsv1.ReasonScopingFailed,
			Message:            fmt.Sprintf("deleting RoleBindings associated with ScopeInstance %q: %v", in.Name, err),
		})
		if cErr != nil {
			return ctrl.Result{Requeue: true}, cErr
		}
		log.Log.Error(err, "in deleting Role Bindings")
		return ctrl.Result{}, err
	}

	log.Log.Info("No ScopeInstance error")

	cErr := r.updateScopeInstanceCondition(ctx, in, metav1.Condition{
		Type:               operatorsv1.TypeScoped,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: st.Generation,
		Reason:             operatorsv1.ReasonScopingSuccessful,
		Message:            fmt.Sprintf("ScopeInstance %q successfully reconciled", in.Name),
	})
	if cErr != nil {
		return ctrl.Result{Requeue: true}, cErr
	}

	return ctrl.Result{}, nil
}

func (r *ScopeInstanceReconciler) ensureBindings(ctx context.Context, in *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate) error {
	// it will create clusterrole as shown below if no namespace is provided
	// TODO: refactor code to handle both roleBindings and clusterRoleBindings
	if len(in.Spec.Namespaces) == 0 {
		for _, cr := range st.Spec.ClusterRoles {
			crb := &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: cr.GenerateName + "-",
					Labels: map[string]string{
						scopeInstanceUIDKey:           string(in.GetUID()),
						scopeTemplateUIDKey:           string(st.GetUID()),
						scopeInstanceHashKey:          util.HashObject(in.Spec),
						scopeTemplateHashKey:          util.HashObject(st.Spec),
						clusterRoleBindingGenerateKey: cr.GenerateName,
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: in.APIVersion,
						Kind:       in.Kind,
						Name:       in.GetObjectMeta().GetName(),
						UID:        in.GetObjectMeta().GetUID(),
					}},
				},
				Subjects: cr.Subjects,
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					Name:     cr.GenerateName,
					APIGroup: "rbac.authorization.k8s.io",
				},
			}

			crbList := &rbacv1.ClusterRoleBindingList{}
			if err := r.Client.List(ctx, crbList, client.MatchingLabels{
				scopeInstanceUIDKey:           string(in.GetUID()),
				scopeTemplateUIDKey:           string(st.GetUID()),
				clusterRoleBindingGenerateKey: cr.GenerateName,
			}); err != nil {
				return err
			}

			if len(crbList.Items) > 1 {
				return fmt.Errorf("more than one ClusterRoleBinding found for ClusterRole %s", cr.GenerateName)
			}

			// GenerateName is immutable, so create the object if it has changed
			if len(crbList.Items) == 0 {
				if err := r.Client.Create(ctx, crb); err != nil {
					return err
				}
				continue
			}

			existingCRB := &crbList.Items[0]

			if util.IsOwnedByLabel(existingCRB.DeepCopy(), in) &&
				reflect.DeepEqual(existingCRB.Subjects, crb.Subjects) &&
				reflect.DeepEqual(existingCRB.Labels, crb.Labels) {
				log.Log.Info("existing ClusterRoleBinding does not need to be updated")
				return nil
			}
			existingCRB.Labels = crb.Labels
			existingCRB.OwnerReferences = crb.OwnerReferences
			existingCRB.Subjects = crb.Subjects

			// server-side apply patch
			existingCRB.ManagedFields = nil
			if err := r.Client.Patch(ctx,
				existingCRB,
				client.Apply,
				client.FieldOwner(siCtrlFieldOwner),
				client.ForceOwnership); err != nil {
				return err
			}

		}
	} else {
		// it will iterate over the namespace and createrole bindings for each cluster roles
		for _, namespace := range in.Spec.Namespaces {
			for _, cr := range st.Spec.ClusterRoles {
				rb := &rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: cr.GenerateName + "-",
						Namespace:    namespace,
						Labels: map[string]string{
							scopeInstanceUIDKey:           string(in.GetUID()),
							scopeTemplateUIDKey:           string(st.GetUID()),
							scopeInstanceHashKey:          util.HashObject(in.Spec),
							scopeTemplateHashKey:          util.HashObject(st.Spec),
							clusterRoleBindingGenerateKey: cr.GenerateName,
						},
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: in.APIVersion,
							Kind:       in.Kind,
							Name:       in.GetObjectMeta().GetName(),
							UID:        in.GetObjectMeta().GetUID(),
						}},
					},
					Subjects: cr.Subjects,
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     cr.GenerateName,
						APIGroup: "rbac.authorization.k8s.io",
					},
				}

				rbList := &rbacv1.RoleBindingList{}
				if err := r.Client.List(ctx, rbList, &client.ListOptions{
					Namespace: namespace,
				}, client.MatchingLabels{
					scopeInstanceUIDKey:           string(in.GetUID()),
					scopeTemplateUIDKey:           string(st.GetUID()),
					clusterRoleBindingGenerateKey: cr.GenerateName,
				}); err != nil {
					return err
				}

				if len(rbList.Items) > 1 {
					return fmt.Errorf("more than one roleBinding found for ClusterRole %s", cr.GenerateName)
				}

				// GenerateName is immutable, so create the object if it has changed
				if len(rbList.Items) == 0 {
					if err := r.Client.Create(ctx, rb); err != nil {
						return err
					}
					continue
				}

				log.Log.Info("Updating existing rb", "namespaced", rbList.Items[0].GetNamespace(), "name", rbList.Items[0].GetName())

				existingRB := &rbList.Items[0]

				if util.IsOwnedByLabel(existingRB.DeepCopy(), in) &&
					reflect.DeepEqual(existingRB.Subjects, rb.Subjects) &&
					reflect.DeepEqual(existingRB.Labels, rb.Labels) {
					log.Log.Info("existing ClusterRoleBinding does not need to be updated")
					return nil
				}
				existingRB.Labels = rb.Labels
				existingRB.OwnerReferences = rb.OwnerReferences
				existingRB.Subjects = rb.Subjects

				// server-side apply patch
				existingRB.ManagedFields = nil
				if err := r.Client.Patch(ctx,
					existingRB,
					client.Apply,
					client.FieldOwner(siCtrlFieldOwner),
					client.ForceOwnership); err != nil {
					return err
				}
			}
		}
	}

	return nil
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

// SetupWithManager sets up the controller with the Manager.
func (r *ScopeInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorsv1.ScopeInstance{}).
		Watches(&source.Kind{Type: &operatorsv1.ScopeTemplate{}}, handler.EnqueueRequestsFromMapFunc(r.mapToScopeInstance)).
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

func (r *ScopeInstanceReconciler) updateScopeInstanceCondition(ctx context.Context, si *operatorsv1.ScopeInstance, condition metav1.Condition) error {
	// Update the condition of the ScopeInstance
	meta.SetStatusCondition(&si.Status.Conditions, condition)
	si.ManagedFields = nil
	return r.Status().Patch(ctx,
		si,
		client.Apply,
		client.FieldOwner(siCtrlFieldOwner),
		client.ForceOwnership)
}

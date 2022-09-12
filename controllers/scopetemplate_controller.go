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

	operatorsv1 "awgreene/scope-operator/api/v1"
	"awgreene/scope-operator/util"

	"github.com/sirupsen/logrus"
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

// ScopeTemplateReconciler reconciles a ScopeTemplate object
type ScopeTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	logger *logrus.Logger
}

const (
	// generateNames are used to track each binding we create for a single scopeTemplate
	clusterRoleGenerateKey = "operators.coreos.io/generateName"
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
	st := &operatorsv1.ScopeTemplate{}
	if err := r.Client.Get(ctx, req.NamespacedName, st); err != nil {
		return ctrl.Result{}, err
	}

	scopeinstances := operatorsv1.ScopeInstanceList{}

	if err := r.Client.List(ctx, &scopeinstances, &client.ListOptions{}); err != nil {
		cErr := r.updateScopeTemplateCondition(ctx, st, metav1.Condition{
			Type:               operatorsv1.TypeTemplated,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: st.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             operatorsv1.ReasonTemplatingFailed,
			Message:            fmt.Sprintf("listing ScopeInstances: %s", err),
		})
		if cErr != nil {
			return ctrl.Result{Requeue: true}, cErr
		}

		return ctrl.Result{}, err
	}

	listOptions := []client.ListOption{
		&client.MatchingLabels{
			scopeTemplateUIDKey: string(st.GetUID()),
		},
	}

	for _, sInstance := range scopeinstances.Items {
		if sInstance.Spec.ScopeTemplateName != st.Name {
			continue
		}
		// create ClusterRoles based on the ScopeTemplate
		log.Log.Info("ScopeInstance found that references ScopeTemplate", "name", st.Name)
		if err := r.ensureClusterRoles(ctx, st); err != nil {
			cErr := r.updateScopeTemplateCondition(ctx, st, metav1.Condition{
				Type:               operatorsv1.TypeTemplated,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: st.Generation,
				LastTransitionTime: metav1.Now(),
				Reason:             operatorsv1.ReasonTemplatingFailed,
				Message:            fmt.Sprintf("creating ClusterRoles: %s", err),
			})
			if cErr != nil {
				return ctrl.Result{Requeue: true}, cErr
			}
			return ctrl.Result{}, fmt.Errorf("Error in create ClusterRoles: %v", err)
		}

		// Add requirement to delete old hashes
		requirement, err := labels.NewRequirement(scopeTemplateHashKey, selection.NotEquals, []string{util.HashObject(st.Spec)})
		if err != nil {
			cErr := r.updateScopeTemplateCondition(ctx, st, metav1.Condition{
				Type:               operatorsv1.TypeTemplated,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: st.Generation,
				LastTransitionTime: metav1.Now(),
				Reason:             operatorsv1.ReasonTemplatingFailed,
				Message:            fmt.Sprintf("adding a requirement to delete old hashes: %s", err),
			})
			if cErr != nil {
				return ctrl.Result{Requeue: true}, cErr
			}
			return ctrl.Result{}, err
		}
		listOptions = append(listOptions, &client.ListOptions{
			LabelSelector: labels.NewSelector().Add(*requirement),
		})
		break
	}

	if err := r.deleteClusterRoles(ctx, listOptions...); err != nil {
		cErr := r.updateScopeTemplateCondition(ctx, st, metav1.Condition{
			Type:               operatorsv1.TypeTemplated,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: st.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             operatorsv1.ReasonTemplatingFailed,
			Message:            fmt.Sprintf("deleting ClusterRoles: %s", err),
		})
		if cErr != nil {
			return ctrl.Result{Requeue: true}, cErr
		}
		return ctrl.Result{}, err
	}

	log.Log.Info("No ScopeTemplate error")

	cErr := r.updateScopeTemplateCondition(ctx, st, metav1.Condition{
		Type:               operatorsv1.TypeTemplated,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: st.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             operatorsv1.ReasonTemplatingSuccessful,
		Message:            "ScopeTemplate successfully reconciled",
	})
	if cErr != nil {
		return ctrl.Result{Requeue: true}, cErr
	}

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
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: cr.GenerateName,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: st.APIVersion,
					Kind:       st.Kind,
					Name:       st.GetObjectMeta().GetName(),
					UID:        st.GetObjectMeta().GetUID(),
				}},
				Labels: map[string]string{
					scopeTemplateUIDKey:    string(st.GetUID()),
					scopeTemplateHashKey:   util.HashObject(st.Spec),
					clusterRoleGenerateKey: cr.GenerateName,
				},
			},
			Rules: cr.Rules,
		}

		crbList := &rbacv1.ClusterRoleList{}
		if err := r.Client.List(ctx, crbList, client.MatchingLabels{
			scopeTemplateUIDKey:    string(st.GetUID()),
			clusterRoleGenerateKey: cr.GenerateName,
		}); err != nil {
			return err
		}

		// TODO: here to compare the clusterRoles against the expected values.
		if len(crbList.Items) > 1 {
			return fmt.Errorf("more than one ClusterRole found %s", cr.GenerateName)
		}

		// GenerateName is immutable, so create the object if it has changed
		if len(crbList.Items) == 0 {
			if err := r.Client.Create(ctx, clusterRole); err != nil {
				return err
			}
			continue
		}

		existingCRB := &crbList.Items[0]

		if util.IsOwnedByLabel(existingCRB.DeepCopy(), st) &&
			reflect.DeepEqual(existingCRB.Rules, clusterRole.Rules) &&
			reflect.DeepEqual(existingCRB.Labels, clusterRole.Labels) {
			r.logger.Debug("Existing ClusterRoleBinding does not need to be updated")
			return nil
		}
		existingCRB.Labels = clusterRole.Labels
		existingCRB.OwnerReferences = clusterRole.OwnerReferences
		existingCRB.Rules = clusterRole.Rules

		if err := r.Client.Update(ctx, existingCRB); err != nil {
			return err
		}
	}
	return nil
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

func (r *ScopeTemplateReconciler) updateScopeTemplateCondition(ctx context.Context, st *operatorsv1.ScopeTemplate, condition metav1.Condition) error {
	// Update the condition of the ScopeTemplate
	meta.SetStatusCondition(&st.Status.Conditions, condition)
	st.ManagedFields = nil
	return r.Status().Patch(ctx, st, client.Apply, client.FieldOwner("scopetemplate-controller"), client.ForceOwnership)
}

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
	"reflect"
	"strings"

	operatorsv1 "awgreene/scope-operator/api/v1"
	"awgreene/scope-operator/util"

	"github.com/sirupsen/logrus"
	rbacv1 "k8s.io/api/rbac/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	logger *logrus.Logger
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

	log.Log.Info("Reconciling ScopeInstance", "namespaceName", req.NamespacedName)

	in := &operatorsv1.ScopeInstance{}
	if err := r.Client.Get(ctx, req.NamespacedName, in); err != nil {
		return ctrl.Result{}, err
	}

	st := &operatorsv1.ScopeTemplate{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: in.Spec.ScopeTemplateName}, st); err != nil {
		if !k8sapierrors.IsNotFound(err) {

			//Delete any RBAC owned by SI CR.
			log.Log.Info("ScopeInstance .Spec.ScopeTemplateName references non-existent ScopeTemplate", "scopeTemplateName", st.GetName())
			roleBindingMap, clusterRoleBindingMap, err := r.getListOfExistingRBAC(ctx, in)
			if err != nil {
				log.Log.Info("Error getting object", "error", err)
				return ctrl.Result{}, err
			}

			if err := r.cleanupInvalidRBAC(ctx, clusterRoleBindingMap, roleBindingMap); err != nil {
				log.Log.Info("Error in deleting Role Bindings", "error", err)
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, err
	}

	roleBindingMap, clusterRoleBindingMap, err := r.getListOfExistingRBAC(ctx, in)
	if err != nil {
		log.Log.Info("Error getting object", "error", err)
		return ctrl.Result{}, err
	}

	// before create and delete loop for update and if it is updated then remove from map and
	// no need to delete those rbac
	if err := r.ensureRBAC(ctx, in, st); err != nil {
		log.Log.Info("Error in creating Role Bindings", "error", err)
		return ctrl.Result{}, err
	}

	if err := r.ensureUpdateOfRBAC(ctx, in, st, clusterRoleBindingMap, roleBindingMap); err != nil {
		log.Log.Info("Error in Updating Role Bindings", "error", err)
		return ctrl.Result{}, err
	}

	if err := r.cleanupInvalidRBAC(ctx, clusterRoleBindingMap, roleBindingMap); err != nil {
		log.Log.Info("Error in deleting Role Bindings", "error", err)
		return ctrl.Result{}, err
	}

	log.Log.Info("No ScopeInstance error")

	return ctrl.Result{}, nil
}

func (r *ScopeInstanceReconciler) getListOfExistingRBAC(ctx context.Context, in *operatorsv1.ScopeInstance) (map[string]rbacv1.RoleBinding, map[string]rbacv1.ClusterRoleBinding, error) {
	existingRoleBindingList := &rbacv1.RoleBindingList{}
	existingClusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}

	// Get the existing ClusterRoleBinding
	if err := r.Client.List(ctx, existingClusterRoleBindingList, client.MatchingLabels{
		"operators.coreos.io/scopeInstance": string(in.GetUID()),
	}); err != nil {
		return nil, nil, err
	}

	// look at namespaces - map of run time objects - think of this later
	// Get the existing RoleBindings
	if err := r.Client.List(ctx, existingRoleBindingList, client.MatchingLabels{
		"operators.coreos.io/scopeInstance": string(in.GetUID()),
	}); err != nil {
		return nil, nil, err
	}

	//Create Map(Key, value) pair of existing Clusterrole binding.
	// it will help to seperate the clusterrolebinding which we have to create and delete unnecessaty ones
	clusterRoleBindingMap := make(map[string]rbacv1.ClusterRoleBinding)
	for _, crb := range existingClusterRoleBindingList.Items {
		clusterRoleBindingMap[crb.RoleRef.Name] = crb
	}

	//Create Map(Key, value) pair of existing role binding.
	// it will help to seperate the rolebinding which we have to create and delete unnecessaty ones
	roleBindingMap := make(map[string]rbacv1.RoleBinding)
	for _, rb := range existingRoleBindingList.Items {
		roleBindingMap[rb.GetNamespace()+"-"+rb.RoleRef.Name] = rb
	}

	return roleBindingMap, clusterRoleBindingMap, nil
}

func (r *ScopeInstanceReconciler) ensureRBAC(ctx context.Context, in *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate) error {

	objects := []client.Object{}
	// it will create clusterrole as shown below if no namespace is provided
	if len(in.Spec.Namespaces) == 0 {
		for _, cr := range st.Spec.ClusterRoles {
			objects = append(objects, &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: cr.GenerateName + "-",
					Labels: map[string]string{
						"operators.coreos.io/scopeInstance": string(in.GetUID()),
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
			})
		}
	} else {
		// it will iterate over the namespace and createrole bindings for each cluster roles
		for _, namespace := range in.Spec.Namespaces {
			for _, cr := range st.Spec.ClusterRoles {
				objects = append(objects, &rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: cr.GenerateName + "-",
						Namespace:    namespace,
						Labels: map[string]string{
							"operators.coreos.io/scopeInstance": string(in.GetUID()),
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
				})

			}
		}
	}

	if objects != nil {
		if err := r.createRBAC(ctx, objects); err != nil {
			log.Log.Info("Error in creating Role Bindings", "error", err)
			return err
		}
	}

	return nil
}

func (r *ScopeInstanceReconciler) createRBAC(ctx context.Context, objects []client.Object) error {
	// Create Objects
	for _, object := range objects {

		log.Log.Info("Creating object")
		if err := r.Client.Create(ctx, object); err != nil {
			if k8sapierrors.IsAlreadyExists(err) {
				log.Log.Info("RBAC is already Present", "Object", object.GetName())
				continue
			}

			log.Log.Error(err, "Error creating object", "object", object)
			return err
			/// TODO: try to create all the (cluster)roleBindings before returning an aggregate error.
		}
	}
	return nil
}

func (r *ScopeInstanceReconciler) updateRBAC(ctx context.Context, objects []client.Object) error {
	// Update Objects
	for _, object := range objects {

		log.Log.Info("Updating object")
		if err := r.Client.Update(ctx, object); err != nil {
			log.Log.Error(err, "Error Updating object", "object", object)
			return err
		}
	}
	return nil
}

func (r *ScopeInstanceReconciler) ensureUpdateOfRBAC(ctx context.Context, in *operatorsv1.ScopeInstance, st *operatorsv1.ScopeTemplate, clusterRoleBindingMap map[string]rbacv1.ClusterRoleBinding, roleBindingMap map[string]rbacv1.RoleBinding) error {

	listUpdateObjects := []client.Object{}
	// it will create clusterrole as shown below if no namespace is provided
	for _, cr := range st.Spec.ClusterRoles {

		if obj, found := clusterRoleBindingMap[cr.GenerateName]; found {

			updateObjects := &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: cr.GenerateName + "-",
					Labels: map[string]string{
						"operators.coreos.io/scopeInstance": string(in.GetUID()),
					},
				},
				Subjects: cr.Subjects,
				RoleRef: rbacv1.RoleRef{
					Kind:     "ClusterRole",
					Name:     cr.GenerateName,
					APIGroup: "rbac.authorization.k8s.io",
				},
			}

			if util.IsOwnedByLabel(obj.DeepCopy(), in) &&
				obj.RoleRef == updateObjects.RoleRef &&
				reflect.DeepEqual(updateObjects.Subjects, obj.Subjects) {
				r.logger.Info("Existing ClusterRoleBinding does not need to be updated")
				return nil
			}
			updateObjects.OwnerReferences = obj.OwnerReferences
			updateObjects.Subjects = obj.Subjects
			updateObjects.RoleRef = obj.RoleRef
			listUpdateObjects = append(listUpdateObjects, updateObjects)
			delete(clusterRoleBindingMap, cr.GenerateName)
		}
	}

	// it will iterate over the namespace and createrole bindings for each cluster roles
	for _, namespace := range in.Spec.Namespaces {
		for _, cr := range st.Spec.ClusterRoles {

			key := namespace + "-" + cr.GenerateName
			if obj, found := roleBindingMap[key]; found {
				updateObjects := &rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: cr.GenerateName + "-",
						Namespace:    namespace,
						Labels: map[string]string{
							"operators.coreos.io/scopeInstance": string(in.GetUID()),
						},
					},
					Subjects: cr.Subjects,
					RoleRef: rbacv1.RoleRef{
						Kind:     "ClusterRole",
						Name:     cr.GenerateName,
						APIGroup: "rbac.authorization.k8s.io",
					},
				}

				if util.IsOwnedByLabel(obj.DeepCopy(), in) &&
					obj.RoleRef == updateObjects.RoleRef &&
					reflect.DeepEqual(updateObjects.Subjects, obj.Subjects) {
					log.Log.Info("Existing RoleBinding does not need to be updated")
					return nil
				}
				updateObjects.OwnerReferences = obj.OwnerReferences
				updateObjects.Subjects = obj.Subjects
				updateObjects.RoleRef = obj.RoleRef
				listUpdateObjects = append(listUpdateObjects, updateObjects)
				delete(roleBindingMap, key)
			}
		}
	}

	if listUpdateObjects != nil {
		if err := r.updateRBAC(ctx, listUpdateObjects); err != nil {
			log.Log.Info("Error in Updating Role Bindings", "error", err)
			return err
		}
	}

	return nil
}

func (r *ScopeInstanceReconciler) cleanupInvalidRBAC(ctx context.Context, clusterRoleBindingMap map[string]rbacv1.ClusterRoleBinding, roleBindingMap map[string]rbacv1.RoleBinding) error {
	deleteObjects := []client.Object{}
	// below is for cleanup, check if Map(Key, value) of Role Binding is empty or not if not then those
	// are remaining ones which we have to delete
	for key, element := range roleBindingMap {
		split := strings.Split(key, "-")

		deleteObjects = append(deleteObjects, &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      element.GetName(),
				Namespace: split[0],
			},
		})
	}

	// below is for cleanup, check if Map(Key, value) of Cluster Role Binding is empty or not if not then those
	// are remaining ones which we have to delete
	for _, element := range clusterRoleBindingMap {
		deleteObjects = append(deleteObjects, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: element.GetName(),
			},
		})
	}

	if deleteObjects != nil {
		if err := r.deleteRBAC(ctx, deleteObjects); err != nil {
			log.Log.Info("Error in deleting Role Bindings", "error", err)
			return err
		}
	}

	return nil
}

func (r *ScopeInstanceReconciler) deleteRBAC(ctx context.Context, deleteObjects []client.Object) error {
	// Delete Unnecessary Objects
	for _, object := range deleteObjects {
		if err := r.Client.Delete(ctx, object); err != nil {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScopeInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorsv1.ScopeInstance{}).
		Watches(&source.Kind{Type: &operatorsv1.ScopeTemplate{}}, handler.EnqueueRequestsFromMapFunc(r.mapToScopInstace)).
		Complete(r)
}

func (r *ScopeInstanceReconciler) mapToScopInstace(obj client.Object) (requests []reconcile.Request) {
	if obj == nil || obj.GetName() == "" {
		return nil
	}

	// Requeue all Scope Instance in the resource namespace
	ctx := context.TODO()
	scopeInstanceList := &operatorsv1.ScopeInstanceList{}

	if err := r.Client.List(ctx, scopeInstanceList); err != nil {
		r.logger.Error(err, "error listing scope instances")
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

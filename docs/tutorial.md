# Oria Operator Tutorial

An in-depth walkthrough of building and running a oria-operator with memcached-operator.

## Prerequisites

- [Operator SDK](https://sdk.operatorframework.io/docs/installation/) v1.21.0 or newer
- User authorized with `cluster-admin` permissions so that you can install an operator, CRDs and change rbac.
- [GNU Make](https://www.gnu.org/software/make/)
- Create an [Kind Cluster](https://kind.sigs.k8s.io/)
- Install [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/)

## Overview

We will create a sample project to let you know how it works and this sample will:

- Create a `memcached-operator` using below mentioned steps. For more infomation refer to this [tutorial](https://sdk.operatorframework.io/docs/building-operators/golang/tutorial/)
- Generate the manifests using `config/default`
- Comment out `ClusterRole` and `ClusterRoleBinding`. 
- Create a `ClusterRole` template in a `ScopeTemplate` and associate it with a `ScopeInstance`
- Run the operator, the `memcached-operator` will start complaining about `RBAC`.
- Run the `oria-operator`, it will create `ClusterRole` and `ClusterRoleBinding`. This will make sure that `memcached-operator` will not complain about `RBAC`. 

### Install and run oria-operator

Clone the [github repo](https://github.com/operator-framework/oria-operator.git)

```
git clone https://github.com/operator-framework/oria-operator.git
```

Then, run the `oria-operator` with below commands on the root of the directory.

```
make install run
```

The above process runs in the foreground. You will need to open a new terminal window/tab to continue with the rest of the tutorial.

### Create a new project

Use the [Operator SDK](https://sdk.operatorframework.io/docs/installation/) CLI to create a new memcached-operator from the [tutorial](https://sdk.operatorframework.io/docs/building-operators/golang/tutorial/).

Create a directory for the memcached operator:

```
mkdir -p memcached-operator
```

Navigate into the newly created directory and initialize the project with:

```sh
operator-sdk init --domain example.com --repo github.com/example/memcached-operator
```

### Create a new API and Controller 

```sh
operator-sdk create api --group cache --version v1alpha1 --kind Memcached --resource --controller
```

Define the API for the Memcached Custom Resource(CR) by modifying the Go type definitions at `api/v1alpha1/memcached_types.go` to have the following spec and status:

```
// MemcachedSpec defines the desired state of Memcached
type MemcachedSpec struct {
	//+kubebuilder:validation:Minimum=0
	// Size is the size of the memcached deployment
	Size int32 `json:"size"`
}

// MemcachedStatus defines the observed state of Memcached
type MemcachedStatus struct {
	// Nodes are the names of the memcached pods
	Nodes []string `json:"nodes"`
}
```

### Now create docker image and push to the dockerhub

The following command will build and push an operator image:

```
make docker-build docker-push IMG=<registry>/<user>/<img-name>:<version>
```

Note:  this tutorial will use the docker image tag `example.com/memcached-operator:v0.0.1` for future steps.  You should replace references to this with the docker tag you pushed as `IMG`, above.

### Generating CRD manifests 

```sh
mkdir manifest && \ 
kustomize build config/default > manifest/manifest.yaml
```

### Update the `manifest.yaml` file

Update image tag from `controller:latest` to `example.com/memcached-operator:v0.0.1`

Then, remove `ClusterRole` with name `memcached-operator-manager-role` and `ClusterRoleBinding` with name `memcached-operator-manager-rolebinding`.

Now, create the `ScopeTemplate` and add above removed `ClusterRole` in spec in `manifests/scopeTemplate.yaml`.

```yaml
apiVersion: operators.io.operator-framework/v1alpha1
kind: ScopeTemplate
metadata:
  name: scopetemplate-sample-manager
spec:
  clusterRoles:
  - generateName: manager-role
    rules:
    - apiGroups:
      - "*"
      resources:
      - deployments
      - memcacheds
      verbs:
      - create
      - delete
      - get
      - list
      - patch
      - update
      - watch
    - apiGroups:
      - cache.example.com
      resources:
      - memcacheds/finalizers
      verbs:
      - update
    - apiGroups:
      - cache.example.com
      resources:
      - memcacheds/status
      verbs:
      - get
      - patch
      - update
    subjects:
    - kind: ServiceAccount
      name: memcached-operator-controller-manager
      namespace: memcached-operator-system
```

Then, apply manifest and look for the pod logs.

```
kubectl apply -f manifest
```

The `memcached-operator` is running successfully with some errors. 

Get the list of pods running under `memcached-operator-system` namespace.

```
kubectl get pods -n memcached-operator-system
```

The output should be similar to

```
NAME                                                     READY   STATUS    RESTARTS   AGE
memcached-operator-controller-manager-684554467c-4jflr   2/2     Running   0          9m45s
```

Check the pod logs and now you will be able to see that pod is complaining about `RBAC`.

```
kubectl logs memcached-operator-controller-manager-684554467c-4jflr manager -n memcached-operator-system
```

Create `ScopeInstance` that refers to `ScopeTemplate` and apply it.

```
cat << EOF | kubectl apply -f -
apiVersion: operators.io.operator-framework/v1alpha1
kind: ScopeInstance
metadata:
  name: scopeinstance-sample-manager
spec:
  scopeTemplateName: scopetemplate-sample-manager
EOF
```

The `oria-operator` will create the `ClusterRole` and `ClusterRoleBinding`. Verify `ClusterRole` and `ClusterRoleBinding` with below commands.

```
kubectl get clusterroles
```

In the list you should see a ClusterRole with the name `manager-role`

```
$ kubectl get clusterrolebinding
```

In the list you should see a ClusterRoleBinding with the name starts with `manager-role`

Then, the `memcached-operator` will pick these RBACs and now it will stop complaining.

```
1.6649296259379733e+09	INFO	Starting workers	{"controller": "memcached", "controllerGroup": "cache.example.com", "controllerKind": "Memcached", "worker count": 1}
```
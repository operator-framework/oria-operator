# oria-operator

## Summary

As part of the Operator Framework’s effort to move towards descoped operators the operator-lifecycle-manager (OLM) must pivot from managing an operator’s Role Based Access Controls (RBAC) to provide tooling that empowers cluster admins and operator authors to control which namespaces an operator reconciles resource events in.

The `oria-operator` will introduce two cluster scoped CRDs, the `scopeTemplate` and `scopeInstance`.

### ScopeTemplate CRD

The scopeTemplate CRD is used to define the namespaced RBAC needed by an operator. It basically allows one to define:

- A clusterRole
- a bindingTemplate, which will be used to create a clusterRoleBinding if the operator is 
scoped to all namespace OR roleBindings in the set of namespaces.

An example of a scopeTemplate CR can be seen below:

```
apiVersion: operators.io.operator-framework/v1
kind: ScopeTemplate
metadata:
  name: scopetemplate-sample
spec:
  clusterRoles:
  - generateName: test
    rules:
    - apiGroups: [""]
      # at the HTTP level, the name of the resource for accessing Secret
      # objects is "secrets"
      resources: ["secrets"]
      verbs: ["get", "watch", "list"]
    subjects:
    - kind: Group
      name: manager  # Name is case sensitive
      apiGroup: rbac.authorization.k8s.io
```

The reconciliation process will verify the below steps:
1. It will check if any ScopeInstance CRs reference to ScopeTemplate name or not.
2. If it is referencing then the clusterRole defined in the scopeTemplate will be created if it does not exist. The created clusterRole will include an ownerReference to thes copeTemplate CR.
3. If no scopeInstance references the scopeTemplate, the clusterRole defined in the scopeTemplate will be deleted if it does not exist.


### ScopeInstance CRD

The ScopeInstance CRD used to define list of `namespaces` that the RBAC in  ScopeTemplate will be created in. A cluster admin will create the scopeInstance CR and will specify:

- The name of a scopeTemplate which defines the RBAC required by the operator
- A set of namespaces that the operator should be scoped to.

```
apiVersion: operators.io.operator-framework/v1
kind: ScopeInstance
metadata:
  name: scopeinstance-sample
spec:
  scopeTemplateName: scopetemplate-sample
  namespaces:
  - default
```

The reconciliation process will verify the below steps:

1. It will look for ScopeTemplate that ScopeInstance is referencing. if it is not referencing then throw an error with the appropriate message.
2. If it is referencing and if the `namespaces` array is empty, a single `clusterRoleBinding` will be created. Otherwise, a `roleBinding` will be created in each of the `namespaces`. These resources will include an ownerReference to the ScopeInstance CR.

## Run the Operator Locally

### 1. Run locally outside the cluster 

First, install newly created ScopeInstance and ScopeTemplate CRs

```
make install
```

It will create CRDs and throw the below message

```
customresourcedefinition.apiextensions.k8s.io/scopeinstances.operators.io.operator-framework created
customresourcedefinition.apiextensions.k8s.io/scopetemplates.operators.io.operator-framework created
```

Then, run the `oria-operator` with below command and apply ScopeInstance and ScopeTemplate CRDs

```
make run
```

Apply ScopeTemplate CRD as below. This will create scopetemplate-sample.

```
$ kubectl apply -f config/samples/operators_v1_scopetemplate.yaml
scopetemplate.operators.io.operator-framework/scopetemplate-sample created
```

Now, create ScopeInstance CRD as below. This will create scopeinstance-sample.

```
$ kubectl apply -f config/samples/operators_v1_scopeinstance.yaml
scopeinstance.operators.io.operator-framework/scopeinstance-sample created
```

Once scopeinstance-sample is created, it will trigger the reconciliation process of ScopeTemplate and ScopeInstance controllers.

ScopeTemplate reconciliation process will create ClusterRoles as defined in CRD.

```
$ kubectl get clusterroles
NAME   CREATED AT
test   2022-09-20T18:39:32Z
```

ScopeInstance reconciliation process will create ClusterRoleBinding as defined in CRD.

```
$ kubectl get rolebindings --all-namespaces
NAMESPACE   NAME         ROLE               AGE
default     test-x8hdc   ClusterRole/test   33m
```

Now, let's update the scopeinstance with a new namespace. Created `test` namespace.

```
$ kubectl create namespace test
namespace/test created
```

ScopeInstance is updated as shown below.

```
apiVersion: operators.io.operator-framework/v1
kind: ScopeInstance
metadata:
  name: scopeinstance-sample
spec:
  scopeTemplateName: scopetemplate-sample
  namespaces:
  - default
  - test
```

```
$ kubectl apply -f config/samples/operators_v1_scopeinstance.yaml
scopeinstance.operators.io.operator-framework/scopeinstance-sample configured
```

Now, verify role bindings for those two namespaces.


```
$ kubectl get rolebindings --all-namespaces
NAMESPACE   NAME         ROLE               AGE
default     test-x8hdc   ClusterRole/test   37m
test        test-64hk7   ClusterRole/test   80s
```

Now, update the ScopeInstance and remove the `default` namespace from it.

```
apiVersion: operators.io.operator-framework/v1
kind: ScopeInstance
metadata:
  name: scopeinstance-sample
spec:
  scopeTemplateName: scopetemplate-sample
  namespaces:
  - test
```

```
$ kubectl apply -f config/samples/operators_v1_scopeinstance.yaml
scopeinstance.operators.io.operator-framework/scopeinstance-sample configured
```

Verify RoleBindings in all namespaces.

```
NAMESPACE   NAME         ROLE               AGE
test        test-64hk7   ClusterRole/test   2m45s
```

In the end, remove all namespaces and check if it can create ClusterRoleBinding and remove rolebindings.

```
apiVersion: operators.io.operator-framework/v1
kind: ScopeInstance
metadata:
  name: scopeinstance-sample
spec:
  scopeTemplateName: scopetemplate-sample
```

```
$ kubectl apply -f config/samples/operators_v1_scopeinstance.yaml
scopeinstance.operators.io.operator-framework/scopeinstance-sample configured
```

Verify ClusterRoleBinding and RoleBindings.

```
$ kubectl get rolebindings --all-namespaces 
No resources found
```

ClusterRoleBinding is created as shown below

```
$ kubectl get clusterrolebindings
NAME         ROLE               AGE
test-mskl2   ClusterRole/test   50s
```

## Dependency and platform support

### Go version

Release binaries will be built with the Go compiler version specified in the [developer guide][dev-guide-prereqs].
A Go Operator project's Go version can be found in its `go.mod` file.

[dev-guide-prereqs]:https://sdk.operatorframework.io/docs/contribution-guidelines/developer-guide#prerequisites

### Kubernetes versions

Supported Kubernetes versions for your Operator project or relevant binary can be determined
by following this [compatibility guide][k8s-compat].

[k8s-compat]:https://sdk.operatorframework.io/docs/overview#kubernetes-version-compatibility

### Platforms

The set of supported platforms for all binaries and images can be found in [these tables][platforms].

[platforms]:https://sdk.operatorframework.io/docs/overview#platform-support

## Community and how to get involved

- [Operator framework community][operator-framework-community]
- [Communication channels][operator-framework-communication]
- [Project meetings][operator-framework-meetings]

## How to contribute

Check out the [contributor documentation][contribution-docs].

## License

Operator SDK is under Apache 2.0 license. See the [LICENSE][license_file] file for details.

[controller-runtime]: https://github.com/kubernetes-sigs/controller-runtime
[license_file]:./LICENSE
[of-home]: https://github.com/operator-framework
[of-blog]: https://www.openshift.com/blog/introducing-the-operator-framework
[operator-link]: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
[sdk-docs]: https://sdk.operatorframework.io
[operator-framework-community]: https://github.com/operator-framework/community
[operator-framework-communication]: https://github.com/operator-framework/community#get-involved
[operator-framework-meetings]: https://github.com/operator-framework/community#meetings
[contribution-docs]: https://sdk.operatorframework.io/docs/contribution-guidelines/


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

package e2e

import (
	"testing"
	"time"

	operatorsv1 "operator-framework/oria-operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	cfg *rest.Config
	c   client.Client
)

func TestAPIs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e tests since -short flag was specified")
	}

	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(1 * time.Minute)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	cfg = ctrl.GetConfigOrDie()

	scheme := runtime.NewScheme()
	err := rbacv1.AddToScheme(scheme)
	Expect(err).To(BeNil())
	err = operatorsv1.AddToScheme(scheme)
	Expect(err).To(BeNil())
	err = corev1.AddToScheme(scheme)
	Expect(err).To(BeNil())

	c, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).To(BeNil())
})

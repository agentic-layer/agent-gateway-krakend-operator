/*
Copyright 2025.

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

// Package testutil provides common test utilities and helper functions for controller tests.
package utils

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentruntimev1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
)

// FetchService retrieves a Service from the Kubernetes API and asserts it exists.
func FetchService(ctx context.Context, k8sClient client.Client, name, namespace string) *corev1.Service {
	service := &corev1.Service{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, service)
	Expect(err).NotTo(HaveOccurred())
	return service
}

// FetchAgentGateway retrieves an AgentGateway from the Kubernetes API and asserts it exists.
func FetchAgentGateway(ctx context.Context, k8sClient client.Client, name, namespace string) *agentruntimev1alpha1.AgentGateway {
	agentGateway := &agentruntimev1alpha1.AgentGateway{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, agentGateway)
	Expect(err).NotTo(HaveOccurred())
	return agentGateway
}

// EventuallyResourceExists waits for a Kubernetes resource to exist using Eventually.
// It polls the API until the resource is found or the timeout is reached.
func EventuallyResourceExists(ctx context.Context, k8sClient client.Client, name, namespace string, obj client.Object, timeout, interval time.Duration) {
	Eventually(func() bool {
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, obj)
		return err == nil
	}, timeout, interval).Should(BeTrue())
}

// ReconcileAndExpectSuccess invokes the reconciler and asserts successful reconciliation.
// It verifies that no error occurred and that the result indicates no requeue.
func ReconcileAndExpectSuccess(ctx context.Context, reconciler interface {
	Reconcile(context.Context, ctrl.Request) (ctrl.Result, error)
}, name, namespace string) {
	result, err := reconciler.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(result).To(Equal(ctrl.Result{}))
}

// Int32Ptr returns a pointer to an int32 value.
// Useful for setting optional int32 fields in Kubernetes resources.
func Int32Ptr(i int32) *int32 {
	return &i
}

// CreateTestAgentGateway creates a test AgentGateway resource with the given name and namespace.
// If className is provided, it will be set in the spec.
func CreateTestAgentGateway(name, namespace string, className *string) *agentruntimev1alpha1.AgentGateway {
	spec := agentruntimev1alpha1.AgentGatewaySpec{
		Replicas: Int32Ptr(2),
	}

	if className != nil {
		spec.AgentGatewayClassName = *className
	}

	return &agentruntimev1alpha1.AgentGateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

// CreateTestAgentGatewayClass creates a test AgentGatewayClass with the given name and controller.
func CreateTestAgentGatewayClass(name, controller string) *agentruntimev1alpha1.AgentGatewayClass {
	return &agentruntimev1alpha1.AgentGatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: agentruntimev1alpha1.AgentGatewayClassSpec{
			Controller: controller,
		},
	}
}

// CreateTestAgentGatewayClassWithDefault creates a test AgentGatewayClass with default annotation.
// The controller is set to the provided controllerName.
func CreateTestAgentGatewayClassWithDefault(name, controllerName string) *agentruntimev1alpha1.AgentGatewayClass {
	return &agentruntimev1alpha1.AgentGatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"agentgatewayclass.kubernetes.io/is-default-class": "true",
			},
		},
		Spec: agentruntimev1alpha1.AgentGatewayClassSpec{
			Controller: controllerName,
		},
	}
}

// CreateTestAgent creates a test Agent resource with the given name, namespace, and exposed flag.
func CreateTestAgent(name, namespace string, exposed bool) *agentruntimev1alpha1.Agent {
	return &agentruntimev1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: agentruntimev1alpha1.AgentSpec{
			Exposed: exposed,
		},
	}
}

// SetAgentUrl updates the agent's status URL and asserts the update succeeds.
func SetAgentUrl(ctx context.Context, k8sClient client.Client, agent *agentruntimev1alpha1.Agent, url string) {
	agent.Status.Url = url
	Expect(k8sClient.Status().Update(ctx, agent)).To(Succeed())
}

// CleanupAllResources removes all test resources from the cluster.
// It deletes AgentGateways, AgentGatewayClasses, Agents, Services, ConfigMaps, and Deployments.
// System resources (like the kubernetes service and system configmaps) are preserved.
func CleanupAllResources(ctx context.Context, k8sClient client.Client) {
	// Clean up AgentGateways
	agentGatewayList := &agentruntimev1alpha1.AgentGatewayList{}
	_ = k8sClient.List(ctx, agentGatewayList)
	for i := range agentGatewayList.Items {
		_ = k8sClient.Delete(ctx, &agentGatewayList.Items[i])
	}

	// Clean up AgentGatewayClasses
	agentGatewayClassList := &agentruntimev1alpha1.AgentGatewayClassList{}
	_ = k8sClient.List(ctx, agentGatewayClassList)
	for i := range agentGatewayClassList.Items {
		_ = k8sClient.Delete(ctx, &agentGatewayClassList.Items[i])
	}

	// Clean up Agents
	agentList := &agentruntimev1alpha1.AgentList{}
	_ = k8sClient.List(ctx, agentList)
	for i := range agentList.Items {
		_ = k8sClient.Delete(ctx, &agentList.Items[i])
	}

	// Clean up Services
	serviceList := &corev1.ServiceList{}
	_ = k8sClient.List(ctx, serviceList)
	for i := range serviceList.Items {
		if serviceList.Items[i].Name != "kubernetes" { // Don't delete the default kubernetes service
			_ = k8sClient.Delete(ctx, &serviceList.Items[i])
		}
	}

	// Clean up ConfigMaps
	configMapList := &corev1.ConfigMapList{}
	_ = k8sClient.List(ctx, configMapList)
	for i := range configMapList.Items {
		if !IsSystemConfigMap(configMapList.Items[i].Name) {
			_ = k8sClient.Delete(ctx, &configMapList.Items[i])
		}
	}

	// Clean up Deployments
	deploymentList := &appsv1.DeploymentList{}
	_ = k8sClient.List(ctx, deploymentList)
	for i := range deploymentList.Items {
		_ = k8sClient.Delete(ctx, &deploymentList.Items[i])
	}
}

// IsSystemConfigMap returns true if the given configmap name is a system configmap that should not be deleted.
func IsSystemConfigMap(name string) bool {
	systemConfigMaps := []string{
		"kube-root-ca.crt",
		"extension-apiserver-authentication",
	}

	for _, systemName := range systemConfigMaps {
		if name == systemName {
			return true
		}
	}
	return false
}

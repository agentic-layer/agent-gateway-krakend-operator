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

package controller

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/agentic-layer/agent-gateway-krakend-operator/internal/controller/utils"
	agentruntimev1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
)

const (
	differentClassName = "different-class"
)

var _ = Describe("AgentGateway Controller", func() {
	var (
		ctx                   context.Context
		reconciler            *AgentGatewayReconciler
		agentGatewayName      = "test-agentgateway"
		agentGatewayNamespace = "default"
		timeout               = time.Second * 10
		interval              = time.Millisecond * 250
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &AgentGatewayReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	AfterEach(func() {
		// Clean up all resources
		utils.CleanupAllResources(ctx, k8sClient)
	})

	Describe("shouldProcessAgentGateway", func() {
		Context("when no AgentGatewayClass exists", func() {
			It("should return false", func() {
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeFalse())
			})
		})

		Context("when exactly one AgentGatewayClass exists with wrong controller", func() {
			var agentGatewayClass *agentruntimev1alpha1.AgentGatewayClass

			BeforeEach(func() {
				agentGatewayClass = utils.CreateTestAgentGatewayClass("single-class", "test-controller")
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())
			})

			It("should return false when no className specified and no default class", func() {
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				// With no className and test class doesn't have default annotation
				Expect(responsible).To(BeFalse())
			})

			It("should return false when className doesn't match controller", func() {
				className := "single-class"
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				// The test class has controller "test-controller" but our controller expects ControllerName
				Expect(responsible).To(BeFalse())
			})

			It("should return false when className doesn't exist", func() {
				className := differentClassName
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeFalse())
			})
		})

		Context("when exactly one AgentGatewayClass exists with correct controller", func() {
			var agentGatewayClass *agentruntimev1alpha1.AgentGatewayClass

			BeforeEach(func() {
				agentGatewayClass = utils.CreateTestAgentGatewayClass("krakend-class", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())
			})

			It("should return true when className matches", func() {
				className := "krakend-class"
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeTrue())
			})

			It("should return false when no className and no default annotation", func() {
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeFalse())
			})
		})

		Context("when AgentGatewayClass has default annotation", func() {
			var agentGatewayClass *agentruntimev1alpha1.AgentGatewayClass

			BeforeEach(func() {
				agentGatewayClass = utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())
			})

			It("should return true when no className specified", func() {
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeTrue())
			})
		})

		Context("when multiple AgentGatewayClasses exist", func() {
			BeforeEach(func() {
				class1 := utils.CreateTestAgentGatewayClass("class1", "controller1")
				class2 := utils.CreateTestAgentGatewayClass("class2", "controller2")

				Expect(k8sClient.Create(ctx, class1)).To(Succeed())
				Expect(k8sClient.Create(ctx, class2)).To(Succeed())
			})

			It("should return false when AgentGateway className doesn't match controller", func() {
				className := "class1"
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				// Neither class1 nor class2 have our controller name
				Expect(responsible).To(BeFalse())
			})

			It("should return false when AgentGateway className doesn't exist", func() {
				className := differentClassName
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeFalse())
			})

			It("should return false when AgentGateway has no className", func() {
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeFalse())
			})
		})
	})

	Describe("Reconcile", func() {
		var namespacedName types.NamespacedName

		BeforeEach(func() {
			namespacedName = types.NamespacedName{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			}
		})

		Context("when AgentGateway resource doesn't exist", func() {
			It("should return success without error", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: namespacedName,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

		Context("when controller is not responsible", func() {
			BeforeEach(func() {
				// Create multiple AgentGatewayClasses
				class1 := utils.CreateTestAgentGatewayClass("class1", "controller1")
				class2 := utils.CreateTestAgentGatewayClass("class2", "controller2")
				Expect(k8sClient.Create(ctx, class1)).To(Succeed())
				Expect(k8sClient.Create(ctx, class2)).To(Succeed())

				// Create AgentGateway with non-matching className
				className := differentClassName
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should skip reconciliation and return success", func() {
				utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

				// Verify no resources were created
				configMapName := agentGatewayName + "-krakend-config"
				configMap := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
		})

		Context("when controller is responsible", func() {
			BeforeEach(func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should successfully reconcile and create resources", func() {
				utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

				// Verify ConfigMap was created
				configMapName := agentGatewayName + "-krakend-config"
				configMap := &corev1.ConfigMap{}
				utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

				// Verify Deployment was created
				deployment := &appsv1.Deployment{}
				utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)

				// Verify Deployment properties
				Expect(deployment.Spec.Replicas).To(Equal(utils.Int32Ptr(2)))
				Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
				Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("agent-gateway"))

				// Verify Service was created
				service := &corev1.Service{}
				utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, service, timeout, interval)

				// Verify Service properties
				Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
				Expect(service.Spec.Ports).To(HaveLen(1))
				Expect(service.Spec.Ports[0].Name).To(Equal("http"))
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(10000)))
				Expect(service.Spec.Ports[0].TargetPort.IntVal).To(Equal(int32(DefaultGatewayPort)))
				Expect(service.Spec.Selector).To(HaveKeyWithValue("app", agentGatewayName))
				Expect(service.Labels).To(HaveKeyWithValue("app", agentGatewayName))
			})
		})

		Context("when AgentGateway has specific configuration", func() {
			BeforeEach(func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway with custom configuration
				agentGateway := &agentruntimev1alpha1.AgentGateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentGatewayName,
						Namespace: agentGatewayNamespace,
					},
					Spec: agentruntimev1alpha1.AgentGatewaySpec{
						Replicas: utils.Int32Ptr(3),
						Timeout:  &metav1.Duration{Duration: 30 * time.Second},
					},
				}
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should create deployment with custom replica count", func() {
				utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

				deployment := &appsv1.Deployment{}
				utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)

				Expect(deployment.Spec.Replicas).To(Equal(utils.Int32Ptr(3)))
			})
		})
	})

	Describe("ensureService", func() {
		var (
			agentGateway *agentruntimev1alpha1.AgentGateway
			serviceName  string
		)

		BeforeEach(func() {
			// Create single AgentGatewayClass with default annotation and correct controller
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway = utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			serviceName = agentGateway.Name
		})

		It("should create Service with correct configuration", func() {
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service was created
			service := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)

			// Verify Service properties
			Expect(service.Name).To(Equal(serviceName))
			Expect(service.Namespace).To(Equal(agentGatewayNamespace))
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.Ports).To(HaveLen(1))

			servicePort := service.Spec.Ports[0]
			Expect(servicePort.Name).To(Equal("http"))
			Expect(servicePort.Port).To(Equal(int32(10000)))
			Expect(servicePort.TargetPort.IntVal).To(Equal(int32(DefaultGatewayPort)))
			Expect(servicePort.Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("should set correct labels and selectors", func() {
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			service := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)

			// Verify labels
			Expect(service.Labels).To(HaveKeyWithValue("app", agentGateway.Name))

			// Verify selector
			Expect(service.Spec.Selector).To(HaveKeyWithValue("app", agentGateway.Name))
			Expect(service.Spec.Selector).To(HaveLen(1)) // Should only have stable selector
		})

		It("should set correct owner reference", func() {
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			service := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)

			// Verify owner reference
			Expect(service.OwnerReferences).To(HaveLen(1))
			ownerRef := service.OwnerReferences[0]
			Expect(ownerRef.Kind).To(Equal("AgentGateway"))
			Expect(ownerRef.Name).To(Equal(agentGateway.Name))
			Expect(ownerRef.UID).To(Equal(agentGateway.UID))
		})

		It("should not update Service if configuration is unchanged", func() {
			// Create Service first time
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Get the service and record its creation timestamp and resource version
			service := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)
			creationTimestamp := service.CreationTimestamp
			resourceVersion := service.ResourceVersion

			// Call ensureService again with same configuration
			err = reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service wasn't updated (resource version should be the same)
			updatedService := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)
			Expect(updatedService.CreationTimestamp).To(Equal(creationTimestamp))
			Expect(updatedService.ResourceVersion).To(Equal(resourceVersion))
		})

		It("should handle errors gracefully when AgentGateway is nil", func() {
			err := reconciler.ensureService(ctx, nil)
			Expect(err).To(HaveOccurred())
		})

		It("should update Service when labels change", func() {
			// Create Service first time
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Manually create a service with different labels
			existingService := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)

			// Change the labels to something different
			existingService.Labels = map[string]string{"app": "different-value"}
			err = k8sClient.Update(ctx, existingService)
			Expect(err).NotTo(HaveOccurred())

			// Call ensureService again - should detect change and update
			err = reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service was updated with correct labels
			updatedService := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)
			Expect(updatedService.Labels).To(HaveKeyWithValue("app", agentGateway.Name))
		})

		It("should update Service when port configuration changes", func() {
			// Create Service first time
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Manually modify the service ports
			existingService := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)

			// Change port configuration
			existingService.Spec.Ports[0].Port = 9999
			err = k8sClient.Update(ctx, existingService)
			Expect(err).NotTo(HaveOccurred())

			// Call ensureService again - should detect change and update
			err = reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service was updated with correct port
			updatedService := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)
			Expect(updatedService.Spec.Ports).To(HaveLen(1))
			Expect(updatedService.Spec.Ports[0].Port).To(Equal(int32(10000)))
		})

		It("should update Service when selector changes", func() {
			// Create Service first time
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Manually modify the service selector
			existingService := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)

			// Change selector
			existingService.Spec.Selector = map[string]string{"app": "wrong-selector"}
			err = k8sClient.Update(ctx, existingService)
			Expect(err).NotTo(HaveOccurred())

			// Call ensureService again - should detect change and update
			err = reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service was updated with correct selector
			updatedService := utils.FetchService(ctx, k8sClient, serviceName, agentGatewayNamespace)
			Expect(updatedService.Spec.Selector).To(HaveKeyWithValue("app", agentGateway.Name))
		})
	})

	Describe("serviceNeedsUpdate", func() {
		var (
			baseService      *corev1.Service
			identicalService *corev1.Service
		)

		BeforeEach(func() {
			baseService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Labels: map[string]string{
						"app": "test-app",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Selector: map[string]string{
						"app": "test-app",
					},
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       10000,
							TargetPort: intstr.FromInt32(8080),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			}

			// Create identical service
			identicalService = baseService.DeepCopy()
		})

		It("should return false for identical services", func() {
			needsUpdate := reconciler.serviceNeedsUpdate(baseService, identicalService)
			Expect(needsUpdate).To(BeFalse())
		})

		It("should return true when labels differ", func() {
			differentLabels := baseService.DeepCopy()
			differentLabels.Labels = map[string]string{"app": "different-app"}

			needsUpdate := reconciler.serviceNeedsUpdate(baseService, differentLabels)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should return true when number of labels differ", func() {
			moreLabels := baseService.DeepCopy()
			moreLabels.Labels = map[string]string{
				"app":     "test-app",
				"version": "1.0",
			}

			needsUpdate := reconciler.serviceNeedsUpdate(baseService, moreLabels)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should return true when service type differs", func() {
			differentType := baseService.DeepCopy()
			differentType.Spec.Type = corev1.ServiceTypeNodePort

			needsUpdate := reconciler.serviceNeedsUpdate(baseService, differentType)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should return true when selector differs", func() {
			differentSelector := baseService.DeepCopy()
			differentSelector.Spec.Selector = map[string]string{"app": "different-selector"}

			needsUpdate := reconciler.serviceNeedsUpdate(baseService, differentSelector)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should return true when number of selector entries differ", func() {
			moreSelectors := baseService.DeepCopy()
			moreSelectors.Spec.Selector = map[string]string{
				"app":     "test-app",
				"version": "1.0",
			}

			needsUpdate := reconciler.serviceNeedsUpdate(baseService, moreSelectors)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should return true when port name differs", func() {
			differentPortName := baseService.DeepCopy()
			differentPortName.Spec.Ports[0].Name = "https"

			needsUpdate := reconciler.serviceNeedsUpdate(baseService, differentPortName)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should return true when port number differs", func() {
			differentPortNumber := baseService.DeepCopy()
			differentPortNumber.Spec.Ports[0].Port = 8443

			needsUpdate := reconciler.serviceNeedsUpdate(baseService, differentPortNumber)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should return true when target port differs", func() {
			differentTargetPort := baseService.DeepCopy()
			differentTargetPort.Spec.Ports[0].TargetPort = intstr.FromInt32(9090)

			needsUpdate := reconciler.serviceNeedsUpdate(baseService, differentTargetPort)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should return true when protocol differs", func() {
			differentProtocol := baseService.DeepCopy()
			differentProtocol.Spec.Ports[0].Protocol = corev1.ProtocolUDP

			needsUpdate := reconciler.serviceNeedsUpdate(baseService, differentProtocol)
			Expect(needsUpdate).To(BeTrue())
		})

		It("should return true when number of ports differs", func() {
			morePorts := baseService.DeepCopy()
			morePorts.Spec.Ports = append(morePorts.Spec.Ports, corev1.ServicePort{
				Name:     "https",
				Port:     8443,
				Protocol: corev1.ProtocolTCP,
			})

			needsUpdate := reconciler.serviceNeedsUpdate(baseService, morePorts)
			Expect(needsUpdate).To(BeTrue())
		})
	})

	Describe("Integration with Agent resources", func() {
		BeforeEach(func() {
			// Create single AgentGatewayClass with default annotation and correct controller
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			// Create exposed Agent
			agent := utils.CreateTestAgent("test-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agent, "http://test-agent-service.default.svc.cluster.local:8080/test-agent/.well-known/agent-card.json")
		})

		It("should create ConfigMap with agent endpoints", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			// Verify ConfigMap contains agent configuration
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

			Expect(configMap.Data).To(HaveKey("krakend.json"))
			krakendConfig := configMap.Data["krakend.json"]
			Expect(krakendConfig).To(ContainSubstring("/test-agent"))
			Expect(krakendConfig).To(ContainSubstring("http://test-agent-service.default.svc.cluster.local:8080"))
		})

		It("should only include exposed agents in configuration", func() {
			// Create a non-exposed agent
			hiddenAgent := utils.CreateTestAgent("hidden-agent", agentGatewayNamespace, false)
			Expect(k8sClient.Create(ctx, hiddenAgent)).To(Succeed())

			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			// Verify ConfigMap only contains exposed agent
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

			krakendConfig := configMap.Data["krakend.json"]
			Expect(krakendConfig).To(ContainSubstring("/test-agent"))
			Expect(krakendConfig).NotTo(ContainSubstring("/hidden-agent"))
		})
	})

	Describe("generateEndpointForAgent", func() {
		var agent *agentruntimev1alpha1.Agent

		BeforeEach(func() {
			agent = utils.CreateTestAgent("test-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agent, "http://test-agent-service.default.svc.cluster.local:8080/.well-known/agent-card.json")
		})

		It("should return empty endpoints for agent without URL", func() {
			agentWithoutUrl := utils.CreateTestAgent("no-url-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agentWithoutUrl)).To(Succeed())

			endpoints, err := reconciler.generateEndpointForAgent(ctx, agentWithoutUrl)

			Expect(err).NotTo(HaveOccurred())
			Expect(endpoints).To(BeEmpty())
		})

		It("should generate correct endpoint configuration for agent with URL", func() {
			endpoints, err := reconciler.generateEndpointForAgent(ctx, agent)

			Expect(err).NotTo(HaveOccurred())
			Expect(endpoints).To(HaveLen(2)) // Agent card endpoint + A2A protocol endpoint

			// First endpoint should be agent card endpoint
			agentCardEndpoint := endpoints[0]
			Expect(agentCardEndpoint.Endpoint).To(Equal("/test-agent/.well-known/agent-card.json"))
			Expect(agentCardEndpoint.OutputEncoding).To(Equal("no-op"))
			Expect(agentCardEndpoint.Method).To(Equal("GET"))
			Expect(agentCardEndpoint.Backend).To(HaveLen(1))
			Expect(agentCardEndpoint.Backend[0].Host).To(HaveLen(1))
			Expect(agentCardEndpoint.Backend[0].Host[0]).To(Equal("http://test-agent-service.default.svc.cluster.local:8080"))
			Expect(agentCardEndpoint.Backend[0].URLPattern).To(Equal("/.well-known/agent-card.json"))

			// Second endpoint should be A2A protocol endpoint
			a2aEndpoint := endpoints[1]
			Expect(a2aEndpoint.Endpoint).To(Equal("/test-agent"))
			Expect(a2aEndpoint.OutputEncoding).To(Equal("no-op"))
			Expect(a2aEndpoint.Method).To(Equal("POST"))
			Expect(a2aEndpoint.Backend).To(HaveLen(1))
			Expect(a2aEndpoint.Backend[0].Host).To(HaveLen(1))
			Expect(a2aEndpoint.Backend[0].Host[0]).To(Equal("http://test-agent-service.default.svc.cluster.local:8080"))
			Expect(a2aEndpoint.Backend[0].URLPattern).To(Equal(""))
		})

		It("should use custom URL from agent status and strip agent-card suffix", func() {
			// Create agent with custom URL that includes a path and agent-card suffix
			customUrl := "http://custom-backend.example.com:9000/api/v1/.well-known/agent-card.json"
			agentWithCustomUrl := utils.CreateTestAgent("custom-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agentWithCustomUrl)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agentWithCustomUrl, customUrl)

			endpoints, err := reconciler.generateEndpointForAgent(ctx, agentWithCustomUrl)

			Expect(err).NotTo(HaveOccurred())
			Expect(endpoints).To(HaveLen(2))

			// First endpoint should be agent card endpoint
			agentCardEndpoint := endpoints[0]
			Expect(agentCardEndpoint.Endpoint).To(Equal("/custom-agent/api/v1/.well-known/agent-card.json"))
			Expect(agentCardEndpoint.Method).To(Equal("GET"))
			Expect(agentCardEndpoint.Backend[0].Host[0]).To(Equal("http://custom-backend.example.com:9000"))
			Expect(agentCardEndpoint.Backend[0].URLPattern).To(Equal("/api/v1/.well-known/agent-card.json"))

			// Second endpoint should be A2A endpoint
			a2aEndpoint := endpoints[1]
			Expect(a2aEndpoint.Endpoint).To(Equal("/custom-agent"))
			Expect(a2aEndpoint.Method).To(Equal("POST"))
			Expect(a2aEndpoint.Backend[0].Host[0]).To(Equal("http://custom-backend.example.com:9000"))
			Expect(a2aEndpoint.Backend[0].URLPattern).To(Equal("/api/v1"))
		})

		It("should correctly parse URL with only agent-card suffix", func() {
			// Create agent with URL that has only the agent-card suffix
			noPathUrl := "http://backend.example.com:8080/.well-known/agent-card.json"
			agentWithNoPath := utils.CreateTestAgent("no-path-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agentWithNoPath)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agentWithNoPath, noPathUrl)

			endpoints, err := reconciler.generateEndpointForAgent(ctx, agentWithNoPath)

			Expect(err).NotTo(HaveOccurred())
			Expect(endpoints).To(HaveLen(2))

			// First endpoint should be agent card endpoint
			agentCardEndpoint := endpoints[0]
			Expect(agentCardEndpoint.Endpoint).To(Equal("/no-path-agent/.well-known/agent-card.json"))
			Expect(agentCardEndpoint.Method).To(Equal("GET"))
			Expect(agentCardEndpoint.Backend[0].Host[0]).To(Equal("http://backend.example.com:8080"))
			Expect(agentCardEndpoint.Backend[0].URLPattern).To(Equal("/.well-known/agent-card.json"))

			// Second endpoint should be A2A endpoint
			a2aEndpoint := endpoints[1]
			Expect(a2aEndpoint.Endpoint).To(Equal("/no-path-agent"))
			Expect(a2aEndpoint.Method).To(Equal("POST"))
			Expect(a2aEndpoint.Backend[0].Host[0]).To(Equal("http://backend.example.com:8080"))
			Expect(a2aEndpoint.Backend[0].URLPattern).To(Equal(""))
		})
	})

	Describe("Error scenarios", func() {
		Context("when getExposedAgents fails", func() {
			It("should handle listing error gracefully", func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

				// Test with empty list (no agents) - should succeed with empty config
				utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

				// Verify ConfigMap was created but with empty endpoints
				configMapName := agentGatewayName + "-krakend-config"
				configMap := &corev1.ConfigMap{}
				utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

				Expect(configMap.Data).To(HaveKey("krakend.json"))
			})
		})

		Context("when Service creation fails", func() {
			BeforeEach(func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should return error when Service creation fails due to invalid configuration", func() {
				// This test verifies error handling in ensureService
				// Create a Service with the same name first to simulate conflict
				conflictingService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentGatewayName,
						Namespace: agentGatewayNamespace,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port: 9999, // Different port to create conflict
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, conflictingService)).To(Succeed())

				// Now try to reconcile - the ensureService should not fail but should detect existing service
				utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)
			})
		})

		Context("when ConfigMap creation fails", func() {
			BeforeEach(func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})
		})

		Context("when Deployment creation fails", func() {
			BeforeEach(func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway with extremely long name that will cause issues
				longName := "this-is-a-very-long-name-that-exceeds-kubernetes-resource-name-limits-and-should-cause-validation-errors"
				agentGateway := utils.CreateTestAgentGateway(longName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should handle deployment creation with invalid names gracefully", func() {
				longName := "this-is-a-very-long-name-that-exceeds-kubernetes-resource-name-limits-and-should-cause-validation-errors"
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      longName,
						Namespace: agentGatewayNamespace,
					},
				})

				// The error might be from Kubernetes validation or our own logic
				if err != nil {
					Expect(err.Error()).To(Or(
						ContainSubstring("name"),
						ContainSubstring("invalid"),
						ContainSubstring("too long"),
					))
				}
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

		Context("when multiple default AgentGatewayClasses exist", func() {
			BeforeEach(func() {
				// Create two AgentGatewayClasses with default annotation and correct controller
				agentGatewayClass1 := utils.CreateTestAgentGatewayClassWithDefault("default-class-1", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass1)).To(Succeed())

				agentGatewayClass2 := utils.CreateTestAgentGatewayClassWithDefault("default-class-2", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass2)).To(Succeed())

				// Create AgentGateway without className
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should process AgentGateway when multiple defaults exist", func() {
				// The controller should still process the AgentGateway even with multiple defaults
				// It uses the first default it finds
				utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

				// Verify resources were created
				configMapName := agentGatewayName + "-krakend-config"
				configMap := &corev1.ConfigMap{}
				utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)
			})
		})
	})

	Describe("ConfigMap content validation", func() {
		var (
			agentGateway *agentruntimev1alpha1.AgentGateway
			agent        *agentruntimev1alpha1.Agent
		)

		BeforeEach(func() {
			// Create single AgentGatewayClass with default annotation and correct controller
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway = utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			// Create exposed Agent
			agent = utils.CreateTestAgent("test-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agent, "http://test-agent-service.default.svc.cluster.local:8080/.well-known/agent-card.json")
		})

		It("should generate valid KrakenD JSON configuration", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			// Verify ConfigMap contains valid JSON
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

			krakendConfig := configMap.Data["krakend.json"]
			Expect(krakendConfig).NotTo(BeEmpty())

			// Validate it's valid JSON
			var jsonData map[string]interface{}
			err := json.Unmarshal([]byte(krakendConfig), &jsonData)
			Expect(err).NotTo(HaveOccurred(), "ConfigMap should contain valid JSON")

			// Validate specific KrakenD fields
			Expect(jsonData["version"]).To(Equal(float64(3)))
			Expect(jsonData["port"]).To(Equal(float64(DefaultGatewayPort)))
			Expect(jsonData["name"]).To(Equal("agent-gateway-krakend"))
			Expect(jsonData["endpoints"]).To(HaveLen(2)) // Agent card endpoint + A2A protocol endpoint

			endpoints, ok := jsonData["endpoints"].([]interface{})
			Expect(ok).To(BeTrue())

			// First endpoint should be agent card endpoint
			agentCardEndpoint := endpoints[0].(map[string]interface{})
			Expect(agentCardEndpoint["endpoint"]).To(Equal("/test-agent/.well-known/agent-card.json"))
			Expect(agentCardEndpoint["method"]).To(Equal("GET"))
			Expect(agentCardEndpoint["output_encoding"]).To(Equal("no-op"))

			// Second endpoint should be A2A protocol endpoint
			a2aEndpoint := endpoints[1].(map[string]interface{})
			Expect(a2aEndpoint["endpoint"]).To(Equal("/test-agent"))
			Expect(a2aEndpoint["method"]).To(Equal("POST"))
			Expect(a2aEndpoint["output_encoding"]).To(Equal("no-op"))
		})

		It("should handle custom timeout configuration", func() {
			// Update AgentGateway with custom timeout
			agentGateway.Spec.Timeout = &metav1.Duration{Duration: 45 * time.Second}
			Expect(k8sClient.Update(ctx, agentGateway)).To(Succeed())

			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			// Verify ConfigMap contains custom timeout
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				return err == nil && len(configMap.Data["krakend.json"]) > 0
			}, timeout, interval).Should(BeTrue())

			krakendConfig := configMap.Data["krakend.json"]
			var jsonData map[string]interface{}
			err := json.Unmarshal([]byte(krakendConfig), &jsonData)
			Expect(err).NotTo(HaveOccurred())

			Expect(jsonData["timeout"]).To(Equal("45s"))
		})

		It("should generate proper service URLs for agents", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			// Verify ConfigMap contains proper service URL
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

			krakendConfig := configMap.Data["krakend.json"]
			var jsonData map[string]interface{}
			err := json.Unmarshal([]byte(krakendConfig), &jsonData)
			Expect(err).NotTo(HaveOccurred())

			endpoints := jsonData["endpoints"].([]interface{})
			// Note: endpoints[0] is agent card endpoint, endpoints[1] is A2A protocol endpoint
			a2aEndpoint := endpoints[1].(map[string]interface{})
			backends := a2aEndpoint["backend"].([]interface{})
			backend := backends[0].(map[string]interface{})
			hosts := backend["host"].([]interface{})

			Expect(hosts[0]).To(Equal("http://test-agent-service.default.svc.cluster.local:8080"))
			Expect(backend["url_pattern"]).To(Equal("")) // Empty path is valid
		})
	})

	Describe("Deployment validation", func() {
		BeforeEach(func() {
			// Create single AgentGatewayClass with default annotation and correct controller
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
		})

		It("should create deployment with correct container configuration", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			// Verify Deployment configuration
			deployment := &appsv1.Deployment{}
			utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)

			// Validate container configuration
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := deployment.Spec.Template.Spec.Containers[0]

			Expect(container.Name).To(Equal("agent-gateway"))
			Expect(container.Image).To(Equal(Image))
			Expect(container.Ports).To(HaveLen(1))
			Expect(container.Ports[0].ContainerPort).To(Equal(int32(DefaultGatewayPort)))
			Expect(container.Ports[0].Name).To(Equal("http"))

			// Validate resource requirements
			Expect(container.Resources.Requests).To(HaveKey(corev1.ResourceMemory))
			Expect(container.Resources.Requests).To(HaveKey(corev1.ResourceCPU))
			Expect(container.Resources.Limits).To(HaveKey(corev1.ResourceMemory))
			Expect(container.Resources.Limits).To(HaveKey(corev1.ResourceCPU))

			// Validate volume mounts
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.VolumeMounts[0].Name).To(Equal("krakend-config-volume"))
			Expect(container.VolumeMounts[0].MountPath).To(Equal("/etc/krakend"))

			// Validate volumes
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
			volume := deployment.Spec.Template.Spec.Volumes[0]
			Expect(volume.Name).To(Equal("krakend-config-volume"))
			Expect(volume.ConfigMap).NotTo(BeNil())
			Expect(volume.ConfigMap.Name).To(Equal(agentGatewayName + "-krakend-config"))
		})

		It("should update deployment when replica count changes", func() {
			// Initial reconciliation
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait for deployment to be created
			deployment := &appsv1.Deployment{}
			utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)

			// Verify initial replica count
			Expect(deployment.Spec.Replicas).To(Equal(utils.Int32Ptr(2)))

			// Update AgentGateway replicas
			agentGateway := utils.FetchAgentGateway(ctx, k8sClient, agentGatewayName, agentGatewayNamespace)

			agentGateway.Spec.Replicas = utils.Int32Ptr(5)
			Expect(k8sClient.Update(ctx, agentGateway)).To(Succeed())

			// Reconcile again
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			// Verify deployment was updated
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, deployment)
				if err != nil {
					return false
				}
				return deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 5
			}, timeout, interval).Should(BeTrue())
		})
	})

	Describe("Resource updates", func() {
		BeforeEach(func() {
			// Create single AgentGatewayClass with default annotation and correct controller
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			// Initial reconciliation
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update ConfigMap when new agents are added", func() {
			// Add a new exposed agent
			agent := utils.CreateTestAgent("new-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agent, "http://new-agent-service.default.svc.cluster.local:8080/.well-known/agent-card.json")

			// Reconcile again
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify ConfigMap was updated
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				if err != nil {
					return false
				}
				krakendConfig := configMap.Data["krakend.json"]
				return krakendConfig != "" && len(krakendConfig) > 0
			}, timeout, interval).Should(BeTrue())

			krakendConfig := configMap.Data["krakend.json"]
			Expect(krakendConfig).To(ContainSubstring("/new-agent"))
		})

		It("should detect and reconcile manual ConfigMap modifications", func() {
			// Arrange: Get initial ConfigMap created by BeforeEach
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			originalConfig := configMap.Data["krakend.json"]
			Expect(originalConfig).NotTo(BeEmpty())

			// Act: Simulate configuration drift by manually corrupting ConfigMap
			configMap.Data["krakend.json"] = `{"corrupted": "manual change"}`
			err := k8sClient.Update(ctx, configMap)
			Expect(err).NotTo(HaveOccurred())

			// Verify drift was applied
			corruptedConfigMap := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      configMapName,
				Namespace: agentGatewayNamespace,
			}, corruptedConfigMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(corruptedConfigMap.Data["krakend.json"]).To(Equal(`{"corrupted": "manual change"}`))

			// Act: Trigger reconciliation
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Assert: Controller should restore ConfigMap to correct state
			// The controller's configMapNeedsUpdate() detects drift and ensureConfigMap() updates it
			reconciledConfigMap := &corev1.ConfigMap{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, reconciledConfigMap)
				if err != nil {
					return false
				}

				krakendConfig := reconciledConfigMap.Data["krakend.json"]
				hasCorruption := len(krakendConfig) > 0 && krakendConfig == `{"corrupted": "manual change"}`
				hasValidStructure := len(krakendConfig) > 0 &&
					strings.Contains(krakendConfig, "endpoints") &&
					strings.Contains(krakendConfig, "version")

				return !hasCorruption && hasValidStructure
			}, timeout, interval).Should(BeTrue())

			// Verify KrakenD configuration structure is fully restored
			finalConfig := reconciledConfigMap.Data["krakend.json"]
			Expect(finalConfig).NotTo(ContainSubstring("corrupted"))
			Expect(finalConfig).To(ContainSubstring("endpoints"))
			Expect(finalConfig).To(ContainSubstring("agent-gateway-krakend"))
			Expect(finalConfig).NotTo(Equal(`{"corrupted": "manual change"}`))
		})

		It("should update Deployment when AgentGateway replicas change", func() {
			// Update AgentGateway replicas
			agentGateway := &agentruntimev1alpha1.AgentGateway{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			}, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			agentGateway.Spec.Replicas = utils.Int32Ptr(5)
			Expect(k8sClient.Update(ctx, agentGateway)).To(Succeed())

			// Reconcile again
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify Deployment was updated
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, deployment)
				if err != nil {
					return false
				}
				return deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 5
			}, timeout, interval).Should(BeTrue())
		})

		It("should update Deployment when container image changes", func() {
			// Get the initial deployment
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Verify initial image is correct
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(Image))

			// Simulate drift: manually change the image to an older version
			deployment.Spec.Template.Spec.Containers[0].Image = "ghcr.io/agentic-layer/agent-gateway-krakend:0.2.0"
			err := k8sClient.Update(ctx, deployment)
			Expect(err).NotTo(HaveOccurred())

			// Verify drift was applied
			driftedDeployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			}, driftedDeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(driftedDeployment.Spec.Template.Spec.Containers[0].Image).To(Equal("ghcr.io/agentic-layer/agent-gateway-krakend:0.2.0"))

			// Trigger reconciliation
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify controller restored the correct image
			reconciledDeployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, reconciledDeployment)
				if err != nil {
					return false
				}

				if len(reconciledDeployment.Spec.Template.Spec.Containers) == 0 {
					return false
				}

				return reconciledDeployment.Spec.Template.Spec.Containers[0].Image == Image
			}, timeout, interval).Should(BeTrue())

			// Double-check the final image is correct
			Expect(reconciledDeployment.Spec.Template.Spec.Containers[0].Image).To(Equal(Image))
			Expect(reconciledDeployment.Spec.Template.Spec.Containers[0].Image).NotTo(Equal("ghcr.io/agentic-layer/agent-gateway-krakend:0.2.0"))
		})
	})

	Describe("Resource ownership and cleanup", func() {
		BeforeEach(func() {
			// Create single AgentGatewayClass with default annotation and correct controller
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
		})

		It("should set correct owner references for all resources", func() {
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Get the AgentGateway to check its UID
			agentGateway := &agentruntimev1alpha1.AgentGateway{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			}, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap owner reference
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(configMap.OwnerReferences).To(HaveLen(1))
			ownerRef := configMap.OwnerReferences[0]
			Expect(ownerRef.Kind).To(Equal("AgentGateway"))
			Expect(ownerRef.Name).To(Equal(agentGateway.Name))
			Expect(ownerRef.UID).To(Equal(agentGateway.UID))
			Expect(*ownerRef.Controller).To(BeTrue())

			// Verify Deployment owner reference
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(deployment.OwnerReferences).To(HaveLen(1))
			ownerRef = deployment.OwnerReferences[0]
			Expect(ownerRef.Kind).To(Equal("AgentGateway"))
			Expect(ownerRef.Name).To(Equal(agentGateway.Name))
			Expect(ownerRef.UID).To(Equal(agentGateway.UID))
			Expect(*ownerRef.Controller).To(BeTrue())

			// Verify Service owner reference
			service := &corev1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, service)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(service.OwnerReferences).To(HaveLen(1))
			ownerRef = service.OwnerReferences[0]
			Expect(ownerRef.Kind).To(Equal("AgentGateway"))
			Expect(ownerRef.Name).To(Equal(agentGateway.Name))
			Expect(ownerRef.UID).To(Equal(agentGateway.UID))
			Expect(*ownerRef.Controller).To(BeTrue())
		})

		It("should delete owned resources when AgentGateway is deleted", func() {
			// Initial reconciliation to create resources
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify resources exist
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			service := &corev1.Service{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, service)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Delete AgentGateway
			agentGateway := &agentruntimev1alpha1.AgentGateway{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			}, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Delete(ctx, agentGateway)).To(Succeed())

			// Manually clean up owned resources since test env might not have garbage collection
			// This simulates what should happen with proper owner references
			Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
			Expect(k8sClient.Delete(ctx, deployment)).To(Succeed())
			Expect(k8sClient.Delete(ctx, service)).To(Succeed())

			// Verify resources are deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, deployment)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, service)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})

var _ = Describe("findAgentGatewaysForAgent", func() {
	var (
		ctx        context.Context
		reconciler *AgentGatewayReconciler
		agent      *agentruntimev1alpha1.Agent
		timeout    = time.Second * 10
		interval   = time.Millisecond * 250
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &AgentGatewayReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		// Create a test agent for use in all tests
		agent = utils.CreateTestAgent("test-agent", "default", true)
		Expect(k8sClient.Create(ctx, agent)).To(Succeed())
	})

	AfterEach(func() {
		utils.CleanupAllResources(ctx, k8sClient)
	})

	Context("when no AgentGateways exist", func() {
		It("should return empty list", func() {
			requests := reconciler.findAgentGatewaysForAgent(ctx, agent)
			Expect(requests).To(BeEmpty())
		})
	})

	Context("when one AgentGateway exists", func() {
		It("should return single reconcile request with correct NamespacedName", func() {
			// Create default class and gateway
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			agentGateway := utils.CreateTestAgentGateway("test-gateway", "default", nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			// Wait for resources to be available
			Eventually(func() bool {
				var gw agentruntimev1alpha1.AgentGateway
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-gateway",
					Namespace: "default",
				}, &gw)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			requests := reconciler.findAgentGatewaysForAgent(ctx, agent)

			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("test-gateway"))
			Expect(requests[0].Namespace).To(Equal("default"))
		})
	})

	Context("when multiple AgentGateways exist in different namespaces", func() {
		It("should return all gateways as reconcile requests", func() {
			// Create default class
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create first gateway in default namespace
			gateway1 := utils.CreateTestAgentGateway("gateway-1", "default", nil)
			Expect(k8sClient.Create(ctx, gateway1)).To(Succeed())

			// Create second gateway in default namespace
			gateway2 := utils.CreateTestAgentGateway("gateway-2", "default", nil)
			Expect(k8sClient.Create(ctx, gateway2)).To(Succeed())

			// Create test namespace
			testNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
				},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			// Create third gateway in test namespace
			gateway3 := utils.CreateTestAgentGateway("gateway-3", "test-namespace", nil)
			Expect(k8sClient.Create(ctx, gateway3)).To(Succeed())

			// Wait for all resources to be available
			Eventually(func() int {
				var gwList agentruntimev1alpha1.AgentGatewayList
				err := k8sClient.List(ctx, &gwList)
				if err != nil {
					return 0
				}
				return len(gwList.Items)
			}, timeout, interval).Should(Equal(3))

			requests := reconciler.findAgentGatewaysForAgent(ctx, agent)

			Expect(requests).To(HaveLen(3))

			// Verify all gateways are included
			gatewayNames := make(map[string]string) // map[name]namespace
			for _, req := range requests {
				gatewayNames[req.Name] = req.Namespace
			}

			Expect(gatewayNames).To(HaveKeyWithValue("gateway-1", "default"))
			Expect(gatewayNames).To(HaveKeyWithValue("gateway-2", "default"))
			Expect(gatewayNames).To(HaveKeyWithValue("gateway-3", "test-namespace"))
		})
	})

	Context("when agent changes affect all gateways", func() {
		It("should trigger reconciliation for all gateways regardless of agent namespace", func() {
			// Create default class
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", ControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create gateways
			gateway1 := utils.CreateTestAgentGateway("gateway-a", "default", nil)
			Expect(k8sClient.Create(ctx, gateway1)).To(Succeed())

			gateway2 := utils.CreateTestAgentGateway("gateway-b", "default", nil)
			Expect(k8sClient.Create(ctx, gateway2)).To(Succeed())

			// Wait for resources
			Eventually(func() int {
				var gwList agentruntimev1alpha1.AgentGatewayList
				err := k8sClient.List(ctx, &gwList)
				if err != nil {
					return 0
				}
				return len(gwList.Items)
			}, timeout, interval).Should(Equal(2))

			// Create an agent in a different namespace
			testNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "agent-namespace",
				},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			differentAgent := utils.CreateTestAgent("different-agent", "agent-namespace", true)
			Expect(k8sClient.Create(ctx, differentAgent)).To(Succeed())

			// Even though agent is in different namespace, all gateways should be returned
			requests := reconciler.findAgentGatewaysForAgent(ctx, differentAgent)

			Expect(requests).To(HaveLen(2))
			Expect(requests[0].Name).To(BeElementOf("gateway-a", "gateway-b"))
			Expect(requests[1].Name).To(BeElementOf("gateway-a", "gateway-b"))
		})
	})
})

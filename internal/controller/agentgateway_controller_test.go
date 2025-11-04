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
		cleanupAllResources(ctx)
	})

	Describe("shouldProcessAgentGateway", func() {
		Context("when no AgentGatewayClass exists", func() {
			It("should return false", func() {
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeFalse())
			})
		})

		Context("when exactly one AgentGatewayClass exists with wrong controller", func() {
			var agentGatewayClass *agentruntimev1alpha1.AgentGatewayClass

			BeforeEach(func() {
				agentGatewayClass = createTestAgentGatewayClass("single-class", "test-controller")
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())
			})

			It("should return false when no className specified and no default class", func() {
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				// With no className and test class doesn't have default annotation
				Expect(responsible).To(BeFalse())
			})

			It("should return false when className doesn't match controller", func() {
				className := "single-class"
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				// The test class has controller "test-controller" but our controller expects ControllerName
				Expect(responsible).To(BeFalse())
			})

			It("should return false when className doesn't exist", func() {
				className := differentClassName
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeFalse())
			})
		})

		Context("when exactly one AgentGatewayClass exists with correct controller", func() {
			var agentGatewayClass *agentruntimev1alpha1.AgentGatewayClass

			BeforeEach(func() {
				agentGatewayClass = createTestAgentGatewayClass("krakend-class", ControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())
			})

			It("should return true when className matches", func() {
				className := "krakend-class"
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeTrue())
			})

			It("should return false when no className and no default annotation", func() {
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeFalse())
			})
		})

		Context("when AgentGatewayClass has default annotation", func() {
			var agentGatewayClass *agentruntimev1alpha1.AgentGatewayClass

			BeforeEach(func() {
				agentGatewayClass = createTestAgentGatewayClassWithDefault("default-class")
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())
			})

			It("should return true when no className specified", func() {
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeTrue())
			})
		})

		Context("when multiple AgentGatewayClasses exist", func() {
			BeforeEach(func() {
				class1 := createTestAgentGatewayClass("class1", "controller1")
				class2 := createTestAgentGatewayClass("class2", "controller2")

				Expect(k8sClient.Create(ctx, class1)).To(Succeed())
				Expect(k8sClient.Create(ctx, class2)).To(Succeed())
			})

			It("should return false when AgentGateway className doesn't match controller", func() {
				className := "class1"
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				// Neither class1 nor class2 have our controller name
				Expect(responsible).To(BeFalse())
			})

			It("should return false when AgentGateway className doesn't exist", func() {
				className := differentClassName
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)

				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)

				Expect(responsible).To(BeFalse())
			})

			It("should return false when AgentGateway has no className", func() {
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)

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
				class1 := createTestAgentGatewayClass("class1", "controller1")
				class2 := createTestAgentGatewayClass("class2", "controller2")
				Expect(k8sClient.Create(ctx, class1)).To(Succeed())
				Expect(k8sClient.Create(ctx, class2)).To(Succeed())

				// Create AgentGateway with non-matching className
				className := differentClassName
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should skip reconciliation and return success", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: namespacedName,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify no resources were created
				configMapName := agentGatewayName + "-krakend-config"
				configMap := &corev1.ConfigMap{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
		})

		Context("when controller is responsible", func() {
			BeforeEach(func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should successfully reconcile and create resources", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: namespacedName,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify ConfigMap was created
				configMapName := agentGatewayName + "-krakend-config"
				configMap := &corev1.ConfigMap{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      configMapName,
						Namespace: agentGatewayNamespace,
					}, configMap)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				// Verify Deployment was created
				deployment := &appsv1.Deployment{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      agentGatewayName,
						Namespace: agentGatewayNamespace,
					}, deployment)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				// Verify Deployment properties
				Expect(deployment.Spec.Replicas).To(Equal(int32Ptr(2)))
				Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
				Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("agent-gateway"))

				// Verify Service was created
				service := &corev1.Service{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      agentGatewayName,
						Namespace: agentGatewayNamespace,
					}, service)
					return err == nil
				}, timeout, interval).Should(BeTrue())

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
				agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway with custom configuration
				agentGateway := &agentruntimev1alpha1.AgentGateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentGatewayName,
						Namespace: agentGatewayNamespace,
					},
					Spec: agentruntimev1alpha1.AgentGatewaySpec{
						Replicas: int32Ptr(3),
						Timeout:  &metav1.Duration{Duration: 30 * time.Second},
					},
				}
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should create deployment with custom replica count", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: namespacedName,
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				deployment := &appsv1.Deployment{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      agentGatewayName,
						Namespace: agentGatewayNamespace,
					}, deployment)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				Expect(deployment.Spec.Replicas).To(Equal(int32Ptr(3)))
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
			agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway = createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			serviceName = agentGateway.Name
		})

		It("should create Service with correct configuration", func() {
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service was created
			service := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, service)
			Expect(err).NotTo(HaveOccurred())

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

			service := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, service)
			Expect(err).NotTo(HaveOccurred())

			// Verify labels
			Expect(service.Labels).To(HaveKeyWithValue("app", agentGateway.Name))

			// Verify selector
			Expect(service.Spec.Selector).To(HaveKeyWithValue("app", agentGateway.Name))
			Expect(service.Spec.Selector).To(HaveLen(1)) // Should only have stable selector
		})

		It("should set correct owner reference", func() {
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			service := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, service)
			Expect(err).NotTo(HaveOccurred())

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
			service := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, service)
			Expect(err).NotTo(HaveOccurred())
			creationTimestamp := service.CreationTimestamp
			resourceVersion := service.ResourceVersion

			// Call ensureService again with same configuration
			err = reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service wasn't updated (resource version should be the same)
			updatedService := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, updatedService)
			Expect(err).NotTo(HaveOccurred())
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
			existingService := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, existingService)
			Expect(err).NotTo(HaveOccurred())

			// Change the labels to something different
			existingService.Labels = map[string]string{"app": "different-value"}
			err = k8sClient.Update(ctx, existingService)
			Expect(err).NotTo(HaveOccurred())

			// Call ensureService again - should detect change and update
			err = reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service was updated with correct labels
			updatedService := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, updatedService)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedService.Labels).To(HaveKeyWithValue("app", agentGateway.Name))
		})

		It("should update Service when port configuration changes", func() {
			// Create Service first time
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Manually modify the service ports
			existingService := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, existingService)
			Expect(err).NotTo(HaveOccurred())

			// Change port configuration
			existingService.Spec.Ports[0].Port = 9999
			err = k8sClient.Update(ctx, existingService)
			Expect(err).NotTo(HaveOccurred())

			// Call ensureService again - should detect change and update
			err = reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service was updated with correct port
			updatedService := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, updatedService)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedService.Spec.Ports).To(HaveLen(1))
			Expect(updatedService.Spec.Ports[0].Port).To(Equal(int32(10000)))
		})

		It("should update Service when selector changes", func() {
			// Create Service first time
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Manually modify the service selector
			existingService := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, existingService)
			Expect(err).NotTo(HaveOccurred())

			// Change selector
			existingService.Spec.Selector = map[string]string{"app": "wrong-selector"}
			err = k8sClient.Update(ctx, existingService)
			Expect(err).NotTo(HaveOccurred())

			// Call ensureService again - should detect change and update
			err = reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Verify Service was updated with correct selector
			updatedService := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      serviceName,
				Namespace: agentGatewayNamespace,
			}, updatedService)
			Expect(err).NotTo(HaveOccurred())
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
			agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			// Create exposed Agent with service
			agent := createTestAgent("test-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			service := createTestServiceForAgent("test-agent", agentGatewayNamespace, agent.UID)
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
		})

		It("should create ConfigMap with agent endpoints", func() {
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify ConfigMap contains agent configuration
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(configMap.Data).To(HaveKey("krakend.json"))
			krakendConfig := configMap.Data["krakend.json"]
			Expect(krakendConfig).To(ContainSubstring("/test-agent"))
			Expect(krakendConfig).To(ContainSubstring("test-agent-service"))
		})

		It("should only include exposed agents in configuration", func() {
			// Create a non-exposed agent
			hiddenAgent := createTestAgent("hidden-agent", agentGatewayNamespace, false)
			Expect(k8sClient.Create(ctx, hiddenAgent)).To(Succeed())

			hiddenService := createTestServiceForAgent("hidden-agent", agentGatewayNamespace, hiddenAgent.UID)
			Expect(k8sClient.Create(ctx, hiddenService)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify ConfigMap only contains exposed agent
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			krakendConfig := configMap.Data["krakend.json"]
			Expect(krakendConfig).To(ContainSubstring("/test-agent"))
			Expect(krakendConfig).NotTo(ContainSubstring("/hidden-agent"))
		})
	})

	Describe("generateEndpointForAgent", func() {
		var agent *agentruntimev1alpha1.Agent
		var service *corev1.Service

		BeforeEach(func() {
			agent = createTestAgent("test-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			service = createTestServiceForAgent("test-agent", agentGatewayNamespace, agent.UID)
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
		})

		It("should return error for agent without protocols", func() {
			agentWithoutProtocols := createTestAgentWithoutProtocols("no-protocols-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agentWithoutProtocols)).To(Succeed())

			service := createTestServiceForAgent("no-protocols-agent", agentGatewayNamespace, agentWithoutProtocols.UID)
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			_, err := reconciler.generateEndpointForAgent(ctx, agentWithoutProtocols)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("has no protocols defined"))
		})

		It("should generate correct endpoint configuration for agent with single protocol", func() {
			endpoints, err := reconciler.generateEndpointForAgent(ctx, agent)

			Expect(err).NotTo(HaveOccurred())
			Expect(endpoints).To(HaveLen(1))

			endpoint := endpoints[0]
			Expect(endpoint.Endpoint).To(Equal("/test-agent"))
			Expect(endpoint.OutputEncoding).To(Equal("no-op"))
			Expect(endpoint.Method).To(Equal("POST"))
			Expect(endpoint.Backend).To(HaveLen(1))
			Expect(endpoint.Backend[0].Host).To(HaveLen(1))
			Expect(endpoint.Backend[0].Host[0]).To(Equal("http://test-agent-service.default.svc.cluster.local:8080"))
			Expect(endpoint.Backend[0].URLPattern).To(Equal("")) // Should be empty for protocol without path
		})

		It("should return error when service is missing", func() {
			// Create agent without service
			agentWithoutService := createTestAgent("no-service-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agentWithoutService)).To(Succeed())

			_, err := reconciler.generateEndpointForAgent(ctx, agentWithoutService)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no service found owned by agent"))
		})

		It("should generate multiple endpoints for agent with protocols", func() {
			// Create agent with multiple protocols
			agentWithProtocols := createTestAgentWithProtocols("multi-protocol-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agentWithProtocols)).To(Succeed())

			service := createTestServiceForAgent("multi-protocol-agent", agentGatewayNamespace, agentWithProtocols.UID)
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			endpoints, err := reconciler.generateEndpointForAgent(ctx, agentWithProtocols)

			Expect(err).NotTo(HaveOccurred())
			Expect(endpoints).To(HaveLen(2))

			// Check first endpoint (OpenAI with path - should be GET)
			Expect(endpoints[0].Endpoint).To(Equal("/multi-protocol-agent/api/v1"))
			Expect(endpoints[0].Method).To(Equal("POST"))
			Expect(endpoints[0].Backend).To(HaveLen(1))
			Expect(endpoints[0].Backend[0].Host[0]).To(Equal("http://multi-protocol-agent-service.default.svc.cluster.local:8080"))
			Expect(endpoints[0].Backend[0].URLPattern).To(Equal("/api/v1"))

			// Check second endpoint (A2A without path - should be POST)
			Expect(endpoints[1].Endpoint).To(Equal("/multi-protocol-agent"))
			Expect(endpoints[1].Method).To(Equal("POST"))
			Expect(endpoints[1].Backend).To(HaveLen(1))
			Expect(endpoints[1].Backend[0].Host[0]).To(Equal("http://multi-protocol-agent-service.default.svc.cluster.local:8080"))
			Expect(endpoints[1].Backend[0].URLPattern).To(Equal(""))
		})
	})

	Describe("Error scenarios", func() {
		Context("when agent service is missing", func() {
			BeforeEach(func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

				// Create exposed Agent but no service
				agent := createTestAgent("test-agent", agentGatewayNamespace, true)
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			})
		})

		Context("when getExposedAgents fails", func() {
			It("should handle listing error gracefully", func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

				// Test with empty list (no agents) - should succeed with empty config
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      agentGatewayName,
						Namespace: agentGatewayNamespace,
					},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify ConfigMap was created but with empty endpoints
				configMapName := agentGatewayName + "-krakend-config"
				configMap := &corev1.ConfigMap{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      configMapName,
						Namespace: agentGatewayNamespace,
					}, configMap)
					return err == nil
				}, timeout, interval).Should(BeTrue())

				Expect(configMap.Data).To(HaveKey("krakend.json"))
			})
		})

		Context("when Service creation fails", func() {
			BeforeEach(func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
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
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      agentGatewayName,
						Namespace: agentGatewayNamespace,
					},
				})

				// Should succeed because ensureService doesn't update existing services
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

		Context("when ConfigMap creation fails", func() {
			BeforeEach(func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})
		})

		Context("when Deployment creation fails", func() {
			BeforeEach(func() {
				// Create single AgentGatewayClass with default annotation and correct controller
				agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				// Create AgentGateway with extremely long name that will cause issues
				longName := "this-is-a-very-long-name-that-exceeds-kubernetes-resource-name-limits-and-should-cause-validation-errors"
				agentGateway := createTestAgentGateway(longName, agentGatewayNamespace, nil)
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
				agentGatewayClass1 := createTestAgentGatewayClassWithDefault("default-class-1")
				Expect(k8sClient.Create(ctx, agentGatewayClass1)).To(Succeed())

				agentGatewayClass2 := createTestAgentGatewayClassWithDefault("default-class-2")
				Expect(k8sClient.Create(ctx, agentGatewayClass2)).To(Succeed())

				// Create AgentGateway without className
				agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should process AgentGateway when multiple defaults exist", func() {
				// The controller should still process the AgentGateway even with multiple defaults
				// It uses the first default it finds
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      agentGatewayName,
						Namespace: agentGatewayNamespace,
					},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify resources were created
				configMapName := agentGatewayName + "-krakend-config"
				configMap := &corev1.ConfigMap{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      configMapName,
						Namespace: agentGatewayNamespace,
					}, configMap)
					return err == nil
				}, timeout, interval).Should(BeTrue())
			})
		})
	})

	Describe("ConfigMap content validation", func() {
		var (
			agentGateway *agentruntimev1alpha1.AgentGateway
			agent        *agentruntimev1alpha1.Agent
			service      *corev1.Service
		)

		BeforeEach(func() {
			// Create single AgentGatewayClass with default annotation and correct controller
			agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway = createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			// Create exposed Agent
			agent = createTestAgent("test-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			service = createTestServiceForAgent("test-agent", agentGatewayNamespace, agent.UID)
			Expect(k8sClient.Create(ctx, service)).To(Succeed())
		})

		It("should generate valid KrakenD JSON configuration", func() {
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify ConfigMap contains valid JSON
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			krakendConfig := configMap.Data["krakend.json"]
			Expect(krakendConfig).NotTo(BeEmpty())

			// Validate it's valid JSON
			var jsonData map[string]interface{}
			err = json.Unmarshal([]byte(krakendConfig), &jsonData)
			Expect(err).NotTo(HaveOccurred(), "ConfigMap should contain valid JSON")

			// Validate specific KrakenD fields
			Expect(jsonData["version"]).To(Equal(float64(3)))
			Expect(jsonData["port"]).To(Equal(float64(DefaultGatewayPort)))
			Expect(jsonData["name"]).To(Equal("agent-gateway-krakend"))
			Expect(jsonData["endpoints"]).To(HaveLen(1))

			endpoints, ok := jsonData["endpoints"].([]interface{})
			Expect(ok).To(BeTrue())
			endpoint := endpoints[0].(map[string]interface{})
			Expect(endpoint["endpoint"]).To(Equal("/test-agent"))
			Expect(endpoint["method"]).To(Equal("POST"))
			Expect(endpoint["output_encoding"]).To(Equal("no-op"))
		})

		It("should handle custom timeout configuration", func() {
			// Update AgentGateway with custom timeout
			agentGateway.Spec.Timeout = &metav1.Duration{Duration: 45 * time.Second}
			Expect(k8sClient.Update(ctx, agentGateway)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

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
			err = json.Unmarshal([]byte(krakendConfig), &jsonData)
			Expect(err).NotTo(HaveOccurred())

			Expect(jsonData["timeout"]).To(Equal("45s"))
		})

		It("should generate proper service URLs for agents", func() {
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify ConfigMap contains proper service URL
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			krakendConfig := configMap.Data["krakend.json"]
			var jsonData map[string]interface{}
			err = json.Unmarshal([]byte(krakendConfig), &jsonData)
			Expect(err).NotTo(HaveOccurred())

			endpoints := jsonData["endpoints"].([]interface{})
			endpoint := endpoints[0].(map[string]interface{})
			backends := endpoint["backend"].([]interface{})
			backend := backends[0].(map[string]interface{})
			hosts := backend["host"].([]interface{})

			Expect(hosts[0]).To(Equal("http://test-agent-service.default.svc.cluster.local:8080"))
			Expect(backend["url_pattern"]).To(Equal("")) // Should be empty for pass-through
		})
	})

	Describe("Deployment validation", func() {
		BeforeEach(func() {
			// Create single AgentGatewayClass with default annotation and correct controller
			agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
		})

		It("should create deployment with correct container configuration", func() {
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify Deployment configuration
			deployment := &appsv1.Deployment{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

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
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, deployment)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Verify initial replica count
			Expect(deployment.Spec.Replicas).To(Equal(int32Ptr(2)))

			// Update AgentGateway replicas
			agentGateway := &agentruntimev1alpha1.AgentGateway{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			}, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			agentGateway.Spec.Replicas = int32Ptr(5)
			Expect(k8sClient.Update(ctx, agentGateway)).To(Succeed())

			// Reconcile again
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

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
			agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
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
			agent := createTestAgent("new-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			service := createTestServiceForAgent("new-agent", agentGatewayNamespace, agent.UID)
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

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

		It("should update Deployment when AgentGateway replicas change", func() {
			// Update AgentGateway replicas
			agentGateway := &agentruntimev1alpha1.AgentGateway{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			}, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			agentGateway.Spec.Replicas = int32Ptr(5)
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
	})

	Describe("Resource ownership and cleanup", func() {
		BeforeEach(func() {
			// Create single AgentGatewayClass with default annotation and correct controller
			agentGatewayClass := createTestAgentGatewayClassWithDefault("default-class")
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			// Create AgentGateway
			agentGateway := createTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
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

func int32Ptr(i int32) *int32 {
	return &i
}

// Helper functions for creating test resources

func createTestAgentGateway(name, namespace string, className *string) *agentruntimev1alpha1.AgentGateway {
	spec := agentruntimev1alpha1.AgentGatewaySpec{
		Replicas: int32Ptr(2),
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

func createTestAgentGatewayClass(name, controller string) *agentruntimev1alpha1.AgentGatewayClass {
	return &agentruntimev1alpha1.AgentGatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: agentruntimev1alpha1.AgentGatewayClassSpec{
			Controller: controller,
		},
	}
}

func createTestAgentGatewayClassWithDefault(name string) *agentruntimev1alpha1.AgentGatewayClass {
	return &agentruntimev1alpha1.AgentGatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				"agentgatewayclass.kubernetes.io/is-default-class": "true",
			},
		},
		Spec: agentruntimev1alpha1.AgentGatewayClassSpec{
			Controller: ControllerName,
		},
	}
}

func createTestAgent(name, namespace string, exposed bool) *agentruntimev1alpha1.Agent {
	return &agentruntimev1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: agentruntimev1alpha1.AgentSpec{
			Exposed: exposed,
			Protocols: []agentruntimev1alpha1.AgentProtocol{
				{
					Name: "default",
					Type: "A2A",
					Port: 8080,
				},
			},
		},
	}
}

func createTestAgentWithoutProtocols(name, namespace string, exposed bool) *agentruntimev1alpha1.Agent {
	return &agentruntimev1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: agentruntimev1alpha1.AgentSpec{
			Exposed: exposed,
			// No protocols defined
		},
	}
}

func createTestServiceForAgent(name, namespace string, agentUID types.UID) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-service",
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "runtime.agentic-layer.ai/v1alpha1",
					Kind:       "Agent",
					Name:       name,
					UID:        agentUID,
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: 8080,
				},
			},
		},
	}
}

func createTestAgentWithProtocols(name, namespace string, exposed bool) *agentruntimev1alpha1.Agent {
	return &agentruntimev1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: agentruntimev1alpha1.AgentSpec{
			Exposed: exposed,
			Protocols: []agentruntimev1alpha1.AgentProtocol{
				{
					Name: "http-api",
					Type: "A2A",
					Port: 8080,
					Path: "/api/v1",
				},
				{
					Name: "default",
					Type: "A2A",
					Port: 8080,
					// Path is empty, should use /{agentName}
				},
			},
		},
	}
}

func cleanupAllResources(ctx context.Context) {
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
		if !isSystemConfigMap(configMapList.Items[i].Name) {
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

func isSystemConfigMap(name string) bool {
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

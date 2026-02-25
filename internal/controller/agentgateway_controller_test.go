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
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kevents "k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: kevents.NewFakeRecorder(100),
		}
	})

	AfterEach(func() {
		utils.CleanupAllResources(ctx, k8sClient)
	})

	Describe("shouldProcessAgentGateway", func() {
		type testCase struct {
			name                string
			setupClasses        func() []*agentruntimev1alpha1.AgentGatewayClass
			gatewayClassName    *string
			expectedResponsible bool
		}

		DescribeTable("responsibility determination",
			func(tc testCase) {
				// Setup classes if provided
				if tc.setupClasses != nil {
					classes := tc.setupClasses()
					for _, class := range classes {
						Expect(k8sClient.Create(ctx, class)).To(Succeed())
					}
				}

				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, tc.gatewayClassName)
				responsible := reconciler.shouldProcessAgentGateway(ctx, agentGateway)
				Expect(responsible).To(Equal(tc.expectedResponsible))
			},
			Entry("no AgentGatewayClass exists", testCase{
				name:                "no class",
				setupClasses:        nil,
				gatewayClassName:    nil,
				expectedResponsible: false,
			}),
			Entry("single class with wrong controller, no className", testCase{
				name: "wrong controller",
				setupClasses: func() []*agentruntimev1alpha1.AgentGatewayClass {
					return []*agentruntimev1alpha1.AgentGatewayClass{
						utils.CreateTestAgentGatewayClass("single-class", "test-controller"),
					}
				},
				gatewayClassName:    nil,
				expectedResponsible: false,
			}),
			Entry("single class with correct controller and matching className", testCase{
				name: "correct controller with className",
				setupClasses: func() []*agentruntimev1alpha1.AgentGatewayClass {
					return []*agentruntimev1alpha1.AgentGatewayClass{
						utils.CreateTestAgentGatewayClass("krakend-class", AgentGatewayKrakendControllerName),
					}
				},
				gatewayClassName:    stringPtr("krakend-class"),
				expectedResponsible: true,
			}),
			Entry("default class with correct controller, no className", testCase{
				name: "default class",
				setupClasses: func() []*agentruntimev1alpha1.AgentGatewayClass {
					return []*agentruntimev1alpha1.AgentGatewayClass{
						utils.CreateTestAgentGatewayClassWithDefault("default-class", AgentGatewayKrakendControllerName),
					}
				},
				gatewayClassName:    nil,
				expectedResponsible: true,
			}),
			Entry("multiple classes without matching controller", testCase{
				name: "multiple wrong controllers",
				setupClasses: func() []*agentruntimev1alpha1.AgentGatewayClass {
					return []*agentruntimev1alpha1.AgentGatewayClass{
						utils.CreateTestAgentGatewayClass("class1", "controller1"),
						utils.CreateTestAgentGatewayClass("class2", "controller2"),
					}
				},
				gatewayClassName:    stringPtr("class1"),
				expectedResponsible: false,
			}),
		)
	})

	Describe("Reconcile", func() {
		Context("when AgentGateway resource doesn't exist", func() {
			It("should return success without error", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      "nonexistent",
						Namespace: agentGatewayNamespace,
					},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

		Context("when controller is not responsible", func() {
			BeforeEach(func() {
				class1 := utils.CreateTestAgentGatewayClass("class1", "controller1")
				Expect(k8sClient.Create(ctx, class1)).To(Succeed())

				className := differentClassName
				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, &className)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should skip reconciliation and not create resources", func() {
				utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

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
				agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", AgentGatewayKrakendControllerName)
				Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

				agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
				Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
			})

			It("should successfully reconcile and create all resources", func() {
				utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

				// Verify ConfigMap
				configMapName := agentGatewayName + "-krakend-config"
				configMap := &corev1.ConfigMap{}
				utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

				// Verify Deployment with correct configuration
				deployment := &appsv1.Deployment{}
				utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)
				Expect(deployment.Spec.Replicas).To(Equal(utils.Int32Ptr(2)))
				Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
				Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("agent-gateway"))

				// Verify Service
				service := &corev1.Service{}
				utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, service, timeout, interval)
				Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
				Expect(service.Spec.Ports).To(HaveLen(1))
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
			})

			It("should respect custom replica count", func() {
				// Update with custom replicas
				agentGateway := utils.FetchAgentGateway(ctx, k8sClient, agentGatewayName, agentGatewayNamespace)
				agentGateway.Spec.Replicas = utils.Int32Ptr(3)
				Expect(k8sClient.Update(ctx, agentGateway)).To(Succeed())

				utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

				deployment := &appsv1.Deployment{}
				utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)
				Expect(deployment.Spec.Replicas).To(Equal(utils.Int32Ptr(3)))
			})
		})
	})

	Describe("Service management", func() {
		BeforeEach(func() {
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", AgentGatewayKrakendControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())
		})

		It("should create Service with correct configuration and owner reference", func() {
			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			service := utils.FetchService(ctx, k8sClient, agentGatewayName, agentGatewayNamespace)

			// Verify Service properties
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Name).To(Equal("http"))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
			Expect(service.Labels).To(HaveKeyWithValue("app", agentGatewayName))
			Expect(service.Spec.Selector).To(HaveKeyWithValue("app", agentGatewayName))

			// Verify owner reference
			verifyOwnerReference(service, agentGateway)
		})

		It("should update Service when configuration drifts", func() {
			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			// Create service first time
			err := reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			// Modify service to simulate drift
			existingService := utils.FetchService(ctx, k8sClient, agentGatewayName, agentGatewayNamespace)
			existingService.Spec.Ports[0].Port = 9999
			err = k8sClient.Update(ctx, existingService)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile should restore correct configuration
			err = reconciler.ensureService(ctx, agentGateway)
			Expect(err).NotTo(HaveOccurred())

			updatedService := utils.FetchService(ctx, k8sClient, agentGatewayName, agentGatewayNamespace)
			Expect(updatedService.Spec.Ports[0].Port).To(Equal(int32(80)))
		})

		It("should handle nil AgentGateway gracefully", func() {
			err := reconciler.ensureService(ctx, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Agent integration", func() {
		BeforeEach(func() {
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", AgentGatewayKrakendControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
		})

		It("should include exposed agents in configuration", func() {
			agent := utils.CreateTestAgent("test-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agent, "http://test-agent-service.default.svc.cluster.local:8080/test-agent/.well-known/agent-card.json")

			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

			krakendConfig := configMap.Data["krakend.json"]
			Expect(krakendConfig).To(ContainSubstring("/test-agent"))
			Expect(krakendConfig).To(ContainSubstring("http://test-agent-service.default.svc.cluster.local:8080"))
		})

		It("should exclude non-exposed agents from configuration", func() {
			exposedAgent := utils.CreateTestAgent("exposed-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, exposedAgent)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, exposedAgent, "http://exposed:8080/.well-known/agent-card.json")

			hiddenAgent := utils.CreateTestAgent("hidden-agent", agentGatewayNamespace, false)
			Expect(k8sClient.Create(ctx, hiddenAgent)).To(Succeed())

			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

			krakendConfig := configMap.Data["krakend.json"]
			Expect(krakendConfig).To(ContainSubstring("/exposed-agent"))
			Expect(krakendConfig).NotTo(ContainSubstring("/hidden-agent"))
		})
	})

	Describe("Endpoint generation", func() {
		It("should generate correct endpoints for basic agent card URL", func() {
			parsedURL, err := url.Parse("http://test-agent-service.default.svc.cluster.local:8080/.well-known/agent-card.json")
			Expect(err).NotTo(HaveOccurred())

			endpoints := reconciler.generateAgentEndpoints("default/test-agent", parsedURL)

			Expect(endpoints).To(HaveLen(2))

			// Agent card endpoint
			Expect(endpoints[0].Endpoint).To(Equal("/default/test-agent/.well-known/agent-card.json"))
			Expect(endpoints[0].Method).To(Equal("GET"))
			Expect(endpoints[0].Backend[0].Host[0]).To(Equal("http://test-agent-service.default.svc.cluster.local:8080"))

			// A2A endpoint
			Expect(endpoints[1].Endpoint).To(Equal("/default/test-agent"))
			Expect(endpoints[1].Method).To(Equal("POST"))
			Expect(endpoints[1].Backend[0].URLPattern).To(Equal(""))
		})

		It("should handle custom URL paths correctly", func() {
			parsedURL, err := url.Parse("http://custom-backend.example.com:9000/api/v1/.well-known/agent-card.json")
			Expect(err).NotTo(HaveOccurred())

			endpoints := reconciler.generateAgentEndpoints("default/custom-agent", parsedURL)

			Expect(endpoints).To(HaveLen(2))
			Expect(endpoints[0].Endpoint).To(Equal("/default/custom-agent/api/v1/.well-known/agent-card.json"))
			Expect(endpoints[1].Backend[0].URLPattern).To(Equal("/api/v1"))
		})
	})

	Describe("Shorthand endpoint generation", func() {
		var configMapName string

		BeforeEach(func() {
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", AgentGatewayKrakendControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			configMapName = agentGatewayName + "-krakend-config"
		})

		It("should create shorthand endpoints for unique agent names", func() {
			agent := utils.CreateTestAgent("unique-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agent, "http://unique-agent:8080/.well-known/agent-card.json")

			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

			krakendConfig := configMap.Data["krakend.json"]
			// Should have both namespaced and shorthand
			Expect(krakendConfig).To(ContainSubstring(`"endpoint": "/default/unique-agent"`))
			Expect(krakendConfig).To(ContainSubstring(`"endpoint": "/unique-agent"`))
		})

		It("should NOT create shorthand endpoints for conflicting names", func() {
			namespaceA := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace-a"}}
			Expect(k8sClient.Create(ctx, namespaceA)).To(Succeed())
			namespaceB := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "namespace-b"}}
			Expect(k8sClient.Create(ctx, namespaceB)).To(Succeed())

			agent1 := utils.CreateTestAgent("common-agent", "namespace-a", true)
			Expect(k8sClient.Create(ctx, agent1)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agent1, "http://agent1:8080/.well-known/agent-card.json")

			agent2 := utils.CreateTestAgent("common-agent", "namespace-b", true)
			Expect(k8sClient.Create(ctx, agent2)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agent2, "http://agent2:8080/.well-known/agent-card.json")

			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

			krakendConfig := configMap.Data["krakend.json"]
			// Should have namespaced endpoints
			Expect(krakendConfig).To(ContainSubstring(`"endpoint": "/namespace-a/common-agent"`))
			Expect(krakendConfig).To(ContainSubstring(`"endpoint": "/namespace-b/common-agent"`))

			// Should NOT have shorthand endpoint
			shorthandCount := strings.Count(krakendConfig, `"endpoint": "/common-agent"`)
			Expect(shorthandCount).To(Equal(0))
		})
	})

	Describe("ConfigMap content validation", func() {
		BeforeEach(func() {
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", AgentGatewayKrakendControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

			agent := utils.CreateTestAgent("test-agent", agentGatewayNamespace, true)
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			utils.SetAgentUrl(ctx, k8sClient, agent, "http://test-agent-service.default.svc.cluster.local:8080/.well-known/agent-card.json")
		})

		It("should generate valid KrakenD configuration", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

			krakendConfig := configMap.Data["krakend.json"]
			Expect(krakendConfig).NotTo(BeEmpty())
			Expect(krakendConfig).To(ContainSubstring(`"version": 3`))
			Expect(krakendConfig).To(ContainSubstring(`"port": 8080`))
			Expect(krakendConfig).To(ContainSubstring(`"name": "agent-gateway-krakend"`))
			Expect(krakendConfig).To(ContainSubstring(`"telemetry/opentelemetry"`))
		})

		It("should handle custom timeout configuration", func() {
			agentGateway := utils.FetchAgentGateway(ctx, k8sClient, agentGatewayName, agentGatewayNamespace)
			agentGateway.Spec.Timeout = &metav1.Duration{Duration: 45 * time.Second}
			Expect(k8sClient.Update(ctx, agentGateway)).To(Succeed())

			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

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
			Expect(krakendConfig).To(ContainSubstring(`"timeout": "45s"`))
		})

		It("should restore ConfigMap after manual drift", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Simulate drift
			configMap.Data["krakend.json"] = `{"corrupted": "manual change"}`
			err := k8sClient.Update(ctx, configMap)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile should restore
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

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
				return len(krakendConfig) > 0 &&
					strings.Contains(krakendConfig, "endpoints") &&
					!strings.Contains(krakendConfig, "corrupted")
			}, timeout, interval).Should(BeTrue())
		})
	})

	Describe("Deployment validation", func() {
		BeforeEach(func() {
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", AgentGatewayKrakendControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
		})

		It("should create deployment with correct container configuration", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			deployment := &appsv1.Deployment{}
			utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Name).To(Equal("agent-gateway"))
			Expect(container.Image).To(Equal(Image))
			Expect(container.Ports[0].ContainerPort).To(Equal(int32(DefaultGatewayPort)))
			Expect(container.Resources.Requests).To(HaveKey(corev1.ResourceMemory))
			Expect(container.Resources.Limits).To(HaveKey(corev1.ResourceCPU))
			Expect(container.VolumeMounts[0].MountPath).To(Equal("/etc/krakend"))

			// Verify volumes
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name).To(Equal(agentGatewayName + "-krakend-config"))
		})

		It("should create deployment with health probes configured", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			deployment := &appsv1.Deployment{}
			utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)

			container := deployment.Spec.Template.Spec.Containers[0]

			// Verify liveness probe
			Expect(container.LivenessProbe).NotTo(BeNil())
			Expect(container.LivenessProbe.HTTPGet).NotTo(BeNil())
			Expect(container.LivenessProbe.HTTPGet.Path).To(Equal("/__health"))
			Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(DefaultGatewayPort)))
			Expect(container.LivenessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTP))
			Expect(container.LivenessProbe.InitialDelaySeconds).To(Equal(int32(10)))
			Expect(container.LivenessProbe.PeriodSeconds).To(Equal(int32(10)))

			// Verify readiness probe
			Expect(container.ReadinessProbe).NotTo(BeNil())
			Expect(container.ReadinessProbe.HTTPGet).NotTo(BeNil())
			Expect(container.ReadinessProbe.HTTPGet.Path).To(Equal("/__health"))
			Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(DefaultGatewayPort)))
			Expect(container.ReadinessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTP))
			Expect(container.ReadinessProbe.InitialDelaySeconds).To(Equal(int32(5)))
			Expect(container.ReadinessProbe.PeriodSeconds).To(Equal(int32(5)))
		})

		It("should update deployment when replica count changes", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			deployment := &appsv1.Deployment{}
			utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)
			Expect(deployment.Spec.Replicas).To(Equal(utils.Int32Ptr(2)))

			// Update replicas
			agentGateway := utils.FetchAgentGateway(ctx, k8sClient, agentGatewayName, agentGatewayNamespace)
			agentGateway.Spec.Replicas = utils.Int32Ptr(5)
			Expect(k8sClient.Update(ctx, agentGateway)).To(Succeed())

			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      agentGatewayName,
					Namespace: agentGatewayNamespace,
				}, deployment)
				return err == nil && deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 5
			}, timeout, interval).Should(BeTrue())
		})

		It("should restore correct image after drift", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			deployment := &appsv1.Deployment{}
			utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)

			// Simulate drift
			deployment.Spec.Template.Spec.Containers[0].Image = "fake-registry.example.com/fake-gateway:outdated"
			err := k8sClient.Update(ctx, deployment)
			Expect(err).NotTo(HaveOccurred())

			// Reconcile should restore
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			reconciledDeployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			}, reconciledDeployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciledDeployment.Spec.Template.Spec.Containers[0].Image).To(Equal(Image))
		})
	})

	Describe("Resource ownership and cleanup", func() {
		BeforeEach(func() {
			agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", AgentGatewayKrakendControllerName)
			Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

			agentGateway := utils.CreateTestAgentGateway(agentGatewayName, agentGatewayNamespace, nil)
			Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())
		})

		It("should set correct owner references for all resources", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			agentGateway := utils.FetchAgentGateway(ctx, k8sClient, agentGatewayName, agentGatewayNamespace)

			// Verify ConfigMap owner reference
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)
			verifyOwnerReference(configMap, agentGateway)

			// Verify Deployment owner reference
			deployment := &appsv1.Deployment{}
			utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)
			verifyOwnerReference(deployment, agentGateway)

			// Verify Service owner reference
			service := &corev1.Service{}
			utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, service, timeout, interval)
			verifyOwnerReference(service, agentGateway)
		})

		It("should delete owned resources when AgentGateway is deleted", func() {
			utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

			// Verify resources exist
			configMapName := agentGatewayName + "-krakend-config"
			configMap := &corev1.ConfigMap{}
			utils.EventuallyResourceExists(ctx, k8sClient, configMapName, agentGatewayNamespace, configMap, timeout, interval)

			deployment := &appsv1.Deployment{}
			utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)

			service := &corev1.Service{}
			utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, service, timeout, interval)

			// Delete AgentGateway
			agentGateway := utils.FetchAgentGateway(ctx, k8sClient, agentGatewayName, agentGatewayNamespace)
			Expect(k8sClient.Delete(ctx, agentGateway)).To(Succeed())

			// Manually clean up owned resources (test env doesn't have GC)
			Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
			Expect(k8sClient.Delete(ctx, deployment)).To(Succeed())
			Expect(k8sClient.Delete(ctx, service)).To(Succeed())

			// Verify deletion
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentGatewayNamespace,
				}, configMap)
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

		agent = utils.CreateTestAgent("test-agent", "default", true)
		Expect(k8sClient.Create(ctx, agent)).To(Succeed())
	})

	AfterEach(func() {
		utils.CleanupAllResources(ctx, k8sClient)
	})

	It("should return empty list when no AgentGateways exist", func() {
		requests := reconciler.findAgentGatewaysForAgent(ctx, agent)
		Expect(requests).To(BeEmpty())
	})

	It("should return all gateways when multiple exist", func() {
		agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", AgentGatewayKrakendControllerName)
		Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

		gateway1 := utils.CreateTestAgentGateway("gateway-1", "default", nil)
		Expect(k8sClient.Create(ctx, gateway1)).To(Succeed())

		gateway2 := utils.CreateTestAgentGateway("gateway-2", "default", nil)
		Expect(k8sClient.Create(ctx, gateway2)).To(Succeed())

		Eventually(func() int {
			var gwList agentruntimev1alpha1.AgentGatewayList
			err := k8sClient.List(ctx, &gwList)
			if err != nil {
				return 0
			}
			return len(gwList.Items)
		}, timeout, interval).Should(Equal(2))

		requests := reconciler.findAgentGatewaysForAgent(ctx, agent)

		Expect(requests).To(HaveLen(2))
		gatewayNames := []string{requests[0].Name, requests[1].Name}
		Expect(gatewayNames).To(ConsistOf("gateway-1", "gateway-2"))
	})
})

var _ = Describe("ConfigMap and Secret Hash Annotations", func() {
	var (
		ctx                   context.Context
		reconciler            *AgentGatewayReconciler
		agentGatewayName      = "test-gateway-with-refs"
		agentGatewayNamespace = "default"
		configMapName         = "test-configmap"
		secretName            = "test-secret"
		timeout               = time.Second * 10
		interval              = time.Millisecond * 250
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &AgentGatewayReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: kevents.NewFakeRecorder(100),
		}

		// Create AgentGatewayClass
		agentGatewayClass := utils.CreateTestAgentGatewayClassWithDefault("default-class", AgentGatewayKrakendControllerName)
		Expect(k8sClient.Create(ctx, agentGatewayClass)).To(Succeed())

		// Create test ConfigMap
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: agentGatewayNamespace,
			},
			Data: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		}
		Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

		// Create test Secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: agentGatewayNamespace,
			},
			Data: map[string][]byte{
				"password": []byte("secret-value"),
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
	})

	AfterEach(func() {
		utils.CleanupAllResources(ctx, k8sClient)
	})

	It("should add hash annotations for referenced ConfigMaps and Secrets", func() {
		// Create AgentGateway with EnvFrom references
		agentGateway := &agentruntimev1alpha1.AgentGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			},
			Spec: agentruntimev1alpha1.AgentGatewaySpec{
				Replicas: utils.Int32Ptr(1),
				EnvFrom: []corev1.EnvFromSource{
					{
						ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: configMapName,
							},
						},
					},
					{
						SecretRef: &corev1.SecretEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: secretName,
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

		// Reconcile
		utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

		// Fetch deployment and check annotations
		deployment := &appsv1.Deployment{}
		utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)

		annotations := deployment.Spec.Template.Annotations
		Expect(annotations).To(HaveKey("runtime.agentic-layer.ai/ConfigMap-" + configMapName + "-hash"))
		Expect(annotations).To(HaveKey("runtime.agentic-layer.ai/Secret-" + secretName + "-hash"))
		Expect(annotations).To(HaveKey("runtime.agentic-layer.ai/config-hash"))

		// Store the initial hashes
		initialConfigMapHash := annotations["runtime.agentic-layer.ai/ConfigMap-"+configMapName+"-hash"]
		initialSecretHash := annotations["runtime.agentic-layer.ai/Secret-"+secretName+"-hash"]

		Expect(initialConfigMapHash).NotTo(BeEmpty())
		Expect(initialSecretHash).NotTo(BeEmpty())
	})

	It("should update hash annotations when ConfigMap content changes", func() {
		// Create AgentGateway with ConfigMap reference
		agentGateway := &agentruntimev1alpha1.AgentGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			},
			Spec: agentruntimev1alpha1.AgentGatewaySpec{
				Replicas: utils.Int32Ptr(1),
				Env: []corev1.EnvVar{
					{
						Name: "TEST_VAR",
						ValueFrom: &corev1.EnvVarSource{
							ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: configMapName,
								},
								Key: "key1",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

		// Initial reconcile
		utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

		// Get initial hash
		deployment := &appsv1.Deployment{}
		utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)
		initialHash := deployment.Spec.Template.Annotations["runtime.agentic-layer.ai/ConfigMap-"+configMapName+"-hash"]
		Expect(initialHash).NotTo(BeEmpty())

		// Update ConfigMap
		configMap := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      configMapName,
			Namespace: agentGatewayNamespace,
		}, configMap)).To(Succeed())

		configMap.Data["key1"] = "updated-value"
		Expect(k8sClient.Update(ctx, configMap)).To(Succeed())

		// Reconcile again
		utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

		// Get updated deployment
		updatedDeployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      agentGatewayName,
			Namespace: agentGatewayNamespace,
		}, updatedDeployment)).To(Succeed())

		newHash := updatedDeployment.Spec.Template.Annotations["runtime.agentic-layer.ai/ConfigMap-"+configMapName+"-hash"]
		Expect(newHash).NotTo(BeEmpty())
		Expect(newHash).NotTo(Equal(initialHash), "Hash should change when ConfigMap content changes")
	})

	It("should handle missing ConfigMaps gracefully", func() {
		// Create AgentGateway referencing a non-existent ConfigMap
		agentGateway := &agentruntimev1alpha1.AgentGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      agentGatewayName,
				Namespace: agentGatewayNamespace,
			},
			Spec: agentruntimev1alpha1.AgentGatewaySpec{
				Replicas: utils.Int32Ptr(1),
				EnvFrom: []corev1.EnvFromSource{
					{
						ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "non-existent-configmap",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, agentGateway)).To(Succeed())

		// Reconcile should succeed despite missing ConfigMap
		utils.ReconcileAndExpectSuccess(ctx, reconciler, agentGatewayName, agentGatewayNamespace)

		// Verify deployment was created
		deployment := &appsv1.Deployment{}
		utils.EventuallyResourceExists(ctx, k8sClient, agentGatewayName, agentGatewayNamespace, deployment, timeout, interval)

		// Should not have annotation for missing ConfigMap
		annotations := deployment.Spec.Template.Annotations
		Expect(annotations).NotTo(HaveKey("runtime.agentic-layer.ai/ConfigMap-non-existent-configmap-hash"))
	})
})

// Helper functions

func stringPtr(s string) *string {
	return &s
}

func verifyOwnerReference(obj client.Object, owner *agentruntimev1alpha1.AgentGateway) {
	ownerRefs := obj.GetOwnerReferences()
	Expect(ownerRefs).To(HaveLen(1))
	ownerRef := ownerRefs[0]
	Expect(ownerRef.Kind).To(Equal("AgentGateway"))
	Expect(ownerRef.Name).To(Equal(owner.Name))
	Expect(ownerRef.UID).To(Equal(owner.UID))
	Expect(*ownerRef.Controller).To(BeTrue())
}

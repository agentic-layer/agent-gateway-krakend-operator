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

package e2e

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/agent-gateway-krakend-operator/test/utils"
)

var _ = Describe("RBAC E2E Tests", func() {
	var (
		cleanupResources []func()
		testNamespace    = "rbac-test-ns"
		gatewayName      = "rbac-test-gateway"
	)

	BeforeEach(func() {
		cleanupResources = []func(){}
	})

	AfterEach(func() {
		// Run all cleanup functions in reverse order
		for i := len(cleanupResources) - 1; i >= 0; i-- {
			cleanupResources[i]()
		}
	})

	Describe("Test 1: Gateway Pod Can Access Agent CRDs", func() {
		It("should verify RBAC permissions allow gateway to list agents", func() {
			By("creating test namespace")
			_, err := utils.Run(exec.Command("kubectl", "create", "namespace", testNamespace))
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Namespace creation info: %v\n", err)
			}
			cleanupResources = append(cleanupResources, func() {
				_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", testNamespace))
			})

			By("deploying test agent")
			agentYAML := fmt.Sprintf(`
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: Agent
metadata:
  name: rbac-test-agent
  namespace: %s
spec:
  exposed: true
  framework: custom
  image: %s
  replicas: 1
  protocols:
    - type: A2A
      port: 8000
`, testNamespace, testAgentImage)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewBufferString(agentYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cleanupResources = append(cleanupResources, func() {
				_, _ = utils.Run(exec.Command("kubectl", "delete", "agent", "rbac-test-agent", "-n", testNamespace))
			})

			// Wait for agent pod to be ready
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "get", "pods",
					"-l", "app=rbac-test-agent",
					"-n", testNamespace,
					"-o", "jsonpath={.items[0].status.phase}"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("deploying AgentGateway")
			gatewayYAML := fmt.Sprintf(`
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: AgentGateway
metadata:
  name: %s
  namespace: %s
spec:
  agentGatewayClassName: krakend
  replicas: 1
`, gatewayName, testNamespace)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewBufferString(gatewayYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cleanupResources = append(cleanupResources, func() {
				_, _ = utils.Run(exec.Command("kubectl", "delete", "agentgateway", gatewayName, "-n", testNamespace))
			})

			By("waiting for gateway deployment to be ready")
			Eventually(func(g Gomega) {
				err := utils.VerifyDeploymentReady(gatewayName, testNamespace, 2*time.Minute)
				g.Expect(err).NotTo(HaveOccurred())
			}, testTimeout, pollingInterval).Should(Succeed())

			By("verifying ServiceAccount was created")
			serviceAccountName := gatewayName + "-gateway"
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "get", "serviceaccount",
					serviceAccountName, "-n", testNamespace, "-o", "name"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring(serviceAccountName))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying ClusterRole was created")
			clusterRoleName := testNamespace + "." + gatewayName + ".gateway-reader"
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "get", "clusterrole",
					clusterRoleName, "-o", "name"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring(clusterRoleName))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying ClusterRole has correct permissions")
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "get", "clusterrole",
					clusterRoleName, "-o", "yaml"))
				g.Expect(err).NotTo(HaveOccurred())

				// Verify the role allows listing agents
				g.Expect(output).To(ContainSubstring("runtime.agentic-layer.ai"))
				g.Expect(output).To(ContainSubstring("agents"))
				g.Expect(output).To(ContainSubstring("get"))
				g.Expect(output).To(ContainSubstring("list"))
				g.Expect(output).To(ContainSubstring("watch"))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying ClusterRoleBinding was created")
			clusterRoleBindingName := testNamespace + "." + gatewayName + ".gateway-reader-binding"
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "get", "clusterrolebinding",
					clusterRoleBindingName, "-o", "name"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring(clusterRoleBindingName))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying ClusterRoleBinding references correct ServiceAccount")
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "get", "clusterrolebinding",
					clusterRoleBindingName, "-o", "yaml"))
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(output).To(ContainSubstring(serviceAccountName))
				g.Expect(output).To(ContainSubstring(testNamespace))
				g.Expect(output).To(ContainSubstring(clusterRoleName))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying gateway pod is using the correct ServiceAccount")
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "get", "pods",
					"-l", fmt.Sprintf("app=%s", gatewayName),
					"-n", testNamespace,
					"-o", "jsonpath={.items[0].spec.serviceAccountName}"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(serviceAccountName))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying gateway pod can successfully start (RBAC is working)")
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "get", "pods",
					"-l", fmt.Sprintf("app=%s", gatewayName),
					"-n", testNamespace,
					"-o", "jsonpath={.items[0].status.phase}"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying no RBAC-related errors in gateway pod logs")
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "logs",
					"-l", fmt.Sprintf("app=%s", gatewayName),
					"-n", testNamespace,
					"--tail=50"))
				g.Expect(err).NotTo(HaveOccurred())

				// Should not contain RBAC permission errors
				g.Expect(output).NotTo(ContainSubstring("forbidden"))
				g.Expect(output).NotTo(ContainSubstring("cannot list resource"))
				g.Expect(output).NotTo(ContainSubstring("User \"system:serviceaccount"))
			}, 30*time.Second, 5*time.Second).Should(Succeed())

			By("verifying gateway successfully discovered the agent")
			Eventually(func(g Gomega) {
				// Check ConfigMap contains agent configuration
				output, err := utils.Run(exec.Command("kubectl", "get", "configmap",
					gatewayName+"-krakend-config",
					"-n", testNamespace,
					"-o", "jsonpath={.data.krakend\\.json}"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("rbac-test-agent"))
			}, 1*time.Minute, 5*time.Second).Should(Succeed())
		})
	})

	Describe("Test 2: RBAC Resources Are Created", func() {
		It("should create all necessary RBAC resources when gateway is deployed", func() {
			By("creating test namespace")
			_, err := utils.Run(exec.Command("kubectl", "create", "namespace", testNamespace))
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Namespace creation info: %v\n", err)
			}
			cleanupResources = append(cleanupResources, func() {
				_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", testNamespace))
			})

			By("deploying AgentGateway")
			gatewayYAML := fmt.Sprintf(`
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: AgentGateway
metadata:
  name: %s
  namespace: %s
spec:
  agentGatewayClassName: krakend
  replicas: 1
`, gatewayName, testNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewBufferString(gatewayYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cleanupResources = append(cleanupResources, func() {
				_, _ = utils.Run(exec.Command("kubectl", "delete", "agentgateway", gatewayName, "-n", testNamespace))
			})

			By("verifying all RBAC resources are created")
			serviceAccountName := gatewayName + "-gateway"
			clusterRoleName := testNamespace + "." + gatewayName + ".gateway-reader"
			clusterRoleBindingName := testNamespace + "." + gatewayName + ".gateway-reader-binding"

			Eventually(func(g Gomega) {
				// Verify ServiceAccount
				output, err := utils.Run(exec.Command("kubectl", "get", "serviceaccount",
					serviceAccountName, "-n", testNamespace, "-o", "name"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring(serviceAccountName))

				// Verify ClusterRole
				output, err = utils.Run(exec.Command("kubectl", "get", "clusterrole",
					clusterRoleName, "-o", "name"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring(clusterRoleName))

				// Verify ClusterRoleBinding
				output, err = utils.Run(exec.Command("kubectl", "get", "clusterrolebinding",
					clusterRoleBindingName, "-o", "name"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring(clusterRoleBindingName))
			}, 1*time.Minute, 5*time.Second).Should(Succeed())

			cleanupResources = append(cleanupResources, func() {
				_, _ = utils.Run(exec.Command("kubectl", "delete", "clusterrolebinding", clusterRoleBindingName))
				_, _ = utils.Run(exec.Command("kubectl", "delete", "clusterrole", clusterRoleName))
			})

			By("verifying ClusterRoleBinding links ServiceAccount to ClusterRole")
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "get", "clusterrolebinding",
					clusterRoleBindingName, "-o", "yaml"))
				g.Expect(err).NotTo(HaveOccurred())

				// Verify it references the correct ServiceAccount and ClusterRole
				g.Expect(output).To(ContainSubstring(serviceAccountName))
				g.Expect(output).To(ContainSubstring(testNamespace))
				g.Expect(output).To(ContainSubstring(clusterRoleName))
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})
	})

	Describe("Test 3: Cross-Namespace Agent Discovery", func() {
		It("should verify gateway can discover agents in all namespaces with RBAC", func() {
			By("creating multiple test namespaces")
			testNs1 := "rbac-test-ns-1"
			testNs2 := "rbac-test-ns-2"

			for _, ns := range []string{testNs1, testNs2} {
				_, err := utils.Run(exec.Command("kubectl", "create", "namespace", ns))
				if err != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Namespace creation info: %v\n", err)
				}
				cleanupResources = append(cleanupResources, func(namespace string) func() {
					return func() {
						_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", namespace))
					}
				}(ns))
			}

			By("deploying agents in different namespaces")
			for i, ns := range []string{testNs1, testNs2} {
				agentName := fmt.Sprintf("cross-ns-agent-%d", i+1)
				agentYAML := fmt.Sprintf(`
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: Agent
metadata:
  name: %s
  namespace: %s
spec:
  exposed: true
  framework: custom
  image: %s
  replicas: 1
  protocols:
    - type: A2A
      port: 8000
`, agentName, ns, testAgentImage)

				cmd := exec.Command("kubectl", "apply", "-f", "-")
				cmd.Stdin = bytes.NewBufferString(agentYAML)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				cleanupResources = append(cleanupResources, func(agent, namespace string) func() {
					return func() {
						_, _ = utils.Run(exec.Command("kubectl", "delete", "agent", agent, "-n", namespace))
					}
				}(agentName, ns))
			}

			By("deploying AgentGateway in first namespace")
			gatewayYAML := fmt.Sprintf(`
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: AgentGateway
metadata:
  name: cross-ns-gateway
  namespace: %s
spec:
  agentGatewayClassName: krakend
  replicas: 1
`, testNs1)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = bytes.NewBufferString(gatewayYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cleanupResources = append(cleanupResources, func() {
				_, _ = utils.Run(exec.Command("kubectl", "delete", "agentgateway", "cross-ns-gateway", "-n", testNs1))
			})

			By("waiting for gateway to be ready")
			Eventually(func(g Gomega) {
				err := utils.VerifyDeploymentReady("cross-ns-gateway", testNs1, 2*time.Minute)
				g.Expect(err).NotTo(HaveOccurred())
			}, 3*time.Minute, 10*time.Second).Should(Succeed())

			By("verifying gateway discovered agents from both namespaces")
			Eventually(func(g Gomega) {
				output, err := utils.Run(exec.Command("kubectl", "get", "configmap",
					"cross-ns-gateway-krakend-config",
					"-n", testNs1,
					"-o", "jsonpath={.data.krakend\\.json}"))
				g.Expect(err).NotTo(HaveOccurred())

				// ConfigMap should contain endpoints for both agents
				g.Expect(output).To(ContainSubstring("cross-ns-agent-1"))
				g.Expect(output).To(ContainSubstring("cross-ns-agent-2"))
				g.Expect(output).To(ContainSubstring(testNs1))
				g.Expect(output).To(ContainSubstring(testNs2))
			}, 2*time.Minute, 10*time.Second).Should(Succeed())
		})
	})
})

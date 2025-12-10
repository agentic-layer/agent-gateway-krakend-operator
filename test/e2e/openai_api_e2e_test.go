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
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/agent-gateway-krakend-operator/test/utils"
)

const (
	testAgentImage       = "ghcr.io/agentic-layer/mock-agent:0.6"
	testNamespaceA       = "test-namespace-a"
	testNamespaceB       = "test-namespace-b"
	uniqueAgentName      = "unique-test-agent"
	conflictingAgentName = "conflicting-test-agent"
	testTimeout          = 5 * time.Minute
	pollingInterval      = 10 * time.Second
)

// OpenAIModelsResponse represents the /models endpoint response
type OpenAIModelsResponse struct {
	Object string `json:"object"`
	Data   []struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

// OpenAIChatCompletionRequest represents a chat completion request
type OpenAIChatCompletionRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// OpenAIChatCompletionResponse represents a chat completion response
type OpenAIChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

var _ = Describe("OpenAI API E2E Tests", func() {
	var (
		cleanupResources []func()
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

	Describe("Test 1: Complete OpenAI API Flow for Unique Agent", func() {
		It("should successfully route requests to a unique agent", func() {
			By("creating test namespace")
			_, err := utils.Run(exec.Command("kubectl", "create", "namespace", "default"))
			if err != nil {
				// Namespace might already exist, that's okay
				_, _ = fmt.Fprintf(GinkgoWriter, "Namespace creation info: %v\n", err)
			}

			By("deploying test agent with unique name")
			deployTestAgent("default", uniqueAgentName)
			cleanupResources = append(cleanupResources, func() {
				cleanupTestAgent("default", uniqueAgentName)
			})

			By("deploying AgentGateway")
			deployAgentGateway("default", "test-gateway-unique")
			cleanupResources = append(cleanupResources, func() {
				cleanupAgentGateway("default", "test-gateway-unique")
			})

			By("waiting for gateway to be ready")
			Eventually(func(g Gomega) {
				err := utils.VerifyDeploymentReady("test-gateway-unique", "default", 2*time.Minute)
				g.Expect(err).NotTo(HaveOccurred())
			}, testTimeout, pollingInterval).Should(Succeed())

			By("sending request to /models endpoint")
			var body []byte
			Eventually(func(g Gomega) {
				var statusCode int
				var err error
				body, statusCode, err = utils.MakeServiceGet("default", "test-gateway-unique", 10000, "/models")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statusCode).To(Equal(200))
			}, testTimeout, pollingInterval).Should(Succeed())

			var modelsResp OpenAIModelsResponse
			err = json.Unmarshal(body, &modelsResp)
			Expect(err).NotTo(HaveOccurred())

			// Verify agent appears in list with namespace/name format
			found := false
			expectedModelID := fmt.Sprintf("default/%s", uniqueAgentName)
			for _, model := range modelsResp.Data {
				if model.ID == expectedModelID {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Agent should appear in /models list with correct format")

			By("sending request to /chat/completions endpoint")
			reqBody := OpenAIChatCompletionRequest{
				Model: expectedModelID,
				Messages: []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					{Role: "user", Content: "Hello"},
				},
			}

			Eventually(func(g Gomega) {
				var statusCode int
				var err error
				body, statusCode, err = utils.MakeServicePost("default", "test-gateway-unique", 10000,
					"/chat/completions", reqBody)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statusCode).To(Equal(200))

				var chatResp OpenAIChatCompletionResponse
				err = json.Unmarshal(body, &chatResp)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify response has at least one choice
				g.Expect(chatResp.Choices).ToNot(BeEmpty())
			}, testTimeout, pollingInterval).Should(Succeed())
		})
	})

	Describe("Test 2: Namespace Conflict Resolution E2E", func() {
		It("should correctly route requests using correct format when agents have same name", func() {
			By("creating test namespaces")
			_, err := utils.Run(exec.Command("kubectl", "create", "namespace", testNamespaceA))
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Namespace A creation info: %v\n", err)
			}
			cleanupResources = append(cleanupResources, func() {
				_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", testNamespaceA))
			})

			_, err = utils.Run(exec.Command("kubectl", "create", "namespace", testNamespaceB))
			if err != nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Namespace B creation info: %v\n", err)
			}
			cleanupResources = append(cleanupResources, func() {
				_, _ = utils.Run(exec.Command("kubectl", "delete", "namespace", testNamespaceB))
			})

			By("deploying test agent in namespace A")
			deployTestAgent(testNamespaceA, conflictingAgentName)
			cleanupResources = append(cleanupResources, func() {
				cleanupTestAgent(testNamespaceA, conflictingAgentName)
			})

			By("deploying test agent in namespace B")
			deployTestAgent(testNamespaceB, conflictingAgentName)
			cleanupResources = append(cleanupResources, func() {
				cleanupTestAgent(testNamespaceB, conflictingAgentName)
			})

			By("deploying AgentGateway in namespace A")
			deployAgentGateway(testNamespaceA, "test-gateway-conflict")
			cleanupResources = append(cleanupResources, func() {
				cleanupAgentGateway(testNamespaceA, "test-gateway-conflict")
			})

			By("waiting for gateway to be ready")
			Eventually(func(g Gomega) {
				err := utils.VerifyDeploymentReady("test-gateway-conflict", testNamespaceA, 2*time.Minute)
				g.Expect(err).NotTo(HaveOccurred())
			}, testTimeout, pollingInterval).Should(Succeed())

			By("sending request to /models endpoint")
			var body []byte
			Eventually(func(g Gomega) {
				var statusCode int
				var err error
				body, statusCode, err = utils.MakeServiceGet(testNamespaceA, "test-gateway-conflict", 10000, "/models")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statusCode).To(Equal(200))
			}, testTimeout, pollingInterval).Should(Succeed())

			var modelsResp OpenAIModelsResponse
			err = json.Unmarshal(body, &modelsResp)
			Expect(err).NotTo(HaveOccurred())

			// Verify both agents appear with correct format
			foundA := false
			foundB := false
			for _, model := range modelsResp.Data {
				if model.ID == fmt.Sprintf("%s/%s", testNamespaceA, conflictingAgentName) {
					foundA = true
				}
				if model.ID == fmt.Sprintf("%s/%s", testNamespaceB, conflictingAgentName) {
					foundB = true
				}
			}
			Expect(foundA).To(BeTrue(), "Agent from namespace A should appear with correct ID format")
			Expect(foundB).To(BeTrue(), "Agent from namespace B should appear with correct ID format")

			By("sending request to namespace A agent using correct format")
			reqBodyA := OpenAIChatCompletionRequest{
				Model: fmt.Sprintf("%s/%s", testNamespaceA, conflictingAgentName),
				Messages: []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					{Role: "user", Content: "Hello"},
				},
			}

			Eventually(func(g Gomega) {
				var statusCode int
				var err error
				body, statusCode, err = utils.MakeServicePost(testNamespaceA, "test-gateway-conflict", 10000,
					"/chat/completions", reqBodyA)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statusCode).To(Equal(200))

				var chatResp OpenAIChatCompletionResponse
				err = json.Unmarshal(body, &chatResp)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify response has at least one choice
				g.Expect(chatResp.Choices).ToNot(BeEmpty())
			}, testTimeout, pollingInterval).Should(Succeed())

			By("sending request to namespace B agent using correct format")
			reqBodyB := OpenAIChatCompletionRequest{
				Model: fmt.Sprintf("%s/%s", testNamespaceB, conflictingAgentName),
				Messages: []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					{Role: "user", Content: "Hello"},
				},
			}

			Eventually(func(g Gomega) {
				var statusCode int
				var err error
				body, statusCode, err = utils.MakeServicePost(testNamespaceA, "test-gateway-conflict", 10000,
					"/chat/completions", reqBodyB)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statusCode).To(Equal(200))

				var chatResp OpenAIChatCompletionResponse
				err = json.Unmarshal(body, &chatResp)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify response has at least one choice
				g.Expect(chatResp.Choices).ToNot(BeEmpty())
			}, testTimeout, pollingInterval).Should(Succeed())

			By("verifying simple format request returns error for conflicting names")
			reqBodySimple := OpenAIChatCompletionRequest{
				Model: conflictingAgentName,
				Messages: []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					{Role: "user", Content: "Hello"},
				},
			}

			Eventually(func(g Gomega) {
				var statusCode int
				var err error
				body, statusCode, err = utils.MakeServicePost(testNamespaceA, "test-gateway-conflict", 10000,
					"/chat/completions", reqBodySimple)
				g.Expect(err).NotTo(HaveOccurred())

				// Expect error response (4xx status code)
				g.Expect(statusCode).To(BeNumerically(">=", 400))
				g.Expect(statusCode).To(BeNumerically("<", 500))
			}, testTimeout, pollingInterval).Should(Succeed())
		})
	})
})

// Helper functions

func deployTestAgent(namespace, name string) {
	By(fmt.Sprintf("creating test agent %s in namespace %s", name, namespace))

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
`, name, namespace, testAgentImage)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = bytes.NewBufferString(agentYAML)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	// Wait for agent pod to be ready
	Eventually(func(g Gomega) {
		output, err := utils.Run(exec.Command("kubectl", "get", "pods",
			"-l", fmt.Sprintf("app=%s", name),
			"-n", namespace,
			"-o", "jsonpath={.items[0].status.phase}"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal("Running"))
	}, 3*time.Minute, 5*time.Second).Should(Succeed())
}

func cleanupTestAgent(namespace, name string) {
	By(fmt.Sprintf("cleaning up test agent %s in namespace %s", name, namespace))
	_, _ = utils.Run(exec.Command("kubectl", "delete", "agent", name, "-n", namespace))
}

func deployAgentGateway(namespace, name string) {
	By(fmt.Sprintf("creating AgentGateway %s in namespace %s", name, namespace))

	gatewayYAML := fmt.Sprintf(`
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: AgentGateway
metadata:
  name: %s
  namespace: %s
spec:
  agentGatewayClassName: krakend
  replicas: 1
`, name, namespace)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = bytes.NewBufferString(gatewayYAML)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func cleanupAgentGateway(namespace, name string) {
	By(fmt.Sprintf("cleaning up AgentGateway %s in namespace %s", name, namespace))
	_, _ = utils.Run(exec.Command("kubectl", "delete", "agentgateway", name, "-n", namespace))
}

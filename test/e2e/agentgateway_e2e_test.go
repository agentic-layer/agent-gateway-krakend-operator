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
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/agent-gateway-krakend-operator/test/utils"
)

var _ = Describe("Agent Gateway", Ordered, func() {

	var gatewayUrl string
	var portForwardCancel context.CancelFunc

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			fetchControllerManagerPodLogs()
			fetchKubernetesEvents()
		}
	})

	BeforeAll(func() {
		By("applying agent and gateway together")
		_, err := utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/runtime_v1alpha1_combined.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply agent and gateway samples")

		By("waiting for agent to be ready")
		err = utils.VerifyAgentReady("mocked-agent-exposed-1", "default", 3*time.Minute)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for agent gateway deployment to be ready")
		Eventually(func() error {
			return utils.VerifyDeploymentReady("agent-gateway", "default", 3*time.Minute)
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "Agent gateway deployment should be ready")

		By("waiting for gateway to have agent endpoints")
		Eventually(func() error {
			// Create temporary port-forward to check endpoint availability
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			localPort, err := utils.PortForwardService(ctx, "default", "agent-gateway", 10000)
			if err != nil {
				return fmt.Errorf("port-forward failed: %w", err)
			}

			testUrl := fmt.Sprintf("http://localhost:%d/mocked-agent-exposed-1/.well-known/agent-card.json", localPort)
			_, err = utils.GetRequest(testUrl)
			if err != nil {
				return fmt.Errorf("agent endpoint not yet available: %w", err)
			}
			return nil
		}, 2*time.Minute, 5*time.Second).Should(Succeed(), "Gateway should have agent endpoints after reconciliation")

		By("creating stable port-forward for tests")
		ctx, cancel := context.WithCancel(context.Background())
		portForwardCancel = cancel
		DeferCleanup(cancel)
		localPort, err := utils.PortForwardService(ctx, "default", "agent-gateway", 10000)
		Expect(err).NotTo(HaveOccurred(), "Failed to create port-forward to agent gateway")

		gatewayUrl = fmt.Sprintf("http://localhost:%d", localPort)
	})

	AfterAll(func() {
		By("cleaning up test resources")
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", "config/samples/runtime_v1alpha1_combined.yaml"))
	})

	It("should proxy A2A requests to agent", func() {

		By("sending HTTP request to the gateway")
		payload := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "message/send",
			"params": map[string]interface{}{
				"message": map[string]interface{}{
					"role": "user",
					"parts": []map[string]interface{}{
						{
							"kind": "text",
							"text": "Message to echo back",
						},
					},
					"messageId": "9229e770-767c-417b-a0b0-f0741243c589",
					"contextId": "abcd1234-5678-90ab-cdef-1234567890ab",
				},
				"metadata": map[string]interface{}{},
			},
		}
		body, err := utils.PostRequest(gatewayUrl+"/mocked-agent-exposed-1", payload)
		Expect(err).NotTo(HaveOccurred(), "Failed to send POST request to agent gateway")

		var responseMap map[string]interface{}
		err = json.Unmarshal(body, &responseMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(responseMap["result"]).NotTo(BeNil())
	})

	It("should retrieve agent card with correct url", func() {
		By("retrieving agent card from the gateway")
		body, err := utils.GetRequest(gatewayUrl + "/mocked-agent-exposed-1/.well-known/agent-card.json")
		Expect(err).NotTo(HaveOccurred(), "Failed to send GET request to agent-card endpoint")

		var responseMap map[string]interface{}
		err = json.Unmarshal(body, &responseMap)
		Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal agent card response")
		Expect(responseMap["url"]).To(Equal(gatewayUrl + "/mocked-agent-exposed-1"))
	})

	It("should reload when agent is deleted", func() {
		By("verifying agent is accessible via agent card")
		_, err := utils.GetRequest(gatewayUrl + "/mocked-agent-exposed-1/.well-known/agent-card.json")
		Expect(err).NotTo(HaveOccurred(), "Agent card should be accessible before deletion")

		By("stopping port-forward before agent deletion")
		portForwardCancel()
		time.Sleep(1 * time.Second) // Allow port-forward to clean up

		By("deleting the agent")
		err = utils.DeleteAgent("mocked-agent-exposed-1", "default")
		Expect(err).NotTo(HaveOccurred(), "Failed to delete agent")

		By("waiting for gateway deployment to complete rollout")
		err = utils.VerifyDeploymentReady("agent-gateway", "default", 3*time.Minute)
		Expect(err).NotTo(HaveOccurred(), "Agent gateway deployment should complete rollout")

		By("verifying deleted agent returns 404")
		Eventually(func() error {
			// Cancel previous port-forward if it exists
			if portForwardCancel != nil {
				portForwardCancel()
				time.Sleep(500 * time.Millisecond) // Allow cleanup
			}

			// Recreate port-forward to restarted gateway
			ctx, cancel := context.WithCancel(context.Background())
			portForwardCancel = cancel
			localPort, err := utils.PortForwardService(ctx, "default", "agent-gateway", 10000)
			if err != nil {
				return fmt.Errorf("failed to create port-forward: %w", err)
			}
			gatewayUrl = fmt.Sprintf("http://localhost:%d", localPort)

			// Wait briefly for port-forward to stabilize
			time.Sleep(1 * time.Second)

			// Check if gateway returns 404 for deleted agent
			_, statusCode, err := utils.GetRequestWithStatus(gatewayUrl + "/mocked-agent-exposed-1/.well-known/agent-card.json")
			if err != nil {
				// Connection/network error - retry
				_, _ = fmt.Fprintf(GinkgoWriter, "Connection error (will retry): %v\n", err)
				return fmt.Errorf("failed to connect to gateway: %w", err)
			}

			// Log the received status code
			_, _ = fmt.Fprintf(GinkgoWriter, "Received HTTP status code: %d\n", statusCode)

			if statusCode == 404 {
				// Success! Gateway correctly removed the deleted agent
				_, _ = fmt.Fprintf(GinkgoWriter, "✓ Gateway correctly returns 404 for deleted agent\n")
				return nil
			}
			if statusCode == 200 {
				// Agent still exists in gateway config - needs more time to reconcile
				_, _ = fmt.Fprintf(GinkgoWriter, "Agent still accessible with HTTP 200, gateway needs more time to reconcile\n")
				return fmt.Errorf("expected 404 for deleted agent, but agent is still accessible (HTTP 200)")
			}
			// Unexpected status code
			_, _ = fmt.Fprintf(GinkgoWriter, "Unexpected status code: %d\n", statusCode)
			return fmt.Errorf("expected 404 for deleted agent, got unexpected status code: %d", statusCode)
		}, 3*time.Minute, 2*time.Second).Should(Succeed(), "Gateway should return 404 for deleted agent")

		By("verified gateway successfully returns 404 for deleted agent")
	})

	It("should reload when agent is added", func() {
		By("verifying agent is not accessible (port-forward exists but agent deleted)")
		_, err := utils.GetRequest(gatewayUrl + "/mocked-agent-exposed-1/.well-known/agent-card.json")
		Expect(err).To(HaveOccurred(), "Agent should not be accessible after deletion")
		Expect(err.Error()).To(ContainSubstring("404"), "Should return 404 for non-existent agent")

		By("stopping port-forward before agent addition")
		portForwardCancel()
		time.Sleep(1 * time.Second) // Allow port-forward to clean up

		By("adding the agent back")
		_, err = utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/runtime_v1alpha1_combined.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply agent")

		By("waiting for agent to be ready")
		err = utils.VerifyAgentReady("mocked-agent-exposed-1", "default", 3*time.Minute)
		Expect(err).NotTo(HaveOccurred(), "Agent should be ready")

		By("waiting for gateway deployment to complete rollout")
		err = utils.VerifyDeploymentReady("agent-gateway", "default", 3*time.Minute)
		Expect(err).NotTo(HaveOccurred(), "Agent gateway deployment should complete rollout")

		By("recreating port-forward to restarted gateway")
		ctx, cancel := context.WithCancel(context.Background())
		portForwardCancel = cancel
		DeferCleanup(cancel)
		localPort, err := utils.PortForwardService(ctx, "default", "agent-gateway", 10000)
		Expect(err).NotTo(HaveOccurred(), "Failed to create port-forward to agent gateway")
		gatewayUrl = fmt.Sprintf("http://localhost:%d", localPort)

		By("verifying agent card has correct URL")
		body, err := utils.GetRequest(gatewayUrl + "/mocked-agent-exposed-1/.well-known/agent-card.json")
		Expect(err).NotTo(HaveOccurred(), "Agent card should be accessible")

		var responseMap map[string]interface{}
		err = json.Unmarshal(body, &responseMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(responseMap["url"]).To(Equal(gatewayUrl + "/mocked-agent-exposed-1"))
	})
})

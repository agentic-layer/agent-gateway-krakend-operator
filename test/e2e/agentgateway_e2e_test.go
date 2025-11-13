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
		// TODO: Once the agent gateway correctly updates itself upon agent changes, join agent and gateway
		// into one file.
		By("applying the agent")
		_, err := utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/runtime_v1alpha1_agent.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply agentgateway sample")

		By("waiting for agent to be ready")
		err = utils.VerifyAgentReady("mocked-agent-exposed-1", "default", 3*time.Minute)
		Expect(err).NotTo(HaveOccurred())

		By("applying the agentgateway")
		_, err = utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/runtime_v1alpha1_agentgateway.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply agentgateway sample")

		By("waiting for agent gateway deployment to be ready")
		Eventually(func() error {
			return utils.VerifyDeploymentReady("agent-gateway", "default", 3*time.Minute)
		}).NotTo(HaveOccurred(), "Agent gateway deployment should be ready")

		By("creating port-forward to the gateway")
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		localPort, err := utils.PortForwardService(ctx, "default", "agent-gateway", 10000)
		Expect(err).NotTo(HaveOccurred(), "Failed to create port-forward to agent gateway")

		gatewayUrl = fmt.Sprintf("http://localhost:%d", localPort)
	})

	AfterAll(func() {
		By("cleaning up test resources")
		_, _ = utils.Run(exec.Command("kubectl", "delete", "agent",
			"-f", "config/samples/runtime_v1alpha1_agent.yaml"))

		_, _ = utils.Run(exec.Command("kubectl", "delete", "agent",
			"-f", "config/samples/runtime_v1alpha1_agentgateway.yaml"))
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
})

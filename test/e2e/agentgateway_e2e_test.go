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
	"encoding/json"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/agent-gateway-krakend-operator/test/utils"
)

var _ = Describe("Agent Gateway", Ordered, func() {

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
		By("applying OTEL configuration ConfigMap")
		_, err := utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/otel-tempo-config.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply OTEL config")

		By("applying agent and gateway together")
		_, err = utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/runtime_v1alpha1_gateway_with_agent.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply agent and gateway samples")
	})

	AfterAll(func() {
		By("cleaning up test resources")
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", "config/samples/runtime_v1alpha1_gateway_with_agent.yaml"))
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", "config/samples/otel-tempo-config.yaml"))
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

		var body []byte
		Eventually(func(g Gomega) {
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServicePost("default", "agent-gateway", 10000,
				"/mocked-agent-exposed-1", payload)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 2*time.Minute, 5*time.Second).Should(Succeed(), "Failed to send POST request to agent gateway")

		var responseMap map[string]interface{}
		err := json.Unmarshal(body, &responseMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(responseMap["result"]).NotTo(BeNil())
	})

	It("should retrieve agent card with correct url", func() {
		By("retrieving agent card from the gateway")

		var body []byte
		Eventually(func(g Gomega) {
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServiceGet("default", "agent-gateway", 10000,
				"/mocked-agent-exposed-1/.well-known/agent-card.json")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 2*time.Minute, 5*time.Second).Should(Succeed(), "Failed to send GET request to agent-card endpoint")

		By("verifying agent card URL")
		var responseMap map[string]interface{}
		err := json.Unmarshal(body, &responseMap)
		Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal agent card response")
		Expect(responseMap["url"]).To(ContainSubstring("/mocked-agent-exposed-1"),
			"Agent card URL should contain the agent path")
	})

	It("should return no agent card for deleted agents", func() {
		By("deleting the agent")
		err := utils.DeleteAgent("mocked-agent-exposed-1", "default")
		Expect(err).NotTo(HaveOccurred(), "Failed to delete agent")

		By("verifying agent-card returns 404")
		Eventually(func(g Gomega) {
			// Make request to check if gateway returns 404 for deleted agent
			_, statusCode, err := utils.MakeServiceGet("default", "agent-gateway", 10000,
				"/mocked-agent-exposed-1/.well-known/agent-card.json")
			g.Expect(err).NotTo(HaveOccurred(), "Failed to send GET request to agent gateway")
			g.Expect(statusCode).To(Equal(404))
		}, 3*time.Minute, 2*time.Second).Should(Succeed(), "Gateway should return 404 for deleted agent")
	})

	It("should proxy added agents", func() {
		By("adding the agent back")
		_, err := utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/runtime_v1alpha1_gateway_with_agent.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply agent")

		By("verifying agent card is accessible and has correct URL")
		var body []byte
		Eventually(func(g Gomega) {
			var err error
			var statusCode int
			body, statusCode, err = utils.MakeServiceGet("default", "agent-gateway", 10000,
				"/mocked-agent-exposed-1/.well-known/agent-card.json")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 2*time.Minute, 5*time.Second).Should(Succeed(), "Agent card should be accessible")

		var responseMap map[string]interface{}
		err = json.Unmarshal(body, &responseMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(responseMap["url"]).To(ContainSubstring("/mocked-agent-exposed-1"),
			"Agent card URL should contain the agent path")
	})
})

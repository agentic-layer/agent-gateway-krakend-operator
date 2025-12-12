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

var _ = Describe("OpenAI API", Ordered, func() {

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

	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			fetchControllerManagerPodLogs()
			fetchKubernetesEvents()
		}
	})

	It("should list agents in /models endpoint", func() {
		By("sending request to /models endpoint")
		var body []byte
		Eventually(func(g Gomega) {
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServiceGet("default", "agent-gateway", 10000, "/models")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying agent appears in list")
		var modelsResp map[string]interface{}
		err := json.Unmarshal(body, &modelsResp)
		Expect(err).NotTo(HaveOccurred())
		Expect(modelsResp["object"]).To(Equal("list"))
		Expect(modelsResp["data"]).NotTo(BeEmpty())

		// Verify our test agent is in the list
		models := modelsResp["data"].([]interface{})
		found := false
		for _, model := range models {
			modelMap := model.(map[string]interface{})
			if modelMap["id"] == "default/mocked-agent-exposed-1" {
				found = true
				Expect(modelMap["object"]).To(Equal("model"))
				break
			}
		}
		Expect(found).To(BeTrue(), "Test agent should appear in /models list")
	})

	It("should route requests via /chat/completions", func() {
		By("sending request to /chat/completions endpoint")
		reqBody := map[string]interface{}{
			"model": "default/mocked-agent-exposed-1",
			"messages": []map[string]interface{}{
				{
					"role":    "user",
					"content": "Hello",
				},
			},
		}

		var body []byte
		Eventually(func(g Gomega) {
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServicePost("default", "agent-gateway", 10000,
				"/chat/completions", reqBody)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying response format")
		var chatResp map[string]interface{}
		err := json.Unmarshal(body, &chatResp)
		Expect(err).NotTo(HaveOccurred())
		Expect(chatResp["object"]).To(Equal("chat.completion"))
		Expect(chatResp["model"]).To(Equal("default/mocked-agent-exposed-1"))

		choices := chatResp["choices"].([]interface{})
		Expect(choices).NotTo(BeEmpty())

		firstChoice := choices[0].(map[string]interface{})
		message := firstChoice["message"].(map[string]interface{})
		Expect(message["role"]).To(Equal("assistant"))
		Expect(message["content"]).NotTo(BeEmpty())
	})
})

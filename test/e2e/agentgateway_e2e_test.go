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
	"fmt"
	"os/exec"
	"strings"
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

	Context("Agent Gateway Routing", func() {
		AfterEach(func() {
			By("cleaning up test resources")
			cmd := exec.Command("kubectl", "delete", "pod", "test-routing")
			_, _ = utils.Run(cmd)

			cmd = exec.Command("kubectl", "delete", "agent", "mocked-agent")
			_, _ = utils.Run(cmd)

			cmd = exec.Command("kubectl", "delete", "agentgateway", "agent-gateway")
			_, _ = utils.Run(cmd)
		})

		It("should deploy an agent and agent gateway and test routing", func() {
			By("deploying a test agent")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/crs/mocked-agent.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy test agent")

			By("waiting for agent deployment to be ready")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "deployment", "mocked-agent", "-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if output == "" || output == "0" {
					return fmt.Errorf("mocked-agent deployment not ready")
				}
				return nil
			}, 3*time.Minute, 10*time.Second).Should(Succeed(), "Agent deployment should be ready")

			By("deploying a test agent gateway")
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/crs/agent-gateway.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy test agent gateway")

			By("waiting for agent gateway to be ready")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "deployment", "agent-gateway", "-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if output == "" || output == "0" {
					return fmt.Errorf("agent gateway deployment not ready yet, status: %s", output)
				}
				return nil
			}, 3*time.Minute, 10*time.Second).Should(Succeed(), "Agent gateway should be ready")

			By("testing routing from gateway to agent")
			cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/crs/curl-routing-test.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create routing test pod")

			By("checking that the response contains HTTP 200 status")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "logs", "test-routing")
				logOutput, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if !strings.Contains(logOutput, "200 OK") {
					return fmt.Errorf("expected HTTP 200 status code, got: %s", logOutput)
				}
				return nil
			}, 1*time.Minute, 5*time.Second).Should(Succeed(), "Should receive HTTP 200 response")
		})
	})
})

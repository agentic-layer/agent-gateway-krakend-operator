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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/agent-gateway-krakend-operator/test/utils"
)

// namespace where the project is deployed in
const namespace = "agent-gateway-krakend-operator-system"

// serviceAccountName created for the project
const serviceAccountName = "agent-gateway-krakend-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "agent-gateway-krakend-operator-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "agent-gateway-krakend-operator-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string
	const agentRuntimeInstallUrl = "https://github.com/agentic-layer/agent-runtime-operator/releases/" +
		"download/v0.4.5/install.yaml"

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("deploying the agent runtime")
		cmd = exec.Command("kubectl", "apply", "-f", agentRuntimeInstallUrl)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the agent runtime")

		By("waiting for agent-runtime-operator-controller-manager to be ready")
		Eventually(func() error {
			cmd := exec.Command("kubectl", "get", "deployment",
				"agent-runtime-operator-controller-manager", "-n", "agent-runtime-operator-system",
				"-o", "jsonpath={.status.readyReplicas}")
			output, err := utils.Run(cmd)
			if err != nil {
				return err
			}
			if output == "" || output == "0" {
				return fmt.Errorf("agent-runtime-operator deployment not ready")
			}
			return nil
		}, 2*time.Minute, 10*time.Second).Should(Succeed(), "agent-runtime-operator deployment should be ready")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("cleaning up the agent runtime")
		cmd = exec.Command("kubectl", "delete", "-f", agentRuntimeInstallUrl)
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=agent-gateway-krakend-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccount": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		Context("Agent Gateway Routing", func() {
			AfterEach(func() {
				By("cleaning up test resources")
				cmd := exec.Command("kubectl", "delete", "pod", "test-routing")
				_, _ = utils.Run(cmd)

				cmd = exec.Command("kubectl", "delete", "pod", "wiremock-config")
				_, _ = utils.Run(cmd)

				cmd = exec.Command("kubectl", "delete", "agent", "mocked-agent")
				_, _ = utils.Run(cmd)

				cmd = exec.Command("kubectl", "delete", "agentgateway", "agent-gateway")
				_, _ = utils.Run(cmd)
			})

			It("should deploy an agent and agent gateway and test routing", func() {
				By("deploying a test agent")
				cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/crs/mocked_agent.yaml")
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

				By("configuring wiremock with test endpoint")
				cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/crs/wiremock-config.yaml")
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to configure wiremock endpoint")

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
				cmd = exec.Command("kubectl", "run", "test-routing", "--restart=Never",
					"--image=curlimages/curl:latest",
					"--overrides",
					`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -XPOST -v http://agent-gateway:10000/mocked-agent"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}]
					}
				}`)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Gateway should successfully route to agent")

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

			It("should route agent card endpoint requests correctly", func() {
				By("deploying a test agent")
				cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/crs/mocked_agent.yaml")
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

				By("configuring wiremock with agent card endpoint")
				cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/crs/wiremock-config.yaml")
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to configure wiremock endpoint")

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

				By("testing agent card endpoint routing from gateway to agent")
				cmd = exec.Command("kubectl", "run", "test-routing", "--restart=Never",
					"--image=curlimages/curl:latest",
					"--overrides",
					`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v http://agent-gateway:10000/mocked-agent/.well-known/agent-card.json"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}]
					}
				}`)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Gateway should successfully route to agent card endpoint")

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

				By("verifying response contains valid agent card JSON")
				Eventually(func() error {
					cmd := exec.Command("kubectl", "logs", "test-routing")
					logOutput, err := utils.Run(cmd)
					if err != nil {
						return err
					}
					if !strings.Contains(logOutput, `"name":"mocked-agent"`) {
						return fmt.Errorf("response does not contain expected agent card JSON")
					}
					if !strings.Contains(logOutput, `"description":"A test agent for e2e tests"`) {
						return fmt.Errorf("response does not contain expected agent description")
					}
					return nil
				}, 1*time.Minute, 5*time.Second).Should(Succeed(), "Should receive valid agent card JSON response")
			})

			It("should return properly formatted agent card data", func() {
				By("deploying a test agent")
				cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/crs/mocked_agent.yaml")
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

				By("configuring wiremock with complete agent card")
				cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/crs/wiremock-config.yaml")
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to configure wiremock endpoint")

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

				By("requesting agent card from gateway")
				cmd = exec.Command("kubectl", "run", "test-routing", "--restart=Never",
					"--image=curlimages/curl:latest",
					"--overrides",
					`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v http://agent-gateway:10000/mocked-agent/.well-known/agent-card.json"],
							"securityContext": {
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}]
					}
				}`)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Should successfully request agent card")

				By("validating required schema fields are present")
				Eventually(func() error {
					cmd := exec.Command("kubectl", "logs", "test-routing")
					logOutput, err := utils.Run(cmd)
					if err != nil {
						return err
					}

					// Validate HTTP 200 response
					if !strings.Contains(logOutput, "200 OK") {
						return fmt.Errorf("expected HTTP 200 status code")
					}

					// Validate required string fields
					requiredStringFields := []string{
						`"name":"mocked-agent"`,
						`"description":"A test agent for e2e tests"`,
						`"version":"1.0.0"`,
					}
					for _, field := range requiredStringFields {
						if !strings.Contains(logOutput, field) {
							return fmt.Errorf("missing required field: %s", field)
						}
					}

					// Validate capabilities array
					if !strings.Contains(logOutput, `"capabilities"`) {
						return fmt.Errorf("missing capabilities field")
					}
					if !strings.Contains(logOutput, `"conversation"`) {
						return fmt.Errorf("capabilities should contain conversation")
					}
					if !strings.Contains(logOutput, `"task-execution"`) {
						return fmt.Errorf("capabilities should contain task-execution")
					}

					// Validate protocols object
					if !strings.Contains(logOutput, `"protocols"`) {
						return fmt.Errorf("missing protocols field")
					}
					if !strings.Contains(logOutput, `"a2a"`) {
						return fmt.Errorf("protocols should contain a2a")
					}

					// Validate metadata object
					if !strings.Contains(logOutput, `"metadata"`) {
						return fmt.Errorf("missing metadata field")
					}
					if !strings.Contains(logOutput, `"author":"PAAL Team"`) {
						return fmt.Errorf("metadata should contain author")
					}
					if !strings.Contains(logOutput, `"license":"Apache-2.0"`) {
						return fmt.Errorf("metadata should contain license")
					}

					// Validate endpoints array
					if !strings.Contains(logOutput, `"endpoints"`) {
						return fmt.Errorf("missing endpoints field")
					}

					return nil
				}, 1*time.Minute, 5*time.Second).Should(Succeed(), "Should return complete agent card schema")
			})

			It("should create ConfigMap with correct plugin configuration", func() {
				By("deploying a test agent")
				cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/crs/mocked_agent.yaml")
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

				By("waiting for ConfigMap to be created")
				Eventually(func() error {
					cmd := exec.Command("kubectl", "get", "configmap", "agent-gateway-krakend-config")
					_, err := utils.Run(cmd)
					return err
				}, 2*time.Minute, 5*time.Second).Should(Succeed(), "ConfigMap should be created")

				By("extracting and validating ConfigMap JSON content")
				var configMapJSON string
				Eventually(func() error {
					cmd := exec.Command("kubectl", "get", "configmap",
						"agent-gateway-krakend-config",
						"-o", "jsonpath={.data.krakend\\.json}")
					output, err := utils.Run(cmd)
					if err != nil {
						return err
					}
					if output == "" {
						return fmt.Errorf("ConfigMap data is empty")
					}
					configMapJSON = output
					return nil
				}, 1*time.Minute, 5*time.Second).Should(Succeed(), "ConfigMap should contain JSON data")

				By("parsing JSON and validating plugin_names")
				var krakendConfig map[string]interface{}
				err = json.Unmarshal([]byte(configMapJSON), &krakendConfig)
				Expect(err).NotTo(HaveOccurred(), "Should be valid JSON")

				extraConfig, ok := krakendConfig["extra_config"].(map[string]interface{})
				Expect(ok).To(BeTrue(), "extra_config should be a map")

				pluginHTTPServer, ok := extraConfig["plugin/http-server"].(map[string]interface{})
				Expect(ok).To(BeTrue(), "plugin/http-server should be a map")

				pluginNames, ok := pluginHTTPServer["name"].([]interface{})
				Expect(ok).To(BeTrue(), "name should be an array")

				Expect(pluginNames).To(HaveLen(2), "should have exactly 2 plugins")
				Expect(pluginNames[0]).To(Equal("agentcard-rw"), "first plugin should be agentcard-rw")
				Expect(pluginNames[1]).To(Equal("openai-a2a"), "second plugin should be openai-a2a")

				By("verifying plugin order is correct (order matters per code comment)")
				// The order is important: agentcard-rw should come before openai-a2a
				// because last entry is outermost/first handler
				Expect(pluginNames[0]).To(Equal("agentcard-rw"))
				Expect(pluginNames[1]).To(Equal("openai-a2a"))
			})
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}

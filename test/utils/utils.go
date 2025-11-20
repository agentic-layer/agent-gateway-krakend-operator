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

package utils

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2" // nolint:revive,staticcheck
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const (
	prometheusOperatorVersion = "v0.77.1"
	prometheusOperatorURL     = "https://github.com/prometheus-operator/prometheus-operator/" +
		"releases/download/%s/bundle.yaml"

	certmanagerVersion = "v1.19.1"
	certmanagerURLTmpl = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"
)

// RequestFunc defines a function that makes an HTTP request given a base URL (host:port)
type RequestFunc func(baseURL string) (body []byte, statusCode int, err error)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %q\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}

	return string(output), nil
}

// InstallPrometheusOperator installs the prometheus Operator to be used to export the enabled metrics.
func InstallPrometheusOperator() error {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "create", "-f", url)
	_, err := Run(cmd)
	return err
}

// UninstallPrometheusOperator uninstalls the prometheus
func UninstallPrometheusOperator() {
	url := fmt.Sprintf(prometheusOperatorURL, prometheusOperatorVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// IsPrometheusCRDsInstalled checks if any Prometheus CRDs are installed
// by verifying the existence of key CRDs related to Prometheus.
func IsPrometheusCRDsInstalled() bool {
	// List of common Prometheus CRDs
	prometheusCRDs := []string{
		"prometheuses.monitoring.coreos.com",
		"prometheusrules.monitoring.coreos.com",
		"prometheusagents.monitoring.coreos.com",
	}

	cmd := exec.Command("kubectl", "get", "crds", "-o", "custom-columns=NAME:.metadata.name")
	output, err := Run(cmd)
	if err != nil {
		return false
	}
	crdList := GetNonEmptyLines(output)
	for _, crd := range prometheusCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// UninstallCertManager uninstalls the cert manager
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// InstallCertManager installs the cert manager bundle.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// Additionally wait for the CA bundle to be populated in the webhook configuration
	// to ensure the webhook is fully operational.
	// Certificates are populated asynchronously after the webhook deployment is ready.
	cmd = exec.Command("kubectl", "wait", "validatingwebhookconfiguration/cert-manager-webhook",
		"--for", "jsonpath={.webhooks[0].clientConfig.caBundle}",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)
	_, err := Run(cmd)

	return err
}

// IsCertManagerCRDsInstalled checks if any Cert Manager CRDs are installed
// by verifying the existence of key CRDs related to Cert Manager.
func IsCertManagerCRDsInstalled() bool {
	// List of common Cert Manager CRDs
	certManagerCRDs := []string{
		"certificates.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"orders.acme.cert-manager.io",
		"challenges.acme.cert-manager.io",
	}

	// Execute the kubectl command to get all CRDs
	cmd := exec.Command("kubectl", "get", "crds")
	output, err := Run(cmd)
	if err != nil {
		return false
	}

	// Check if any of the Cert Manager CRDs are present
	crdList := GetNonEmptyLines(output)
	for _, crd := range certManagerCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// LoadImageToKindClusterWithName loads a local docker image to the kind cluster
func LoadImageToKindClusterWithName(name string) error {
	cluster := "kind"
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindOptions := []string{"load", "docker-image", name, "--name", cluster}
	cmd := exec.Command("kind", kindOptions...)
	_, err := Run(cmd)
	return err
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, fmt.Errorf("failed to get current working directory: %w", err)
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// resolveServiceToPod resolves a service to one of its backing pods and the target port
func resolveServiceToPod(
	ctx context.Context, clientset *kubernetes.Clientset, namespace, serviceName string, servicePort int,
) (*corev1.Pod, int, error) {
	// Get the service
	service, err := clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get service: %w", err)
	}

	// Check if service has selectors
	if len(service.Spec.Selector) == 0 {
		return nil, 0, fmt.Errorf("service %s does not have a selector", serviceName)
	}

	// Find the service port definition
	var targetPort *int
	var targetPortName string
	for _, port := range service.Spec.Ports {
		if port.Port == int32(servicePort) {
			// Found the matching service port
			if port.TargetPort.Type == 0 { // IntVal
				tp := int(port.TargetPort.IntVal)
				targetPort = &tp
			} else { // StrVal
				targetPortName = port.TargetPort.StrVal
			}
			break
		}
	}

	if targetPort == nil && targetPortName == "" {
		return nil, 0, fmt.Errorf("service %s does not expose port %d", serviceName, servicePort)
	}

	// Convert service selector to label selector
	set := labels.Set(service.Spec.Selector)
	listOptions := metav1.ListOptions{
		LabelSelector: set.AsSelector().String(),
	}

	// List pods matching the service selector
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, listOptions)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return nil, 0, fmt.Errorf("no pods found for service %s", serviceName)
	}

	// Filter for running pods and select the first ready one
	var selectedPod *corev1.Pod
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			// Check if pod is ready
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					selectedPod = &pod
					break
				}
			}
			if selectedPod != nil {
				break
			}
		}
	}

	// If no ready pod found, use first running pod
	if selectedPod == nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				selectedPod = &pod
				break
			}
		}
	}

	if selectedPod == nil {
		return nil, 0, fmt.Errorf("no running pods found for service %s", serviceName)
	}

	// Resolve named port if necessary
	if targetPortName != "" {
		resolved := false
		for _, container := range selectedPod.Spec.Containers {
			for _, port := range container.Ports {
				if port.Name == targetPortName {
					tp := int(port.ContainerPort)
					targetPort = &tp
					resolved = true
					break
				}
			}
			if resolved {
				break
			}
		}
		if !resolved {
			return nil, 0, fmt.Errorf("could not resolve named port %s in pod %s", targetPortName, selectedPod.Name)
		}
	}

	if targetPort == nil {
		return nil, 0, fmt.Errorf("could not determine target port for service %s", serviceName)
	}

	return selectedPod, *targetPort, nil
}

// PortForwardPod creates a port forward to a specific pod
func PortForwardPod(ctx context.Context, namespace, podName string, port int) (int, error) {
	// Get Kubernetes REST config
	config, err := rest.InClusterConfig()
	if err != nil {
		// If not in cluster, try to use kubeconfig
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		config, err = kubeConfig.ClientConfig()
		if err != nil {
			return 0, fmt.Errorf("failed to get kubernetes config: %w", err)
		}
	}

	// Build the port forward URL - must be to a pod
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)

	serverURL, err := neturl.Parse(config.Host)
	if err != nil {
		return 0, fmt.Errorf("failed to parse host URL: %w", err)
	}
	serverURL.Path = path

	// Create a SPDY roundtripper
	transport, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return 0, fmt.Errorf("failed to create roundtripper: %w", err)
	}

	// Create a dialer
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, serverURL)

	// Set up streams
	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{}, 1)
	out := GinkgoWriter
	errOut := GinkgoWriter

	// Use port 0 to get a random local port
	ports := []string{fmt.Sprintf("0:%d", port)}

	// Create the portforwarder
	forwarder, err := portforward.New(dialer, ports, stopChan, readyChan, out, errOut)
	if err != nil {
		return 0, fmt.Errorf("failed to create port forwarder: %w", err)
	}

	// Start port forwarding in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := forwarder.ForwardPorts(); err != nil {
			errChan <- err
		}
	}()

	// Wait for the port forwarder to be ready or fail
	select {
	case <-readyChan:
		// Port forwarder is ready
	case err := <-errChan:
		return 0, fmt.Errorf("port forwarding failed: %w", err)
	case <-ctx.Done():
		close(stopChan)
		return 0, ctx.Err()
	}

	// Get the actual local port that was assigned
	forwardedPorts, err := forwarder.GetPorts()
	if err != nil {
		close(stopChan)
		return 0, fmt.Errorf("failed to get forwarded ports: %w", err)
	}
	if len(forwardedPorts) == 0 {
		close(stopChan)
		return 0, fmt.Errorf("no ports were forwarded")
	}
	localPort := int(forwardedPorts[0].Local)

	By(fmt.Sprintf("port-forward established: 127.0.0.1:%d -> %s/%s:%d", localPort, namespace, podName, port))

	// Monitor context and stop port forwarding when canceled
	go func() {
		<-ctx.Done()
		_, _ = fmt.Fprintf(GinkgoWriter, "stopping port-forward for %s/%s\n", namespace, podName)
		close(stopChan)
	}()

	return localPort, nil
}

// PortForwardService creates a port forward to a service by resolving it to a pod
func PortForwardService(ctx context.Context, namespace, serviceName string, port int) (int, error) {
	// Get Kubernetes REST config
	config, err := rest.InClusterConfig()
	if err != nil {
		// If not in cluster, try to use kubeconfig
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		config, err = kubeConfig.ClientConfig()
		if err != nil {
			return 0, fmt.Errorf("failed to get kubernetes config: %w", err)
		}
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return 0, fmt.Errorf("failed to create clientset: %w", err)
	}

	// Resolve service to pod and target port
	pod, targetPort, err := resolveServiceToPod(ctx, clientset, namespace, serviceName, port)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve service to pod: %w", err)
	}

	By(fmt.Sprintf("resolved service %s (port %d) to pod %s (port %d)", serviceName, port, pod.Name, targetPort))

	// Port forward to the resolved pod using the target port
	return PortForwardPod(ctx, namespace, pod.Name, targetPort)
}

// MakeGatewayRequest establishes a port-forward to the service, makes an HTTP request, and cleans up.
// The requestFunc receives the base URL (e.g., "http://localhost:12345") and performs the actual request.
// Non-2xx HTTP status codes are returned successfully (not treated as errors), allowing callers
// to verify specific status codes like 404.
func MakeGatewayRequest(
	namespace, serviceName string,
	servicePort int,
	requestFunc RequestFunc,
) (body []byte, statusCode int, err error) {
	// Create fresh port-forward for this request
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localPort, pfErr := PortForwardService(ctx, namespace, serviceName, servicePort)
	if pfErr != nil {
		return nil, 0, fmt.Errorf("port-forward failed: %w", pfErr)
	}

	baseURL := fmt.Sprintf("http://localhost:%d", localPort)
	return requestFunc(baseURL)
}

// MakeGatewayGet is a convenience wrapper for GET requests to the gateway.
func MakeGatewayGet(namespace, serviceName string, servicePort int, endpoint string) ([]byte, int, error) {
	return MakeGatewayRequest(
		namespace, serviceName, servicePort,
		func(baseURL string) ([]byte, int, error) {
			return GetRequestWithStatus(baseURL + endpoint)
		},
	)
}

// MakeGatewayPost is a convenience wrapper for POST requests to the gateway.
func MakeGatewayPost(
	namespace, serviceName string,
	servicePort int,
	endpoint string,
	payload interface{},
) ([]byte, int, error) {
	return MakeGatewayRequest(
		namespace, serviceName, servicePort,
		func(baseURL string) ([]byte, int, error) {
			return PostRequestWithStatus(baseURL+endpoint, payload)
		},
	)
}

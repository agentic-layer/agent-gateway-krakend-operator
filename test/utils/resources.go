package utils

import (
	"fmt"
	"os/exec"
	"time"
)

// VerifyDeploymentReady verifies that a deployment is ready within the given timeout
func VerifyDeploymentReady(name, namespace string, timeout time.Duration) error {
	cmd := exec.Command("kubectl", "wait", "deployment", name, "-n", namespace,
		"--for=condition=Available", "--timeout="+timeout.String())
	if output, err := Run(cmd); err != nil {
		describeDeployment, _ := Run(exec.Command("kubectl", "describe", "deployment", name, "-n", namespace))
		describePods, _ := Run(exec.Command("kubectl", "describe", "pod", "-l", "app="+name, "-n", namespace))
		return fmt.Errorf("deployment is not ready (%s):\n%s\nPods:\n%s",
			output, describeDeployment, describePods,
		)
	}
	return nil
}

// VerifyAgentReady verifies that an agent is ready within the given timeout
func VerifyAgentReady(name, namespace string, timeout time.Duration) error {
	cmd := exec.Command("kubectl", "wait", "agent", name, "-n", namespace,
		"--for=condition=Ready", "--timeout="+timeout.String())
	if output, err := Run(cmd); err != nil {
		describeAgent, _ := Run(exec.Command("kubectl", "describe", "agent", name, "-n", namespace))
		describeDeployment, _ := Run(exec.Command("kubectl", "describe", "deployment", name, "-n", namespace))
		describePods, _ := Run(exec.Command("kubectl", "describe", "pod", "-l", "app="+name, "-n", namespace))
		return fmt.Errorf("deployment is not ready (%s):\nAgent:\n%s\nDeployment:\n%s\nPods:\n%s",
			output, describeAgent, describeDeployment, describePods,
		)
	}

	// Currently, the agent is considered ready even though the deployment is still in progress
	return VerifyDeploymentReady(name, namespace, timeout)
}

// DeleteAgent deletes an agent resource
func DeleteAgent(name, namespace string) error {
	cmd := exec.Command("kubectl", "delete", "agent", name, "-n", namespace)
	if output, err := Run(cmd); err != nil {
		return fmt.Errorf("failed to delete agent %s in namespace %s: %s", name, namespace, output)
	}
	return nil
}

// GetConfigMap retrieves a ConfigMap by name and namespace
func GetConfigMap(name, namespace string) (string, error) {
	cmd := exec.Command("kubectl", "get", "configmap", name, "-n", namespace, "-o", "yaml")
	output, err := Run(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get configmap %s in namespace %s: %w", name, namespace, err)
	}
	return output, nil
}

// UpdateConfigMapData updates a ConfigMap's data field with a patch
func UpdateConfigMapData(name, namespace, key, value string) error {
	patch := fmt.Sprintf(`{"data":{"%s":"%s"}}`, key, value)
	cmd := exec.Command("kubectl", "patch", "configmap", name, "-n", namespace,
		"--type=merge", "-p", patch)
	if output, err := Run(cmd); err != nil {
		return fmt.Errorf("failed to update configmap %s in namespace %s: %s", name, namespace, output)
	}
	return nil
}

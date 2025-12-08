package controller

import (
	"fmt"
	"regexp"
)

const (
	// DNS1123LabelMaxLength is the maximum length for DNS-1123 label names
	DNS1123LabelMaxLength = 253

	// ClusterRoleSuffixLength is the length of the suffix ".gateway-reader"
	ClusterRoleSuffixLength = 15

	// ClusterRoleBindingSuffixLength is the length of the suffix ".gateway-reader-binding"
	ClusterRoleBindingSuffixLength = 23
)

// dns1123LabelRegex validates DNS-1123 label format
// Must consist of lower case alphanumeric characters or '-',
// and must start and end with an alphanumeric character
var dns1123LabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateAgentGatewayName validates that the AgentGateway name and namespace
// will produce valid ClusterRole and ClusterRoleBinding names.
// It checks:
// 1. Namespace and name follow DNS-1123 label format
// 2. Generated ClusterRole name doesn't exceed max length
// 3. Generated ClusterRoleBinding name doesn't exceed max length
func ValidateAgentGatewayName(namespace, name string) error {
	// Validate namespace format
	if !dns1123LabelRegex.MatchString(namespace) {
		return fmt.Errorf("namespace %q must be a valid DNS-1123 label: lowercase alphanumeric characters or '-', starting and ending with alphanumeric", namespace)
	}

	// Validate name format
	if !dns1123LabelRegex.MatchString(name) {
		return fmt.Errorf("name %q must be a valid DNS-1123 label: lowercase alphanumeric characters or '-', starting and ending with alphanumeric", name)
	}

	// Check ClusterRole name length
	clusterRoleName := generateClusterRoleName(namespace, name)
	if len(clusterRoleName) > DNS1123LabelMaxLength {
		return fmt.Errorf("generated ClusterRole name %q exceeds maximum length of %d characters (has %d). Please use a shorter namespace or name",
			clusterRoleName, DNS1123LabelMaxLength, len(clusterRoleName))
	}

	// Check ClusterRoleBinding name length
	bindingName := generateClusterRoleBindingName(namespace, name)
	if len(bindingName) > DNS1123LabelMaxLength {
		return fmt.Errorf("generated ClusterRoleBinding name %q exceeds maximum length of %d characters (has %d). Please use a shorter namespace or name",
			bindingName, DNS1123LabelMaxLength, len(bindingName))
	}

	return nil
}

// generateClusterRoleName generates the ClusterRole name using dot separator format.
// Format: namespace.name.gateway-reader
// This prevents collisions between:
// - foo-bar/baz → foo-bar.baz.gateway-reader
// - foo/bar-baz → foo.bar-baz.gateway-reader
func generateClusterRoleName(namespace, name string) string {
	return fmt.Sprintf("%s.%s.gateway-reader", namespace, name)
}

// generateClusterRoleBindingName generates the ClusterRoleBinding name using dot separator format.
// Format: namespace.name.gateway-reader-binding
// This prevents collisions between:
// - foo-bar/baz → foo-bar.baz.gateway-reader-binding
// - foo/bar-baz → foo.bar-baz.gateway-reader-binding
func generateClusterRoleBindingName(namespace, name string) string {
	return fmt.Sprintf("%s.%s.gateway-reader-binding", namespace, name)
}

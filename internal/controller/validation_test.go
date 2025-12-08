package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAgentGatewayName_ValidNames(t *testing.T) {
	testCases := []struct {
		name      string
		namespace string
		gwName    string
	}{
		{
			name:      "simple lowercase names",
			namespace: "default",
			gwName:    "my-gateway",
		},
		{
			name:      "names with numbers",
			namespace: "ns123",
			gwName:    "gw456",
		},
		{
			name:      "names with hyphens",
			namespace: "my-namespace",
			gwName:    "my-agent-gateway",
		},
		{
			name:      "single character names",
			namespace: "a",
			gwName:    "b",
		},
		{
			name:      "long but valid names",
			namespace: "very-long-namespace-name-with-many-parts",
			gwName:    "very-long-gateway-name-with-many-parts",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAgentGatewayName(tc.namespace, tc.gwName)
			assert.NoError(t, err, "expected valid names to pass validation")
		})
	}
}

func TestValidateAgentGatewayName_InvalidFormat(t *testing.T) {
	testCases := []struct {
		name         string
		namespace    string
		gwName       string
		expectedErr  string
		invalidField string // "namespace" or "name"
	}{
		{
			name:         "uppercase in namespace",
			namespace:    "MyNamespace",
			gwName:       "gateway",
			expectedErr:  "must be a valid DNS-1123 label",
			invalidField: "namespace",
		},
		{
			name:         "uppercase in name",
			namespace:    "default",
			gwName:       "MyGateway",
			expectedErr:  "must be a valid DNS-1123 label",
			invalidField: "name",
		},
		{
			name:         "special character in namespace",
			namespace:    "my_namespace",
			gwName:       "gateway",
			expectedErr:  "must be a valid DNS-1123 label",
			invalidField: "namespace",
		},
		{
			name:         "special character in name",
			namespace:    "default",
			gwName:       "my.gateway",
			expectedErr:  "must be a valid DNS-1123 label",
			invalidField: "name",
		},
		{
			name:         "starts with hyphen in namespace",
			namespace:    "-namespace",
			gwName:       "gateway",
			expectedErr:  "must be a valid DNS-1123 label",
			invalidField: "namespace",
		},
		{
			name:         "ends with hyphen in name",
			namespace:    "default",
			gwName:       "gateway-",
			expectedErr:  "must be a valid DNS-1123 label",
			invalidField: "name",
		},
		{
			name:         "empty namespace",
			namespace:    "",
			gwName:       "gateway",
			expectedErr:  "must be a valid DNS-1123 label",
			invalidField: "namespace",
		},
		{
			name:         "empty name",
			namespace:    "default",
			gwName:       "",
			expectedErr:  "must be a valid DNS-1123 label",
			invalidField: "name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAgentGatewayName(tc.namespace, tc.gwName)
			assert.Error(t, err, "expected invalid format to fail validation")
			assert.Contains(t, err.Error(), tc.expectedErr)
			assert.Contains(t, err.Error(), tc.invalidField)
		})
	}
}

func TestValidateAgentGatewayName_LengthViolations(t *testing.T) {
	testCases := []struct {
		name        string
		namespace   string
		gwName      string
		expectedErr string
	}{
		{
			name: "ClusterRole name exceeds max length",
			// Create names that when combined exceed 253 chars
			// Format: namespace.name.gateway-reader (15 chars for suffix)
			// So we need namespace + name + 1 (dot) + 15 > 253
			// Therefore namespace + name > 237
			namespace:   strings.Repeat("a", 120),
			gwName:      strings.Repeat("b", 120),
			expectedErr: "exceeds maximum length of 253 characters",
		},
		{
			name: "ClusterRoleBinding name exceeds max length",
			// Format: namespace.name.gateway-reader-binding (23 chars for suffix)
			// So we need namespace + name + 1 (dot) + 23 > 253
			// Therefore namespace + name > 229
			namespace:   strings.Repeat("c", 120),
			gwName:      strings.Repeat("d", 120),
			expectedErr: "exceeds maximum length of 253 characters",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAgentGatewayName(tc.namespace, tc.gwName)
			assert.Error(t, err, "expected overly long names to fail validation")
			assert.Contains(t, err.Error(), tc.expectedErr)
		})
	}
}

func TestValidateAgentGatewayName_MaxLengthBoundary(t *testing.T) {
	// Test names that are exactly at the boundary (should pass)
	// ClusterRoleBinding format: namespace.name.gateway-reader-binding
	// 253 (max) - 23 (suffix length) - 1 (dot) = 229 chars for namespace + name
	namespace := strings.Repeat("a", 114)
	gwName := strings.Repeat("b", 115)

	err := ValidateAgentGatewayName(namespace, gwName)
	assert.NoError(t, err, "expected boundary case to pass validation")

	// Verify generated names are within limits
	clusterRoleName := generateClusterRoleName(namespace, gwName)
	assert.LessOrEqual(t, len(clusterRoleName), DNS1123LabelMaxLength)

	bindingName := generateClusterRoleBindingName(namespace, gwName)
	assert.LessOrEqual(t, len(bindingName), DNS1123LabelMaxLength)
}

func TestGenerateClusterRoleName_CollisionPrevention(t *testing.T) {
	testCases := []struct {
		name       string
		namespace1 string
		gwName1    string
		namespace2 string
		gwName2    string
	}{
		{
			name:       "hyphen in namespace vs hyphen in name",
			namespace1: "foo-bar",
			gwName1:    "baz",
			namespace2: "foo",
			gwName2:    "bar-baz",
		},
		{
			name:       "multiple hyphens collision scenario",
			namespace1: "a-b-c",
			gwName1:    "d",
			namespace2: "a-b",
			gwName2:    "c-d",
		},
		{
			name:       "single char vs combined",
			namespace1: "x",
			gwName1:    "y-z",
			namespace2: "x-y",
			gwName2:    "z",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate ClusterRole names
			clusterRoleName1 := generateClusterRoleName(tc.namespace1, tc.gwName1)
			clusterRoleName2 := generateClusterRoleName(tc.namespace2, tc.gwName2)

			// Assert they are different (collision prevented)
			assert.NotEqual(t, clusterRoleName1, clusterRoleName2,
				"Expected different ClusterRole names for %s/%s and %s/%s",
				tc.namespace1, tc.gwName1, tc.namespace2, tc.gwName2)

			// Verify format
			expectedName1 := tc.namespace1 + "." + tc.gwName1 + ".gateway-reader"
			expectedName2 := tc.namespace2 + "." + tc.gwName2 + ".gateway-reader"
			assert.Equal(t, expectedName1, clusterRoleName1)
			assert.Equal(t, expectedName2, clusterRoleName2)
		})
	}
}

func TestGenerateClusterRoleBindingName_CollisionPrevention(t *testing.T) {
	testCases := []struct {
		name       string
		namespace1 string
		gwName1    string
		namespace2 string
		gwName2    string
	}{
		{
			name:       "hyphen in namespace vs hyphen in name",
			namespace1: "foo-bar",
			gwName1:    "baz",
			namespace2: "foo",
			gwName2:    "bar-baz",
		},
		{
			name:       "multiple hyphens collision scenario",
			namespace1: "a-b-c",
			gwName1:    "d",
			namespace2: "a-b",
			gwName2:    "c-d",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate ClusterRoleBinding names
			bindingName1 := generateClusterRoleBindingName(tc.namespace1, tc.gwName1)
			bindingName2 := generateClusterRoleBindingName(tc.namespace2, tc.gwName2)

			// Assert they are different (collision prevented)
			assert.NotEqual(t, bindingName1, bindingName2,
				"Expected different ClusterRoleBinding names for %s/%s and %s/%s",
				tc.namespace1, tc.gwName1, tc.namespace2, tc.gwName2)

			// Verify format
			expectedName1 := tc.namespace1 + "." + tc.gwName1 + ".gateway-reader-binding"
			expectedName2 := tc.namespace2 + "." + tc.gwName2 + ".gateway-reader-binding"
			assert.Equal(t, expectedName1, bindingName1)
			assert.Equal(t, expectedName2, bindingName2)
		})
	}
}

func TestGenerateClusterRoleName_Format(t *testing.T) {
	testCases := []struct {
		namespace string
		gwName    string
		expected  string
	}{
		{
			namespace: "default",
			gwName:    "my-gateway",
			expected:  "default.my-gateway.gateway-reader",
		},
		{
			namespace: "production",
			gwName:    "api-gateway",
			expected:  "production.api-gateway.gateway-reader",
		},
		{
			namespace: "test-ns",
			gwName:    "gw-123",
			expected:  "test-ns.gw-123.gateway-reader",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			actual := generateClusterRoleName(tc.namespace, tc.gwName)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestGenerateClusterRoleBindingName_Format(t *testing.T) {
	testCases := []struct {
		namespace string
		gwName    string
		expected  string
	}{
		{
			namespace: "default",
			gwName:    "my-gateway",
			expected:  "default.my-gateway.gateway-reader-binding",
		},
		{
			namespace: "production",
			gwName:    "api-gateway",
			expected:  "production.api-gateway.gateway-reader-binding",
		},
		{
			namespace: "test-ns",
			gwName:    "gw-123",
			expected:  "test-ns.gw-123.gateway-reader-binding",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			actual := generateClusterRoleBindingName(tc.namespace, tc.gwName)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

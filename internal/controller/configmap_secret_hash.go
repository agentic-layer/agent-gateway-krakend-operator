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

package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ReferencedResource represents a ConfigMap or Secret referenced in the deployment
type ReferencedResource struct {
	Name      string
	Namespace string
	Kind      string // "ConfigMap" or "Secret"
}

// collectReferencedResources discovers all ConfigMaps and Secrets referenced
// in the container environment variables and returns them as a list
func collectReferencedResources(containers []corev1.Container, namespace string) []ReferencedResource {
	resourceMap := make(map[string]ReferencedResource)

	for _, container := range containers {
		// Check Env for ConfigMapKeyRef and SecretKeyRef
		for _, env := range container.Env {
			if env.ValueFrom != nil {
				if env.ValueFrom.ConfigMapKeyRef != nil {
					key := fmt.Sprintf("ConfigMap/%s/%s", namespace, env.ValueFrom.ConfigMapKeyRef.Name)
					resourceMap[key] = ReferencedResource{
						Name:      env.ValueFrom.ConfigMapKeyRef.Name,
						Namespace: namespace,
						Kind:      "ConfigMap",
					}
				}
				if env.ValueFrom.SecretKeyRef != nil {
					key := fmt.Sprintf("Secret/%s/%s", namespace, env.ValueFrom.SecretKeyRef.Name)
					resourceMap[key] = ReferencedResource{
						Name:      env.ValueFrom.SecretKeyRef.Name,
						Namespace: namespace,
						Kind:      "Secret",
					}
				}
			}
		}

		// Check EnvFrom for ConfigMapRef and SecretRef
		for _, envFrom := range container.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				key := fmt.Sprintf("ConfigMap/%s/%s", namespace, envFrom.ConfigMapRef.Name)
				resourceMap[key] = ReferencedResource{
					Name:      envFrom.ConfigMapRef.Name,
					Namespace: namespace,
					Kind:      "ConfigMap",
				}
			}
			if envFrom.SecretRef != nil {
				key := fmt.Sprintf("Secret/%s/%s", namespace, envFrom.SecretRef.Name)
				resourceMap[key] = ReferencedResource{
					Name:      envFrom.SecretRef.Name,
					Namespace: namespace,
					Kind:      "Secret",
				}
			}
		}
	}

	// Convert map to sorted slice for consistent ordering
	resources := make([]ReferencedResource, 0, len(resourceMap))
	for _, resource := range resourceMap {
		resources = append(resources, resource)
	}

	// Sort by kind, namespace, and name for consistent ordering
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Kind != resources[j].Kind {
			return resources[i].Kind < resources[j].Kind
		}
		if resources[i].Namespace != resources[j].Namespace {
			return resources[i].Namespace < resources[j].Namespace
		}
		return resources[i].Name < resources[j].Name
	})

	return resources
}

// generateResourceHash fetches the content of a ConfigMap or Secret and generates a hash
func (r *AgentGatewayReconciler) generateResourceHash(ctx context.Context, resource ReferencedResource) (string, error) {
	log := logf.FromContext(ctx)

	switch resource.Kind {
	case "ConfigMap":
		var cm corev1.ConfigMap
		if err := r.Get(ctx, types.NamespacedName{
			Name:      resource.Name,
			Namespace: resource.Namespace,
		}, &cm); err != nil {
			log.Error(err, "Failed to get ConfigMap for hashing",
				"name", resource.Name,
				"namespace", resource.Namespace)
			return "", err
		}

		// Hash the Data and BinaryData
		h := sha256.New()

		// Sort keys for consistent hashing
		keys := make([]string, 0, len(cm.Data))
		for k := range cm.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte(cm.Data[k]))
		}

		// Hash binary data
		binaryKeys := make([]string, 0, len(cm.BinaryData))
		for k := range cm.BinaryData {
			binaryKeys = append(binaryKeys, k)
		}
		sort.Strings(binaryKeys)

		for _, k := range binaryKeys {
			h.Write([]byte(k))
			h.Write(cm.BinaryData[k])
		}

		return fmt.Sprintf("%x", h.Sum(nil))[:16], nil

	case "Secret":
		var secret corev1.Secret
		if err := r.Get(ctx, types.NamespacedName{
			Name:      resource.Name,
			Namespace: resource.Namespace,
		}, &secret); err != nil {
			log.Error(err, "Failed to get Secret for hashing",
				"name", resource.Name,
				"namespace", resource.Namespace)
			return "", err
		}

		// Hash the Data and StringData
		h := sha256.New()

		// Sort keys for consistent hashing
		keys := make([]string, 0, len(secret.Data))
		for k := range secret.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			h.Write([]byte(k))
			h.Write(secret.Data[k])
		}

		return fmt.Sprintf("%x", h.Sum(nil))[:16], nil

	default:
		return "", fmt.Errorf("unsupported resource kind: %s", resource.Kind)
	}
}

// generateResourceAnnotations generates a map of annotations with hashes for all referenced resources
func (r *AgentGatewayReconciler) generateResourceAnnotations(ctx context.Context, containers []corev1.Container, namespace string) map[string]string {
	log := logf.FromContext(ctx)

	resources := collectReferencedResources(containers, namespace)
	annotations := make(map[string]string)

	for _, resource := range resources {
		hash, err := r.generateResourceHash(ctx, resource)
		if err != nil {
			log.Info("Skipping resource hash due to error",
				"kind", resource.Kind,
				"name", resource.Name,
				"namespace", resource.Namespace,
				"error", err.Error())
			// Continue with other resources instead of failing
			continue
		}

		// Create annotation key in format: runtime.agentic-layer.ai/configmap-<name>-hash
		// or runtime.agentic-layer.ai/secret-<name>-hash
		annotationKey := fmt.Sprintf("runtime.agentic-layer.ai/%s-%s-hash",
			resource.Kind, resource.Name)
		annotations[annotationKey] = hash
	}

	return annotations
}

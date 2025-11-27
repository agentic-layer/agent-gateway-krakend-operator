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
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
	"text/template"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	agentruntimev1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
)

const DefaultGatewayPort = 8080
const AgentGatewayKrakendControllerName = "runtime.agentic-layer.ai/agent-gateway-krakend-controller"
const Image = "ghcr.io/agentic-layer/agent-gateway-krakend:0.3.0"

// KrakendBackend represents a backend configuration in KrakenD
type KrakendBackend struct {
	// Host defines the agents host addresses
	Host []string `json:"host"`
	// URLPattern specifies the path the agent is called at
	URLPattern string `json:"url_pattern"`
}

// KrakendEndpoint represents an endpoint configuration in KrakenD
type KrakendEndpoint struct {
	// Endpoint defines the API endpoint path
	Endpoint string `json:"endpoint"`
	// OutputEncoding specifies the response encoding format
	OutputEncoding string `json:"output_encoding"`
	// Method defines the HTTP method for this endpoint
	Method string `json:"method"`
	// Backend contains the backend configuration for this endpoint
	Backend []KrakendBackend `json:"backend"`
}

// KrakendConfigData holds the data for template execution
type KrakendConfigData struct {
	// Port specifies the server port number
	Port int32
	// Timeout defines the request timeout duration
	Timeout string
	// PluginNames contains ordered list of plugin handler names
	PluginNames []string
	// Endpoints contains all configured API endpoints
	Endpoints []KrakendEndpoint
}

// AgentGatewayReconciler reconciles a AgentGateway object
type AgentGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=agents,verbs=get;list;watch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=agentgateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=agentgateways/status,verbs=get
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=agentgatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AgentGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AgentGateway instance
	var agentGateway agentruntimev1alpha1.AgentGateway
	if err := r.Get(ctx, req.NamespacedName, &agentGateway); err != nil {
		if errors.IsNotFound(err) {
			log.Info("AgentGateway resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get AgentGateway")
		return ctrl.Result{}, err
	}

	log.Info("Reconciling AgentGateway",
		"name", agentGateway.Name,
		"namespace", agentGateway.Namespace,
		"agentGatewayClass", agentGateway.Spec.AgentGatewayClassName)

	// Check if this controller is process for this AgentGateway
	process := r.shouldProcessAgentGateway(ctx, &agentGateway)

	if !process {
		log.Info("Controller is not responsible for this AgentGateway, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Get all exposed agents
	exposedAgents, err := r.getExposedAgents(ctx)
	if err != nil {
		log.Error(err, "Failed to get exposed agents")
		return ctrl.Result{}, err
	}

	// Create ConfigMap
	configMapName := agentGateway.Name + "-krakend-config"
	configHash, err := r.ensureConfigMap(ctx, &agentGateway, configMapName, exposedAgents)
	if err != nil {
		log.Error(err, "Failed to ensure ConfigMap for KrakenD")
		return ctrl.Result{}, err
	}

	// Create Deployment
	deploymentName := agentGateway.Name
	if err := r.ensureDeployment(ctx, &agentGateway, deploymentName, configMapName, configHash); err != nil {
		log.Error(err, "Failed to ensure Deployment for KrakenD")
		return ctrl.Result{}, err
	}

	// Create Service
	if err := r.ensureService(ctx, &agentGateway); err != nil {
		log.Error(err, "Failed to ensure Service for AgentGateway")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// shouldProcessAgentGateway determines if this controller is responsible for the given AgentGateway
func (r *AgentGatewayReconciler) shouldProcessAgentGateway(ctx context.Context, agentGateway *agentruntimev1alpha1.AgentGateway) bool {
	log := logf.FromContext(ctx)

	// If no className specified, check for default AgentGatewayClass
	var agentGatewayClassList agentruntimev1alpha1.AgentGatewayClassList
	if err := r.List(ctx, &agentGatewayClassList); err != nil {
		log.Info("If we can't list classes, don't process to avoid errors")
		return false
	}

	// Filter agentGatewayClassList to only contain classes with matching controller
	var krakendClasses []agentruntimev1alpha1.AgentGatewayClass
	for _, agentGatewayClass := range agentGatewayClassList.Items {
		if agentGatewayClass.Spec.Controller == AgentGatewayKrakendControllerName {
			krakendClasses = append(krakendClasses, agentGatewayClass)
		}
	}

	// If className is explicitly set, check if it matches any of our managed classes
	agentGatewayClassName := agentGateway.Spec.AgentGatewayClassName
	if agentGatewayClassName != "" {
		for _, krakendClass := range krakendClasses {
			if krakendClass.Name == agentGatewayClassName {
				return true
			}
		}
	}

	// Look for AgentGatewayClass with default annotation among filtered classes
	for _, krakendClass := range krakendClasses {
		if krakendClass.Annotations["agentgatewayclass.kubernetes.io/is-default-class"] == "true" {
			return true
		}
	}

	return false
}

// getExposedAgents queries all Agent resources across all namespaces with Exposed: true
func (r *AgentGatewayReconciler) getExposedAgents(ctx context.Context) ([]*agentruntimev1alpha1.Agent, error) {
	var agentList agentruntimev1alpha1.AgentList
	if err := r.List(ctx, &agentList); err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	var exposedAgents []*agentruntimev1alpha1.Agent
	for i := range agentList.Items {
		agent := &agentList.Items[i]
		if agent.Spec.Exposed {
			exposedAgents = append(exposedAgents, agent)
		}
	}

	return exposedAgents, nil
}

// ensureConfigMap creates or updates the ConfigMap with KrakenD configuration
func (r *AgentGatewayReconciler) ensureConfigMap(ctx context.Context, agentGateway *agentruntimev1alpha1.AgentGateway, configMapName string, exposedAgents []*agentruntimev1alpha1.Agent) (string, error) {
	log := logf.FromContext(ctx)

	// First, generate the desired ConfigMap configuration
	desiredConfigMap, configHash, err := r.createConfigMapForKrakend(ctx, agentGateway, configMapName, exposedAgents)
	if err != nil {
		return "", fmt.Errorf("failed to generate desired ConfigMap: %w", err)
	}

	// Try to get existing ConfigMap
	existingConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: agentGateway.Namespace}, existingConfigMap)

	if err != nil && errors.IsNotFound(err) {
		// ConfigMap doesn't exist, create it
		if err := r.Create(ctx, desiredConfigMap); err != nil {
			return "", fmt.Errorf("failed to create new ConfigMap %s: %w", configMapName, err)
		}
		log.Info("Successfully created ConfigMap", "name", configMapName, "namespace", agentGateway.Namespace)
	} else if err != nil {
		return "", fmt.Errorf("failed to get ConfigMap: %w", err)
	} else {
		// ConfigMap exists, check if update is needed
		if r.configMapNeedsUpdate(existingConfigMap, desiredConfigMap) {
			// Update the existing ConfigMap's data and labels
			existingConfigMap.Data = desiredConfigMap.Data
			// Only update our required labels, preserve others
			if existingConfigMap.Labels == nil {
				existingConfigMap.Labels = make(map[string]string)
			}
			for key, value := range desiredConfigMap.Labels {
				existingConfigMap.Labels[key] = value
			}

			if err := r.Update(ctx, existingConfigMap); err != nil {
				return "", fmt.Errorf("failed to update ConfigMap %s: %w", configMapName, err)
			}
			log.Info("Successfully updated ConfigMap", "name", configMapName, "namespace", agentGateway.Namespace)
		} else {
			log.V(1).Info("ConfigMap is up to date, no update needed", "name", configMapName, "namespace", agentGateway.Namespace)
		}
	}

	return configHash, nil
}

// ensureDeployment creates or updates the KrakenD deployment
func (r *AgentGatewayReconciler) ensureDeployment(ctx context.Context, agentGateway *agentruntimev1alpha1.AgentGateway, deploymentName string, configMapName string, configHash string) error {
	log := logf.FromContext(ctx)

	// First, generate the desired Deployment configuration
	desiredDeployment, err := r.createDeploymentForKrakend(agentGateway, deploymentName, configMapName, configHash)
	if err != nil {
		return fmt.Errorf("failed to generate desired Deployment: %w", err)
	}

	// Try to get existing Deployment
	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: agentGateway.Namespace}, existingDeployment)

	if err != nil && errors.IsNotFound(err) {
		// Deployment doesn't exist, create it
		if err := r.Create(ctx, desiredDeployment); err != nil {
			return fmt.Errorf("failed to create new Deployment %s: %w", deploymentName, err)
		}
		log.Info("Successfully created Deployment", "name", deploymentName, "namespace", agentGateway.Namespace)
	} else if err != nil {
		return fmt.Errorf("failed to get Deployment: %w", err)
	} else {
		// Deployment exists, check if update is needed
		if r.deploymentNeedsUpdate(existingDeployment, desiredDeployment) {
			// Update the existing Deployment's spec and labels
			existingDeployment.Spec.Replicas = desiredDeployment.Spec.Replicas
			existingDeployment.Spec.Template.Labels = desiredDeployment.Spec.Template.Labels
			existingDeployment.Spec.Template.Spec.Volumes = desiredDeployment.Spec.Template.Spec.Volumes

			// Update pod template annotations to trigger rolling restart on config changes
			if existingDeployment.Spec.Template.Annotations == nil {
				existingDeployment.Spec.Template.Annotations = make(map[string]string)
			}
			for key, value := range desiredDeployment.Spec.Template.Annotations {
				existingDeployment.Spec.Template.Annotations[key] = value
			}

			// Only update our required labels, preserve others
			if existingDeployment.Labels == nil {
				existingDeployment.Labels = make(map[string]string)
			}
			for key, value := range desiredDeployment.Labels {
				existingDeployment.Labels[key] = value
			}

			if err := r.Update(ctx, existingDeployment); err != nil {
				return fmt.Errorf("failed to update Deployment %s: %w", deploymentName, err)
			}
			log.Info("Successfully updated Deployment", "name", deploymentName, "namespace", agentGateway.Namespace)
		} else {
			log.V(1).Info("Deployment is up to date, no update needed", "name", deploymentName, "namespace", agentGateway.Namespace)
		}
	}

	return nil
}

// ensureService creates or updates the AgentGateway service
func (r *AgentGatewayReconciler) ensureService(ctx context.Context, agentGateway *agentruntimev1alpha1.AgentGateway) error {
	log := logf.FromContext(ctx)

	if agentGateway == nil {
		return fmt.Errorf("agentGateway cannot be nil")
	}

	serviceName := agentGateway.Name

	// First, generate the desired Service configuration
	desiredService, err := r.createServiceForAgentGateway(agentGateway, serviceName)
	if err != nil {
		return fmt.Errorf("failed to generate desired Service: %w", err)
	}

	// Try to get existing Service
	existingService := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: agentGateway.Namespace}, existingService)

	if err != nil && errors.IsNotFound(err) {
		// Service doesn't exist, create it
		if err := r.Create(ctx, desiredService); err != nil {
			return fmt.Errorf("failed to create new Service %s: %w", serviceName, err)
		}
		log.Info("Successfully created Service", "name", serviceName, "namespace", agentGateway.Namespace)
	} else if err != nil {
		return fmt.Errorf("failed to get Service: %w", err)
	} else {
		// Service exists, check if update is needed
		if r.serviceNeedsUpdate(existingService, desiredService) {
			// Update the existing Service's spec and labels
			existingService.Spec.Type = desiredService.Spec.Type
			existingService.Spec.Selector = desiredService.Spec.Selector
			existingService.Spec.Ports = desiredService.Spec.Ports
			// Only update our required labels, preserve others
			if existingService.Labels == nil {
				existingService.Labels = make(map[string]string)
			}
			for key, value := range desiredService.Labels {
				existingService.Labels[key] = value
			}

			if err := r.Update(ctx, existingService); err != nil {
				return fmt.Errorf("failed to update Service %s: %w", serviceName, err)
			}
			log.Info("Successfully updated Service", "name", serviceName, "namespace", agentGateway.Namespace)
		} else {
			log.V(1).Info("Service is up to date, no update needed", "name", serviceName, "namespace", agentGateway.Namespace)
		}
	}

	return nil
}

// createServiceForAgentGateway creates a service for the AgentGateway
func (r *AgentGatewayReconciler) createServiceForAgentGateway(agentGateway *agentruntimev1alpha1.AgentGateway, serviceName string) (*corev1.Service, error) {
	labels := map[string]string{
		"app": agentGateway.Name,
	}

	// Service selector should match deployment selector (stable labels only)
	selectorLabels := map[string]string{
		"app": agentGateway.Name,
	}

	// Create service port for the gateway (port 10000 named http)
	servicePort := corev1.ServicePort{
		Name:       "http",
		Port:       10000,
		TargetPort: intstr.FromInt32(DefaultGatewayPort), // Target the agent gateway container port
		Protocol:   corev1.ProtocolTCP,
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: agentGateway.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels,
			Ports:    []corev1.ServicePort{servicePort},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	// Set AgentGateway as the owner of the Service
	if err := ctrl.SetControllerReference(agentGateway, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

// createConfigMapForKrakend creates a ConfigMap with KrakenD configuration
func (r *AgentGatewayReconciler) createConfigMapForKrakend(ctx context.Context, agentGateway *agentruntimev1alpha1.AgentGateway, configMapName string, exposedAgents []*agentruntimev1alpha1.Agent) (*corev1.ConfigMap, string, error) {
	log := logf.FromContext(ctx)

	// Generate endpoints for all exposed agents
	var endpoints []KrakendEndpoint
	for _, agent := range exposedAgents {
		agentEndpoints, err := r.generateEndpointForAgent(ctx, agent)
		if err != nil {
			log.Error(err, "failed to generate endpoints for agent", "agent.name", agent.Name)
		} else {
			endpoints = append(endpoints, agentEndpoints...)
		}
	}

	templateData := KrakendConfigData{
		Port:        DefaultGatewayPort,
		Timeout:     agentGateway.Spec.Timeout.Duration.String(),
		PluginNames: []string{"agentcard-rw", "openai-a2a"}, // Order matters here
		Endpoints:   endpoints,
	}

	// Parse and execute the template
	tmpl, err := template.New("krakend").Parse(krakendConfigTemplate)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse KrakenD template: %w", err)
	}

	var configBuffer bytes.Buffer
	if err := tmpl.Execute(&configBuffer, templateData); err != nil {
		return nil, "", fmt.Errorf("failed to execute KrakenD template: %w", err)
	}
	krakendConfig := configBuffer.String()

	hash := sha256.Sum256(configBuffer.Bytes())
	configHash := fmt.Sprintf("%x", hash)[:16]

	labels := map[string]string{
		"app": agentGateway.Name,
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: agentGateway.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"krakend.json": krakendConfig,
		},
	}

	// Set AgentGateway as the owner of the ConfigMap
	if err := ctrl.SetControllerReference(agentGateway, configMap, r.Scheme); err != nil {
		return nil, "", err
	}

	return configMap, configHash, nil
}

// createDeploymentForKrakend creates a deployment for KrakenD
func (r *AgentGatewayReconciler) createDeploymentForKrakend(agentGateway *agentruntimev1alpha1.AgentGateway, deploymentName string, configMapName string, configHash string) (*appsv1.Deployment, error) {
	replicas := agentGateway.Spec.Replicas
	if replicas == nil {
		replicas = new(int32)
		*replicas = 1 // Default to 1 replica if not specified
	}

	labels := map[string]string{
		"app": agentGateway.Name,
	}

	// Selector labels should be stable (immutable in Kubernetes)
	selectorLabels := map[string]string{
		"app": agentGateway.Name,
	}

	// Create container port
	containerPort := corev1.ContainerPort{
		Name:          "http",
		ContainerPort: DefaultGatewayPort,
		Protocol:      corev1.ProtocolTCP,
	}

	// Create the krakend config volume
	configVolume := corev1.Volume{
		Name: "krakend-config-volume",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: agentGateway.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"runtime.agentic-layer.ai/config-hash": configHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "agent-gateway",
							Image: Image,
							Ports: []corev1.ContainerPort{containerPort},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("32Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("64Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      configVolume.Name,
									MountPath: "/etc/krakend",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						configVolume,
					},
				},
			},
		},
	}

	// Set AgentGateway as the owner of the Deployment
	if err := ctrl.SetControllerReference(agentGateway, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

// generateEndpointForAgent creates the endpoint JSON configuration for an agent based on its Status.Url
func (r *AgentGatewayReconciler) generateEndpointForAgent(ctx context.Context, agent *agentruntimev1alpha1.Agent) ([]KrakendEndpoint, error) {
	log := logf.FromContext(ctx)

	// If the agent doesn't have a URL set in status, log and return empty endpoints (not an error)
	if agent.Status.Url == "" {
		log.Info("Agent has no URL set in status, skipping endpoint generation", "agent.name", agent.Name, "agent.namespace", agent.Namespace)
		return []KrakendEndpoint{}, nil
	}

	// Pre-allocate slice with capacity for RPC + agent card endpoint
	endpoints := make([]KrakendEndpoint, 0, 2)

	// Parse the URL to separate host from path
	parsedURL, err := url.Parse(agent.Status.Url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL for agent %s: %w", agent.Name, err)
	}

	// Construct the host (scheme + host + port)
	hostURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)

	// Create agent card endpoint
	endpoints = append(endpoints, KrakendEndpoint{
		Endpoint:       fmt.Sprintf("/%s%s", agent.Name, parsedURL.Path),
		OutputEncoding: "no-op",
		Method:         "GET",
		Backend: []KrakendBackend{
			{
				Host:       []string{hostURL},
				URLPattern: parsedURL.Path,
			},
		},
	})

	// Remove /.well-known/agent-card.json from the path to get the RPC URL
	urlPattern := strings.TrimSuffix(parsedURL.Path, "/.well-known/agent-card.json")

	// Create A2A endpoint for the agent (for both service and external URLs)
	endpoints = append(endpoints, KrakendEndpoint{
		Endpoint:       fmt.Sprintf("/%s", agent.Name),
		OutputEncoding: "no-op",
		Method:         "POST", // A2A protocol uses POST
		Backend: []KrakendBackend{
			{
				Host:       []string{hostURL},
				URLPattern: urlPattern,
			},
		},
	})

	return endpoints, nil
}

// configMapNeedsUpdate compares existing and desired ConfigMaps to determine if an update is needed
func (r *AgentGatewayReconciler) configMapNeedsUpdate(existing, desired *corev1.ConfigMap) bool {
	// Compare data content
	if len(existing.Data) != len(desired.Data) {
		return true
	}

	for key, desiredValue := range desired.Data {
		if existingValue, exists := existing.Data[key]; !exists || existingValue != desiredValue {
			return true
		}
	}

	// Compare labels (optional but good practice for consistency)
	if len(existing.Labels) != len(desired.Labels) {
		return true
	}

	for key, desiredValue := range desired.Labels {
		if existingValue, exists := existing.Labels[key]; !exists || existingValue != desiredValue {
			return true
		}
	}

	return false
}

// deploymentNeedsUpdate compares existing and desired Deployments to determine if an update is needed
func (r *AgentGatewayReconciler) deploymentNeedsUpdate(existing, desired *appsv1.Deployment) bool {
	// Compare replica count
	existingReplicas := int32(1) // Default replica count
	if existing.Spec.Replicas != nil {
		existingReplicas = *existing.Spec.Replicas
	}

	desiredReplicas := int32(1) // Default replica count
	if desired.Spec.Replicas != nil {
		desiredReplicas = *desired.Spec.Replicas
	}

	if existingReplicas != desiredReplicas {
		return true
	}

	// Compare labels
	if len(existing.Labels) != len(desired.Labels) {
		return true
	}

	for key, desiredValue := range desired.Labels {
		if existingValue, exists := existing.Labels[key]; !exists || existingValue != desiredValue {
			return true
		}
	}

	// Compare template labels (pod labels)
	if len(existing.Spec.Template.Labels) != len(desired.Spec.Template.Labels) {
		return true
	}

	for key, desiredValue := range desired.Spec.Template.Labels {
		if existingValue, exists := existing.Spec.Template.Labels[key]; !exists || existingValue != desiredValue {
			return true
		}
	}

	if existing.Spec.Template.Annotations["runtime.agentic-layer.ai/config-hash"] != desired.Spec.Template.Annotations["runtime.agentic-layer.ai/config-hash"] {
		return true
	}

	// Compare ConfigMap volume reference (in case the ConfigMap name changes)
	existingConfigMapName := r.getConfigMapNameFromVolumes(existing.Spec.Template.Spec.Volumes)
	desiredConfigMapName := r.getConfigMapNameFromVolumes(desired.Spec.Template.Spec.Volumes)

	return existingConfigMapName != desiredConfigMapName
}

// serviceNeedsUpdate compares existing and desired Services to determine if an update is needed
func (r *AgentGatewayReconciler) serviceNeedsUpdate(existing, desired *corev1.Service) bool {
	// Compare labels
	if len(existing.Labels) != len(desired.Labels) {
		return true
	}

	for key, desiredValue := range desired.Labels {
		if existingValue, exists := existing.Labels[key]; !exists || existingValue != desiredValue {
			return true
		}
	}

	// Compare service type
	if existing.Spec.Type != desired.Spec.Type {
		return true
	}

	// Compare selector
	if len(existing.Spec.Selector) != len(desired.Spec.Selector) {
		return true
	}

	for key, desiredValue := range desired.Spec.Selector {
		if existingValue, exists := existing.Spec.Selector[key]; !exists || existingValue != desiredValue {
			return true
		}
	}

	// Compare ports
	if len(existing.Spec.Ports) != len(desired.Spec.Ports) {
		return true
	}

	for i, desiredPort := range desired.Spec.Ports {
		if i >= len(existing.Spec.Ports) {
			return true
		}
		existingPort := existing.Spec.Ports[i]

		if existingPort.Name != desiredPort.Name ||
			existingPort.Port != desiredPort.Port ||
			existingPort.TargetPort != desiredPort.TargetPort ||
			existingPort.Protocol != desiredPort.Protocol {
			return true
		}
	}

	return false
}

// getConfigMapNameFromVolumes extracts the ConfigMap name from the volumes
func (r *AgentGatewayReconciler) getConfigMapNameFromVolumes(volumes []corev1.Volume) string {
	for _, volume := range volumes {
		if volume.ConfigMap != nil {
			return volume.ConfigMap.Name
		}
	}
	return ""
}

// findAgentGatewaysForAgent returns all AgentGateway resources that need to be reconciled
// when an Agent changes. Since all gateways discover agents across all namespaces,
// any Agent change affects all AgentGateway resources.
func (r *AgentGatewayReconciler) findAgentGatewaysForAgent(ctx context.Context, _ client.Object) []ctrl.Request {
	var agentGatewayList agentruntimev1alpha1.AgentGatewayList
	if err := r.List(ctx, &agentGatewayList); err != nil {
		return []ctrl.Request{}
	}

	requests := make([]ctrl.Request, len(agentGatewayList.Items))
	for i, gw := range agentGatewayList.Items {
		requests[i] = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      gw.Name,
				Namespace: gw.Namespace,
			},
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentruntimev1alpha1.AgentGateway{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Watches(
			&agentruntimev1alpha1.Agent{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentGatewaysForAgent),
		).
		Named(AgentGatewayKrakendControllerName).
		Complete(r)
}

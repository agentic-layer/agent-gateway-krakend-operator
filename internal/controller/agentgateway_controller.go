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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	kevents "k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/agentic-layer/agent-gateway-krakend-operator/internal/controller/utils"
	agentruntimev1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
)

const DefaultGatewayPort = 8080
const AgentGatewayKrakendControllerName = "runtime.agentic-layer.ai/agent-gateway-krakend-controller"
const Image = "ghcr.io/agentic-layer/agent-gateway-krakend:0.6.0"

// Version set at build time using ldflags
var Version = "dev"

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

// AgentConfigInfo holds agent information for the KrakenD config template
type AgentConfigInfo struct {
	// ModelID is the unique identifier for the model (format: namespace/name)
	ModelID string
	// URL is the agent backend URL
	URL string
	// OwnedBy indicates the owner/namespace of the model
	OwnedBy string
	// CreatedAt is the Unix timestamp of agent creation
	CreatedAt int64
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
	// Agents contains all exposed agents for the openai-a2a plugin config
	Agents []AgentConfigInfo
	// ServiceVersion is the Docker image path used to identify the service version
	ServiceVersion string
	// DeploymentName is the name of the deployment
	DeploymentName string
	// OtelCollectorHost is the hostname of the OpenTelemetry collector
	OtelCollectorHost string
	// OtelCollectorPort is the port of the OpenTelemetry collector
	OtelCollectorPort string
}

// AgentGatewayReconciler reconciles a AgentGateway object
type AgentGatewayReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder kevents.EventRecorder
}

// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=agents,verbs=get;list;watch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=agentgateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=agentgateways/status,verbs=get
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=agentgatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AgentGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AgentGateway instance
	var agentGateway agentruntimev1alpha1.AgentGateway
	if err := r.Get(ctx, req.NamespacedName, &agentGateway); err != nil {
		if apierrors.IsNotFound(err) {
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

	// Create ConfigMap with retry on conflict
	configMapName := agentGateway.Name + "-krakend-config"
	var configHash string
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var err error
		configHash, err = r.ensureConfigMap(ctx, &agentGateway, configMapName, exposedAgents)
		return err
	}); err != nil {
		log.Error(err, "Failed to ensure ConfigMap for KrakenD")
		return ctrl.Result{}, err
	}

	// Create Deployment with retry on conflict
	deploymentName := agentGateway.Name
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.ensureDeployment(ctx, &agentGateway, deploymentName, configMapName, configHash)
	}); err != nil {
		log.Error(err, "Failed to ensure Deployment for KrakenD")
		return ctrl.Result{}, err
	}

	// Create Service with retry on conflict
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.ensureService(ctx, &agentGateway)
	}); err != nil {
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

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: agentGateway.Namespace,
		},
	}

	var configHash string
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		// Generate the KrakenD configuration
		krakendConfig, hash, err := r.generateKrakendConfig(ctx, agentGateway, exposedAgents)
		if err != nil {
			return fmt.Errorf("failed to generate KrakenD config: %w", err)
		}
		configHash = hash

		// Set ConfigMap data
		configMap.Data = map[string]string{
			"krakend.json": krakendConfig,
		}

		// Set labels
		if configMap.Labels == nil {
			configMap.Labels = make(map[string]string)
		}
		configMap.Labels["app"] = agentGateway.Name

		// Set controller reference
		return ctrl.SetControllerReference(agentGateway, configMap, r.Scheme)
	})

	if err != nil {
		return "", fmt.Errorf("failed to create or update ConfigMap %s: %w", configMapName, err)
	}

	log.Info("Successfully ensured ConfigMap", "name", configMapName, "namespace", agentGateway.Namespace)
	return configHash, nil
}

// ensureDeployment creates or updates the KrakenD deployment.
// It configures the deployment with the specified replicas, labels, volumes, and containers.
// The deployment uses the provided configMapName for KrakenD configuration and includes
// a config hash annotation to trigger pod restarts when configuration changes.
func (r *AgentGatewayReconciler) ensureDeployment(ctx context.Context, agentGateway *agentruntimev1alpha1.AgentGateway, deploymentName string, configMapName string, configHash string) error {
	log := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: agentGateway.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Set replicas
		replicas := agentGateway.Spec.Replicas
		if replicas == nil {
			replicas = new(int32)
			*replicas = 1
		}
		deployment.Spec.Replicas = replicas

		// Set labels
		labels := map[string]string{"app": agentGateway.Name}
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		for key, value := range labels {
			deployment.Labels[key] = value
		}

		// CRITICAL: Only set immutable selector on first creation
		if deployment.CreationTimestamp.IsZero() {
			deployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: labels,
			}
		}

		// Set pod template metadata
		deployment.Spec.Template.Labels = labels
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		deployment.Spec.Template.Annotations["runtime.agentic-layer.ai/config-hash"] = configHash

		// Generate and add annotations for referenced ConfigMaps and Secrets
		// Note: We generate annotations based on the spec, not the final container state
		// This ensures we track all resources that will be referenced
		containers := []corev1.Container{{
			Env:     agentGateway.Spec.Env,
			EnvFrom: agentGateway.Spec.EnvFrom,
		}}
		resourceAnnotations := r.generateResourceAnnotations(ctx, containers, agentGateway.Namespace)
		for k, v := range resourceAnnotations {
			deployment.Spec.Template.Annotations[k] = v
		}

		// Configure volume
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
		deployment.Spec.Template.Spec.Volumes = []corev1.Volume{configVolume}

		// Find or create the agent-gateway container
		container := utils.FindContainerByName(&deployment.Spec.Template.Spec, "agent-gateway")
		if container == nil {
			// Container doesn't exist, create minimal container and append it
			newContainer := corev1.Container{
				Name: "agent-gateway",
			}
			deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, newContainer)
			// Get pointer to the newly added container
			container = &deployment.Spec.Template.Spec.Containers[len(deployment.Spec.Template.Spec.Containers)-1]
		}

		// Update only the managed fields (preserves fields set by other controllers)
		container.Image = Image
		container.Ports = []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: DefaultGatewayPort,
				Protocol:      corev1.ProtocolTCP,
			},
		}
		container.Env = append([]corev1.EnvVar{
			{
				Name:  "FC_ENABLE",
				Value: "1",
			},
		}, agentGateway.Spec.Env...)
		container.EnvFrom = agentGateway.Spec.EnvFrom
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("32Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("64Mi"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		}
		container.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      configVolume.Name,
				MountPath: "/etc/krakend",
			},
		}
		container.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/__health",
					Port:   intstr.FromInt32(DefaultGatewayPort),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		}
		container.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/__health",
					Port:   intstr.FromInt32(DefaultGatewayPort),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
			TimeoutSeconds:      3,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		}

		// Set controller reference
		return ctrl.SetControllerReference(agentGateway, deployment, r.Scheme)
	})

	if err != nil {
		return fmt.Errorf("failed to create or update Deployment %s: %w", deploymentName, err)
	}

	log.Info("Successfully ensured Deployment", "name", deploymentName, "namespace", agentGateway.Namespace)
	return nil
}

// ensureService creates or updates the AgentGateway service
func (r *AgentGatewayReconciler) ensureService(ctx context.Context, agentGateway *agentruntimev1alpha1.AgentGateway) error {
	log := logf.FromContext(ctx)

	if agentGateway == nil {
		return fmt.Errorf("agentGateway cannot be nil")
	}

	serviceName := agentGateway.Name

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: agentGateway.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		labels := map[string]string{
			"app": agentGateway.Name,
		}

		selectorLabels := map[string]string{
			"app": agentGateway.Name,
		}

		// Set labels
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		for key, value := range labels {
			service.Labels[key] = value
		}

		// Configure service spec
		service.Spec.Type = corev1.ServiceTypeClusterIP
		service.Spec.Selector = selectorLabels
		service.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt32(DefaultGatewayPort),
				Protocol:   corev1.ProtocolTCP,
			},
		}

		// Set controller reference
		return ctrl.SetControllerReference(agentGateway, service, r.Scheme)
	})

	if err != nil {
		return fmt.Errorf("failed to create or update Service %s: %w", serviceName, err)
	}

	log.Info("Successfully ensured Service", "name", serviceName, "namespace", agentGateway.Namespace)
	return nil
}

// generateKrakendConfig generates the KrakenD configuration and returns the config string and hash
func (r *AgentGatewayReconciler) generateKrakendConfig(ctx context.Context, agentGateway *agentruntimev1alpha1.AgentGateway, exposedAgents []*agentruntimev1alpha1.Agent) (string, string, error) {
	log := logf.FromContext(ctx)

	// Generate endpoints for all exposed agents
	var endpoints []KrakendEndpoint

	// Build agent config info for the openai-a2a plugin
	agents := make([]AgentConfigInfo, 0, len(exposedAgents))

	// Track agent names to detect conflicts for shorthand endpoints
	agentNameCounts := make(map[string]int)
	for _, agent := range exposedAgents {
		agentNameCounts[agent.Name]++
	}

	// Generate endpoints for all agents
	shorthandCount := 0
	for _, agent := range exposedAgents {
		// Skip agents without URLs
		if agent.Status.Url == "" {
			log.Info("Agent has no URL set in status, skipping endpoint generation", "agent.name", agent.Name, "agent.namespace", agent.Namespace)
			continue
		}

		// Parse URL to get backend configuration
		parsedURL, err := url.Parse(agent.Status.Url)
		if err != nil {
			log.Error(err, "failed to parse URL for agent", "agent.name", agent.Name)
			continue
		}

		// Always create namespaced endpoints
		namespacedPath := fmt.Sprintf("%s/%s", agent.Namespace, agent.Name)
		endpoints = append(endpoints, r.generateAgentEndpoints(namespacedPath, parsedURL)...)

		// Create shorthand endpoints if agent name is unique
		if agentNameCounts[agent.Name] == 1 {
			endpoints = append(endpoints, r.generateAgentEndpoints(agent.Name, parsedURL)...)
			shorthandCount++
		}

		// Add agent to config for the openai-a2a plugin
		agents = append(agents, AgentConfigInfo{
			ModelID:   fmt.Sprintf("%s/%s", agent.Namespace, agent.Name),
			URL:       agent.Status.Url,
			OwnedBy:   agent.Namespace,
			CreatedAt: agent.CreationTimestamp.Unix(),
		})
	}

	log.Info("Generated config with agents", "count", len(agents), "shorthand_endpoints", shorthandCount)

	templateData := KrakendConfigData{
		Port:           DefaultGatewayPort,
		Timeout:        agentGateway.Spec.Timeout.Duration.String(),
		PluginNames:    []string{"body-logger", "agentcard-rw", "openai-a2a"}, // Order matters here
		Endpoints:      endpoints,
		Agents:         agents,
		ServiceVersion: Version,
		DeploymentName: agentGateway.Name,
		// double templating
		// 1) go processing adds escaped code to config
		// 2) krakenD config processing splits the env var into host and port
		OtelCollectorHost: `{{ (split ":" (splitList "://" (env "OTEL_EXPORTER_OTLP_ENDPOINT") | last))._0 }}`,
		OtelCollectorPort: `{{ int ((split ":" (splitList "://" (env "OTEL_EXPORTER_OTLP_ENDPOINT") | last))._1) }}`,
	}

	// Parse and execute the template
	tmpl, err := template.New("krakend").Parse(krakendConfigTemplate)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse KrakenD template: %w", err)
	}

	var configBuffer bytes.Buffer
	if err := tmpl.Execute(&configBuffer, templateData); err != nil {
		return "", "", fmt.Errorf("failed to execute KrakenD template: %w", err)
	}
	krakendConfig := configBuffer.String()

	hash := sha256.Sum256(configBuffer.Bytes())
	configHash := fmt.Sprintf("%x", hash)[:16]

	return krakendConfig, configHash, nil
}

// generateAgentEndpoints creates Agent Card and A2A endpoint configurations for a given path
func (r *AgentGatewayReconciler) generateAgentEndpoints(path string, parsedURL *url.URL) []KrakendEndpoint {
	// Construct the host (scheme + host + port)
	hostURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)

	// Remove /.well-known/agent-card.json from the path to get the A2A URL pattern
	urlPattern := strings.TrimSuffix(parsedURL.Path, "/.well-known/agent-card.json")

	return []KrakendEndpoint{
		// 1. Agent card endpoint
		{
			Endpoint:       fmt.Sprintf("/%s%s", path, parsedURL.Path),
			OutputEncoding: "no-op",
			Method:         "GET",
			Backend: []KrakendBackend{
				{
					Host:       []string{hostURL},
					URLPattern: parsedURL.Path,
				},
			},
		},
		// 2. A2A endpoint
		{
			Endpoint:       fmt.Sprintf("/%s", path),
			OutputEncoding: "no-op",
			Method:         "POST",
			Backend: []KrakendBackend{
				{
					Host:       []string{hostURL},
					URLPattern: urlPattern,
				},
			},
		},
	}
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

// findAgentGatewaysForConfigMapOrSecret returns all AgentGateway resources that need to be reconciled
// when a ConfigMap or Secret changes. We need to reconcile all gateways since we don't know
// which gateways reference which ConfigMaps/Secrets without reading each gateway's spec.
func (r *AgentGatewayReconciler) findAgentGatewaysForConfigMapOrSecret(ctx context.Context, obj client.Object) []ctrl.Request {
	log := logf.FromContext(ctx)

	var agentGatewayList agentruntimev1alpha1.AgentGatewayList
	if err := r.List(ctx, &agentGatewayList); err != nil {
		log.Error(err, "Failed to list AgentGateways for ConfigMap/Secret change")
		return []ctrl.Request{}
	}

	// Filter to only gateways in the same namespace as the ConfigMap/Secret
	requests := make([]ctrl.Request, 0)
	for _, gw := range agentGatewayList.Items {
		if gw.Namespace == obj.GetNamespace() {
			requests = append(requests, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      gw.Name,
					Namespace: gw.Namespace,
				},
			})
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
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentGatewaysForConfigMapOrSecret),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentGatewaysForConfigMapOrSecret),
		).
		Named(AgentGatewayKrakendControllerName).
		Complete(r)
}

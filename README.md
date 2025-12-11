# Agent Gateway KrakenD Operator

The Agent Gateway KrakenD Operator is a specialized Kubernetes operator that provides a KrakenD-based Agent gateway implementation for the Agent Runtime Operator ecosystem. It manages `AgentGateway` resources and automatically generates KrakenD configurations to expose Agent workloads through a unified gateway interface.

This operator works as a companion to the [Agent Runtime Operator](https://github.com/agentic-layer/agent-runtime-operator) and implements the `AgentGatewayClass` controller pattern to provide centralized gateway management for agentic workloads.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [Configuration](#configuration)
- [End-to-End (E2E) Testing](#end-to-end-e2e-testing)
- [Testing Tools and Configuration](#testing-tools-and-configuration)
- [Sample Data](#sample-data)
- [Contribution](#contribution)

----

## Prerequisites

**IMPORTANT: The Agent Runtime Operator must be installed before deploying this operator.**

This operator depends on the Agent Runtime Operator to provide `Agent`, `AgentGateway`, and `AgentGatewayClass` Custom Resource Definitions (CRDs). Please install and configure the Agent Runtime Operator first by following the instructions in the [Agent Runtime Operator repository](https://github.com/agentic-layer/agent-runtime-operator).

Before working with this project, ensure you have the following tools installed on your system:

* **Go**: version 1.24.0 or higher
* **Docker**: version 20.10+ (or a compatible alternative like Podman)
* **kubectl**: The Kubernetes command-line tool
* **kind**: For running Kubernetes locally in Docker
* **make**: The build automation tool

----

## Getting Started

📖 **For detailed setup instructions**, see our [Getting Started guide](https://docs.agentic-layer.ai/agent-gateway-krakend-operator/) in the documentation.

**Quick Start:**

> **Note:** This operator requires the [Agent Runtime Operator](https://github.com/agentic-layer/agent-runtime-operator) to be installed first, as it provides the required CRDs (`AgentGateway` and `AgentGatewayClass`).

```shell
# Create local cluster and install cert-manager
kind create cluster
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml

# Install the Agent Runtime Operator (provides CRDs)
kubectl apply -f https://github.com/agentic-layer/agent-runtime-operator/releases/download/v0.13.0/install.yaml

# Install the Agent Gateway operator
kubectl apply -f  https://github.com/agentic-layer/agent-gateway-krakend-operator/releases/download/v0.3.0/install.yaml
```

## Development

Follow the prerequisites above to set up your local environment.
Then follow these steps to build and deploy the operator locally:

```shell
# Install CRDs into the cluster
make install
# Build docker image
make docker-build
# Load image into kind cluster (not needed if using local registry)
make kind-load
# Deploy the operator to the cluster
make deploy
```

After a successful start, you should see the controller manager pod running in the `agent-gateway-krakend-operator-system` namespace.

```bash
kubectl get pods -n agent-gateway-krakend-operator-system
```

## Configuration

### AgentGateway Configuration

To create a KrakenD-based gateway for your agents, define an `AgentGateway` resource:

```yaml
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: AgentGateway
metadata:
  labels:
    app.kubernetes.io/name: agent-gateway-krakend-operator
    app.kubernetes.io/managed-by: kustomize
  name: my-agent-gateway
spec:
  agentGatewayClassName: krakend  # Optional: specify controller responsibility
  replicas: 2  # Number of gateway replicas (optional, default: 1)
  timeout: "30000ms"  # Request timeout (optional, default: 60000ms)
```

### Agent Integration

The gateway automatically discovers and configures endpoints for `Agent` resources that have `exposed: true`:

```yaml
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: Agent
metadata:
  name: weather-agent
spec:
  framework: google-adk
  image: eu.gcr.io/agentic-layer/weather-agent:0.3.0
  exposed: true  # This agent will be included in the gateway configuration
  protocols:
    - type: A2A
```

The operator will:
1. Discover all exposed agents across all namespaces
2. Generate KrakenD endpoint configurations for each agent
3. Create a ConfigMap with the complete KrakenD configuration
4. Deploy a KrakenD deployment that serves the unified gateway
5. Create a ServiceAccount for the gateway deployment

**Endpoint Routing:**

Each exposed agent gets endpoints in two formats:
- **Namespaced** (always): `/{namespace}/{agent-name}` and `/{namespace}/{agent-name}/.well-known/agent-card.json`
- **Shorthand** (when unique): `/{agent-name}` and `/{agent-name}/.well-known/agent-card.json`

Shorthand endpoints are only created when the agent name is unique across all namespaces. This prevents conflicts while providing convenience for uniquely-named agents.

The gateway also provides OpenAI-compatible endpoints:
- `GET /models` - List all exposed agents
- `POST /chat/completions` - Chat with agents using the `model` parameter for routing

### Default AgentGatewayClass

The operator installs a `AgentGatewayClass` resource:

```yaml
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: AgentGatewayClass
metadata:
  name: krakend
spec:
  controller: runtime.agentic-layer.ai/agent-gateway-controller
```

## End-to-End (E2E) Testing

### Prerequisites for E2E Tests

- **kind** must be installed and available in PATH
- **Docker** running and accessible
- **kubectl** configured and working

### Running E2E Tests

The E2E tests automatically create an isolated Kind cluster, deploy the operator, run comprehensive tests, and clean up afterwards.

```bash
# Run complete E2E test suite
make test-e2e
```

The E2E test suite includes:
- Operator deployment verification
- CRD installation testing
- Webhook functionality testing
- Certificate management verification

### Manual E2E Test Setup

If you need to run E2E tests manually or inspect the test environment:

```bash
# Set up test cluster (will create 'ai-gateway-litellm-test-e2e' cluster)
make setup-test-e2e
```
```bash
# Run E2E tests against the existing cluster
KIND_CLUSTER=ai-gateway-litellm-test-e2e go test ./test/e2e/ -v -ginkgo.v
```
```bash
# Clean up test cluster when done
make cleanup-test-e2e
```

### E2E Test Coverage

**E2E Test Features:**
- OpenAI API `/models` and `/chat/completions` endpoints
- Unique agent routing
- Namespace conflict resolution
- Agent gateway deployment verification
- KrakenD configuration generation

## Testing Tools and Configuration

The project includes comprehensive test coverage:

- **Unit Tests**: Complete test suite for the controller with AgentGatewayClass responsibility checks
- **Integration Tests**: Tests for Agent discovery and KrakenD configuration generation
- **E2E Tests**: End-to-end tests for OpenAI API flows and namespace conflict resolution
- **Ginkgo/Gomega**: BDD-style testing framework
- **EnvTest**: Kubernetes API server testing environment

Run tests with:

```bash
# Run unit and integration tests
make test

# Run E2E tests in kind cluster
make test-e2e
```

## Sample Data

The project includes sample manifests to help you get started.

  * **Where to find sample data?**
    Sample manifests are located in the `config/samples/` directory.

  * **How to deploy sample resources?**
    You can deploy sample AgentGateway resources with:

    ```bash
    kubectl apply -k config/samples/
    ```

  * **How to verify the sample gateway?**
    After applying the sample, check the created resources:

    ```bash
    # Check the gateway status
    kubectl get agentgateways -o yaml

    # Check the KrakenD deployment
    kubectl get deployments -l provider=krakend

    # Check the generated configuration
    kubectl get configmaps -l provider=krakend
    ```



## Contribution

See [Contribution Guide](https://github.com/agentic-layer/agent-runtime-krakend-operator?tab=contributing-ov-file) for details on contribution, and the process for submitting pull requests.

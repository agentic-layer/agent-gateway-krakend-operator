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
- [Contributing](#contributing)
- [Project Distribution](#project-distribution)

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
  * **Git**: For version control
  * **Agent Runtime Operator**: Must be installed and running in the cluster

----

## Getting Started

Follow these steps to get the operator up and running on a local Kubernetes cluster.

**Important:** These instructions assume you have already installed and configured the [Agent Runtime Operator](https://github.com/agentic-layer/agent-runtime-operator).

1.  **Clone the repository:**

    ```bash
    git clone https://github.com/agentic-layer/agent-gateway-krakend-operator
    cd agent-gateway-krakend-operator
    ```

2.  **Verify Agent Runtime Operator is installed:**
    Ensure the Agent Runtime Operator is running and the required CRDs are available:

    ```bash
    kubectl get crd | grep runtime.agentic-layer.ai
    kubectl get pods -n agent-runtime-operator-system
    ```

3.  **Build and deploy the operator:**
    These commands will build the operator's container image, load it into your cluster, and deploy it.

    ```bash
    make docker-build
    make kind-load
    make deploy
    ```

4.  **Verify the deployment:**
    After a successful start, you should see the controller manager pod running in the `agent-gateway-krakend-operator-system` namespace.

    ```bash
    kubectl get pods -n agent-gateway-krakend-operator-system
    ```

5.  **Create the default AgentGatewayClass:**
    The operator includes a default `AgentGatewayClass` that will be created automatically during installation. Verify it exists:

    ```bash
    kubectl get agentgatewayclasses
    ```

## Configuration

### Environment Variables

The operator can be configured using the following environment variables:

- `ENABLE_WEBHOOKS` - Set to `false` to disable admission webhooks (default: `true`)
- `METRICS_BIND_ADDRESS` - Address for metrics server (default: `:8443`)
- `HEALTH_PROBE_BIND_ADDRESS` - Address for health probes (default: `:8081`)

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
  cacheTTL: "300s"  # Cache TTL (optional, default: 300s)
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
- **Agent Runtime Operator** must be installed in the test environment

### Running E2E Tests

```bash
# Run complete E2E test suite
make test-e2e
```

### Manual Testing

```bash
# Set up test cluster
make setup-test-e2e

# Run unit tests with comprehensive coverage
make test

# Clean up test cluster
make cleanup-test-e2e
```

## Testing Tools and Configuration

The project includes comprehensive test coverage:

- **Unit Tests**: Complete test suite for the controller with AgentGatewayClass responsibility checks
- **Integration Tests**: Tests for Agent discovery and KrakenD configuration generation
- **Ginkgo/Gomega**: BDD-style testing framework
- **EnvTest**: Kubernetes API server testing environment

Run tests with:

```bash
make test
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

## Contributing

We welcome contributions to the Agent Gateway KrakenD Operator! Please follow these guidelines:

### Setup for Contributors

1. **Fork and clone the repository**
2. **Install the Agent Runtime Operator** in your development environment
3. **Install pre-commit hooks** (if available):
   ```bash
   # Install hooks for this repository
   pre-commit install
   ```

4. **Verify your development environment**:
   ```bash
   # Run all checks
   make fmt vet lint test
   ```

### Code Style and Standards

- **Go Style**: We follow standard Go conventions and use `gofmt` for formatting
- **Linting**: Code must pass golangci-lint checks (see `.golangci.yml`)
- **Testing**: All new features must include appropriate unit tests
- **Documentation**: Update relevant documentation for new features

### Development Workflow

1. **Create a feature branch** from `main`:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes** following the code style guidelines

3. **Run development checks**:
   ```bash
   # Format code
   make fmt

   # Run static analysis
   make vet

   # Run linting
   make lint

   # Run unit tests
   make test

   # Generate updated manifests if needed
   make manifests generate
   ```

4. **Test your changes**:
   ```bash
   # Run E2E tests
   make test-e2e
   ```

5. **Commit your changes** with a descriptive commit message

6. **Submit a pull request** with:
   - Clear description of the changes
   - Reference to any related issues
   - Test results and verification steps

## Project Distribution

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```bash
make build-installer IMG=<some-registry>/agent-gateway-krakend-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project.

2. **Using the installer**

Users can install the project with:

```bash
kubectl apply -f https://raw.githubusercontent.com/agentic-layer/agent-gateway-krakend-operator/<tag or branch>/dist/install.yaml
```

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

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
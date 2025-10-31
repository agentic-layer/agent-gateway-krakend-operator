# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Project Overview and Developer Documentation
- @README.md

User Guides and How-To Guides
- @docs/modules/operator/partials/how-to-guide.adoc
- @docs/modules/gateway/partials/how-to-guide.adoc

Reference Documentation
- Overall Agentic Layer Architecture: https://docs.agentic-layer.ai/architecture/main/index.html

Documentation in AsciiDoc format is located in the `docs/` directory.
This folder is hosted as a separate [documentation site](https://docs.agentic-layer.ai/agent-gateway-krakend-operator/index.html).

## Architecture

### Core Components

- **AgentGateway CRD** (provided by Agent Runtime Operator): Defines the AgentGateway custom resource with:
  - Gateway provider abstraction (KrakenD, Envoy, Nginx)
  - Replica configuration for high availability
  - Timeout and caching configuration
  - AgentGatewayClass reference for controller selection

- **AgentGatewayClass CRD** (provided by Agent Runtime Operator): Defines the controller responsibility pattern:
  - Controller specification for gateway implementations
  - Default class annotation support
  - Used to determine which operator manages which AgentGateway resources

- **Agent CRD** (provided by Agent Runtime Operator): Defines the Agent custom resource with:
  - Framework specification (google-adk, custom)
  - Container image and replica configuration
  - Protocol definitions (A2A, OpenAI)
  - `exposed` flag to indicate gateway inclusion

- **AgentGateway Controller** (`internal/controller/agentgateway_controller.go`): Reconciles AgentGateway resources by:
  - **Controller responsibility checking**: Validates AgentGatewayClass ownership before processing
    - Checks if className matches this controller's managed classes
    - Falls back to default class if no className specified
    - Only processes gateways when responsible
  - **Agent discovery**: Queries all Agent resources with `exposed: true` across namespaces
  - **KrakenD configuration generation**: Creates endpoint configurations for each agent protocol
  - **ConfigMap management**: Generates and maintains KrakenD configuration with hash-based updates
  - **Deployment management**: Creates and updates KrakenD deployments with proper volume mounts
  - **Service management**: Exposes gateway through Kubernetes Services on port 10000
  - **Protocol-aware routing**: Generates appropriate endpoints based on agent protocols (A2A, OpenAI)

- **KrakenD Configuration Template** (`internal/controller/template.go`): Provides:
  - JSON-based gateway configuration template
  - Plugin integration support (OpenAI A2A plugin)
  - Dynamic endpoint generation from agent specifications
  - Backend URL routing to agent services

### Project Structure

```
├── cmd/main.go           # Operator entry point
├── config/               # Kubernetes manifests and Kustomize configs
│   ├── crd/              # AgentGatewayClass CRD (from Agent Runtime Operator)
│   ├── rbac/             # Role-based access control
│   ├── manager/          # Operator deployment
│   └── samples/          # Example AgentGateway resources
├── docs/                 # AsciiDoc documentation
├── internal/
│   └── controller/       # Reconciliation logic
│       ├── agentgateway_controller.go  # Main controller logic
│       └── template.go                  # KrakenD config template
└── test/
    ├── e2e/              # End-to-end tests
    └── utils/            # Test utilities
```

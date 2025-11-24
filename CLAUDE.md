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
This folder is hosted as a separate [documentation site](https://docs.agentic-layer.ai/agent-runtime-operator/index.html).

### Project Structure

```
├── cmd/main.go           # Operator entry point
├── config/               # Kubernetes manifests and Kustomize configs
│   ├── crd/              # Custom Resource Definitions
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

# Agent Gateway KrakenD Operator

The Agent Gateway KrakenD Operator is a Kubernetes operator that provides a KrakenD-based `AgentGateway` implementation for the [Agent Runtime Operator](https://github.com/agentic-layer/agent-runtime-operator) ecosystem. It reconciles `AgentGateway` resources and generates KrakenD configurations that expose `Agent` workloads through a unified gateway.

📖 **Documentation:** https://docs.agentic-layer.ai/agent-gateway-krakend-operator/

## Development

### Prerequisites

- **Go** 1.26+
- **Docker**
- **kubectl**
- **kind** (used for local development and E2E tests)
- **make**

### Build and deploy locally

```shell
# Create a local cluster and install cert-manager
kind create cluster

# Install Cert Manager for webhook TLS
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml

# Install the Agent Runtime Operator (provides the AgentGateway CRDs)
kubectl apply -f https://github.com/agentic-layer/agent-runtime-operator/releases/latest/download/install.yaml

# Install CRDs into the cluster
make install
# Build docker image
make docker-build
# Load image into kind cluster (not needed if using local registry)
make kind-load
# Deploy the operator to the cluster
make deploy
```

The controller manager runs in the `agent-gateway-krakend-operator-system` namespace.

### Test

```shell
make lint       # linting
make test       # unit + integration tests
make test-e2e   # E2E tests in a Kind cluster
```

For manual E2E setup against an existing cluster, see the `Makefile` targets `setup-test-e2e` and `cleanup-test-e2e`.

### Verify the local deploy

Apply the bundled `AgentGateway` + exposed `Agent` samples and check the gateway picks them up:

```shell
kubectl apply -k config/samples/
kubectl get agentgateways -o yaml
kubectl get deployments -l provider=krakend
kubectl get configmaps -l provider=krakend
```

### Create or Update API and Webhooks

The operator-sdk CLI can be used to create or update APIs and webhooks.
This is the preferred way to add new APIs and webhooks to the operator.
If the operator-sdk CLI is updated, you may need to re-run these commands to update the generated code.

```shell
# Create API for Agent CRD
operator-sdk create api --group runtime --version v1alpha1 --kind Agent

# Create webhook for Agent CRD
operator-sdk create webhook --group runtime --version v1alpha1 --kind Agent --defaulting --programmatic-validation
```

## Contributing

See the [Contribution Guide](https://github.com/agentic-layer/agent-gateway-krakend-operator?tab=contributing-ov-file).

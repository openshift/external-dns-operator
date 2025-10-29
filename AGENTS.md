This file provides guidance to AI agents working with the External DNS Operator repository.

## Repository Overview

The **External DNS Operator** manages [ExternalDNS](https://github.com/kubernetes-sigs/external-dns) deployments in OpenShift/Kubernetes clusters, automating DNS record creation across multiple cloud providers (AWS Route53, Azure DNS, GCP Cloud DNS, Infoblox, BlueCat).

**Key components**:
- Operator code: `pkg/operator/controller/` (externaldns, ca-configmap, credentials-secret controllers)
- CRD definitions: `api/v1beta1/externaldns_types.go` (storage version), `api/v1alpha1/` (deprecated)
- Deployment manifests: `config/` (CRDs, RBAC, samples for each provider)
- OLM artifacts: `bundle/`, `catalog/` (operator lifecycle management)
- Tests: Unit tests alongside code, e2e tests in `test/e2e/`

**ExternalDNS CRD** (cluster-scoped):
- Spec: `provider` (AWS/Azure/GCP/Infoblox/BlueCat), `source` (Route/Service/CRD), `zones`, `domains`
- Status: `conditions`, `observedGeneration`, `zones`

## Essential Commands

### Development
```bash
make build              # Generate code, fmt, vet, compile binary
make test               # Unit tests with race detector & coverage (uses envtest k8s 1.33.0)
make verify             # CI checks: lint, gofmt, deps, generated code, OLM bundle/catalog, version
make lint               # golangci-lint (.golangci.yaml: errcheck, gofmt, goimports, gosimple, govet, etc.)
```

### Code Generation (required after API changes)
```bash
make generate           # DeepCopy methods (controller-gen)
make manifests          # CRDs, RBAC, webhooks from kubebuilder markers
```

### Deployment
```bash
make deploy             # Deploy to cluster (uses kustomize, patches image to $IMG)
make undeploy           # Remove from cluster
```

### Container Images
```bash
export CONTAINER_ENGINE=podman  # or docker (default)
make image-build image-push     # Operator image (runs test first)
make bundle-image-build bundle-image-push  # OLM bundle
make catalog-image-build catalog-image-push # OLM catalog
```

### E2E Testing (requires cloud credentials)
```bash
export DNS_PROVIDER=AWS  # AWS, Azure, GCP, Infoblox, BlueCat
export AWS_ACCESS_KEY_ID=xxx AWS_SECRET_ACCESS_KEY=xxx  # Provider-specific creds
make test-e2e           # Runs test/e2e/operator_test.go (1h timeout)
```

## Makefile Target Details

**`make verify`** (Makefile:133-138) runs:
1. `lint` - golangci-lint with 10m timeout, vendor mode
2. `hack/verify-gofmt.sh` - All .go files must be gofmt-compliant
3. `hack/verify-deps.sh` - `go mod vendor && go mod tidy`, checks vendor/ sync (CI only)
4. `hack/verify-generated.sh` - `make generate manifests`, checks no uncommitted changes (CI only)
5. `hack/verify-olm.sh` - `make bundle catalog`, validates OLM manifests (CI only)
6. `hack/verify-version.sh` - VERSION file consistency

**`make test`** (Makefile:114-116):
- Pre: `make manifests generate fmt vet`
- Runs: `CGO_ENABLED=1 go test ./... -race -covermode=atomic -coverprofile coverage.out`
- Uses: `setup-envtest` with Kubernetes 1.33.0 binaries

**`make build`** (Makefile:146):
- Pre: `make generate fmt vet`
- Runs: `GO111MODULE=on GOFLAGS=-mod=vendor CGO_ENABLED=0 go build -ldflags "-X ..SHORTCOMMIT -X ..COMMIT"`
- Output: `bin/external-dns-operator`

## Working with the Codebase

### Modifying API Types (`api/v1beta1/externaldns_types.go`)
1. Edit types, add kubebuilder markers: `+kubebuilder:validation:Required`, `+kubebuilder:validation:Enum=A;B`, `+optional`
2. Run: `make generate manifests` (updates zz_generated.deepcopy.go and config/crd/bases/)
3. Update controller logic: `pkg/operator/controller/externaldns/deployment.go`, `pod.go`, `status.go`
4. Add tests, run: `make verify test build`
5. Generate OLM bundle: `make bundle`

### Adding DNS Providers
1. Update `api/v1beta1/externaldns_types.go`: Add enum, provider options struct, union field
2. `make generate manifests`
3. Update `pkg/operator/controller/externaldns/`: deployment.go (args), pod.go (volumes/env), credentials_request.go (CCO)
4. Add sample CR: `config/samples/<provider>/`
5. Add e2e tests: `test/e2e/<provider>.go`
6. Update docs: README.md, docs/usage.md, docs/<provider>-openshift.md

### Testing Strategy
- **Unit**: `*_test.go` alongside code, uses gomega matchers, controller-runtime/envtest
- **E2E**: `test/e2e/operator_test.go`, requires cluster + provider credentials, build tag `e2e`
- **Coverage**: View with `go tool cover -func=coverage.out`

## PR Review Requirements

**CI must pass**:
1. ✅ All `make verify` checks (lint, format, deps, generated, OLM)
2. ✅ All `make test` passes
3. ✅ `make build` succeeds
4. ✅ Generated files committed (`make generate manifests bundle catalog`)
5. ✅ Tests updated for controller changes
6. ✅ Documentation updated for user-facing changes
7. ✅ Commit format: `<JIRA-ID>: Description` (e.g., `NE-2076: Add feature X`)

## Repository Structure (Key Paths)

```
api/v1beta1/externaldns_types.go  # Primary API definition (DO NOT EDIT zz_generated.*)
pkg/operator/controller/
  externaldns/                     # Main controller
    controller.go                  # Reconciliation loop
    deployment.go                  # ExternalDNS deployment generation
    pod.go                         # Pod spec (provider-specific args/volumes)
    status.go                      # Status condition updates
  ca-configmap/                    # Trusted CA sync
  credentials-secret/              # Secret replication
config/
  crd/bases/                       # Generated CRDs (from make manifests)
  samples/                         # Provider-specific example CRs
  rbac/                            # ClusterRole, bindings
Makefile                           # Build targets (see lines 114, 133, 146)
.golangci.yaml                     # Linter config (10 enabled linters)
```

## Known Limitations

**Domain name length**: TXT registry prefix (`external-dns-<type>-<name>`) reduces max length:
- CNAME: 44 chars (42 for wildcard on Azure)
- A: 48 chars (46 for wildcard on Azure)

## Custom Claude Code Command

**`/external-dns-operator-helper <pr-url>`**

Simple, automated PR review workflow - single execution, no prompts:

1. **Pre-flight**: Check clean working dir, verify upstream remote
2. **Checkout PR**: Fetch and checkout using native git commands
3. **CI Status** ⭐: Display Prow job results with clickable links (✅/❌/⏳)
4. **Commit Validation**: Ensure `<JIRA-ID>: Description` format
5. **Effective Go Checks** ⭐: Validate receiver names, error strings, exported docs (https://go.dev/doc/effective_go)
6. **Local Verification**: `make verify` (lint, format, deps, generated, OLM)
7. **Unit Tests**: `make test` with coverage
8. **Build**: `make build` compilation check
9. **Specialized Checks**: API/CRD sync, controller/test coverage, docs
10. **Summary**: Clear pass/fail report with timing
11. **Auto Cleanup**: Return to original branch

**Requirements**:
- Git repository with `upstream` remote pointing to `https://github.com/openshift/external-dns-operator.git`
- `jq` (optional - gracefully degrades without it)

**Key Features**:
- ✅ **Single execution** - No multiple prompts or approvals
- ✅ **Fast** - Completes in ~2-3 minutes
- 🎯 **Prow CI integration** - Shows all job statuses with links
- 📚 **Effective Go validation** - Catches style issues beyond golangci-lint
- ⏱️ **Time savings** - ~15 minutes per PR review
- 🛡️ **Robust** - Automatic cleanup even on failures
- 🚀 **Minimal dependencies** - Just git, curl (jq optional)

## Environment Variables

**Build**: `IMG`, `BUNDLE_IMG`, `CATALOG_IMG`, `CONTAINER_ENGINE` (docker/podman), `BUNDLE_VERSION`
**E2E**: `KUBECONFIG`, `DNS_PROVIDER`, `AWS_ACCESS_KEY_ID`, `AZURE_CLIENT_ID`, `GCP_PROJECT`, `INFOBLOX_GRID_HOST`, etc.

## Resources

- [Operator SDK](https://sdk.operatorframework.io/), [Kubebuilder](https://book.kubebuilder.io/)
- [ExternalDNS Docs](https://kubernetes-sigs.github.io/external-dns/)
- [Enhancement Proposal](https://github.com/openshift/enhancements/pull/786)

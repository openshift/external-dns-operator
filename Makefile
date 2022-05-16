# BUNDLE_VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the BUNDLE_VERSION as arg of the bundle target (e.g make bundle BUNDLE_VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export BUNDLE_VERSION=0.0.2)
BUNDLE_VERSION ?= 1.0.0

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= quay.io/openshift/origin-external-dns-operator:latest
# Used for internal registry access
TLS_VERIFY ?= true

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:preserveUnknownFields=false"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CONTROLLER_GEN := go run sigs.k8s.io/controller-tools/cmd/controller-gen
SETUP_ENVTEST := go run sigs.k8s.io/controller-runtime/tools/setup-envtest
K8S_ENVTEST_VERSION := 1.21.4

PACKAGE=github.com/openshift/external-dns-operator

BIN=bin/$(lastword $(subst /, ,$(PACKAGE)))
BIN_DIR=$(shell pwd)/bin

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

CONTAINER_ENGINE ?= docker

BUNDLE_DIR := bundle
BUNDLE_MANIFEST_DIR := $(BUNDLE_DIR)/manifests
BUNDLE_IMG ?= olm-bundle:latest
INDEX_IMG ?= olm-bundle-index:latest
OPM_VERSION ?= v1.17.4

GOLANGCI_LINT_BIN=$(BIN_DIR)/golangci-lint

OPERATOR_SDK_BIN=$(BIN_DIR)/operator-sdk

COMMIT ?= $(shell git rev-parse HEAD)
SHORTCOMMIT ?= $(shell git rev-parse --short HEAD)
GOBUILD_VERSION_ARGS = -ldflags "-X $(PACKAGE)/pkg/version.SHORTCOMMIT=$(SHORTCOMMIT) -X $(PACKAGE)/pkg/version.COMMIT=$(COMMIT)"

E2E_TIMEOUT ?= 1h

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=external-dns-operator webhook paths="./..." output:crd:artifacts:config=config/crd/bases

generate: ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

ENVTEST_ASSETS_DIR ?= $(shell pwd)/testbin
test: manifests generate fmt vet ## Run tests.
	mkdir -p "$(ENVTEST_ASSETS_DIR)"
	KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use "$(K8S_ENVTEST_VERSION)" --print path --bin-dir "$(ENVTEST_ASSETS_DIR)")" go test ./... -race -covermode=atomic -coverprofile coverage.out

.PHONY: test-e2e
test-e2e:
	go test \
	$(GOBUILD_VERSION_ARGS) \
	-timeout $(E2E_TIMEOUT) \
	-count 1 \
	-v \
	-tags e2e \
	-run "$(TEST)" \
	./test/e2e

verify: lint
	hack/verify-gofmt.sh
	hack/verify-deps.sh
	hack/verify-generated.sh
	hack/verify-olm.sh

##@ Build
GO=GO111MODULE=on GOFLAGS=-mod=vendor CGO_ENABLED=0 go

build-operator: ## Build operator binary, no additional checks or code generation
	$(GO) build $(GOBUILD_VERSION_ARGS) -o $(BIN) $(PACKAGE)

build: generate build-operator fmt vet ## Build operator binary.

run: manifests generate fmt vet ## Run a controller from your host.
	go run $(PACKAGE)

image-build: test ## Build container image with the operator.
	$(CONTAINER_ENGINE) build -t ${IMG} .

image-push: ## Push container image with the operator.
	$(CONTAINER_ENGINE) push ${IMG}  --tls-verify=${TLS_VERIFY}

##@ Deployment

install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	# do not commit the following 2 changes
	cd config/manager && $(KUSTOMIZE) edit set image quay.io/openshift/origin-external-dns-operator=${IMG}
	# webhook volume and service are added explicilty so that they don't land in the bundle where it's managed by OLM
	cd config/default && $(KUSTOMIZE) edit add patch --path=manager_webhook_volume_patch.yaml
	# disable tls config in service monitor
	cd config/prometheus && $(KUSTOMIZE) edit add patch --path=insecure_tls_patch.yaml
	# consume certificate from the service serving certificate
	cd config/default && $(KUSTOMIZE) edit add patch --path=manager_insecure_tls_auth_proxy_patch.yaml
	$(KUSTOMIZE) build config/default | kubectl apply -f -

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

.PHONY: bundle
bundle: $(OPERATOR_SDK_BIN) manifests kustomize
	$(OPERATOR_SDK_BIN) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image quay.io/openshift/origin-external-dns-operator=${IMG}
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK_BIN) generate bundle -q --overwrite=false --version $(BUNDLE_VERSION) $(BUNDLE_METADATA_OPTS)
	$(OPERATOR_SDK_BIN) bundle validate $(BUNDLE_DIR)

.PHONY: bundle-image-build
bundle-image-build: bundle
	$(CONTAINER_ENGINE) build -t ${BUNDLE_IMG} -f Dockerfile.bundle .

.PHONY: bundle-image-push
bundle-image-push:
	$(CONTAINER_ENGINE) push ${BUNDLE_IMG}

.PHONY: index-image-build
index-image-build: opm
	$(OPM) index add -c $(CONTAINER_ENGINE) --bundles ${BUNDLE_IMG} --tag ${INDEX_IMG}

.PHONY: index-image-push
index-image-push:
	$(CONTAINER_ENGINE) push ${INDEX_IMG}

KUSTOMIZE = $(BIN_DIR)/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef


OPM=$(BIN_DIR)/opm
opm: ## Download opm locally if necessary.
	$(call get-bin,$(OPM),$(BIN_DIR),https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/linux-amd64-opm)

define get-bin
@[ -f "$(1)" ] || { \
	[ ! -d "$(2)" ] && mkdir -p "$(2)" || true ;\
	echo "Downloading $(3)" ;\
	curl -fL $(3) -o "$(1)" ;\
	chmod +x "$(1)" ;\
}
endef

.PHONY: lint
## Checks the code with golangci-lint
lint: $(GOLANGCI_LINT_BIN)
	$(GOLANGCI_LINT_BIN) run -c .golangci.yaml --deadline=30m

$(GOLANGCI_LINT_BIN):
	mkdir -p $(BIN_DIR)
	hack/golangci-lint.sh $(GOLANGCI_LINT_BIN)

$(OPERATOR_SDK_BIN):
	mkdir -p $(BIN_DIR)
	hack/operator-sdk.sh $(OPERATOR_SDK_BIN)

clean:
	$(GO) clean
	rm -f $(BIN)

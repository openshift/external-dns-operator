
# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Used for internal registry access
TLS_VERIFY ?= true

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

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
MAIN_PACKAGE=$(PACKAGE)/cmd/external-dns-operator

BIN=bin/$(lastword $(subst /, ,$(MAIN_PACKAGE)))
BIN_DIR=$(shell pwd)/bin

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

CONTAINER_ENGINE ?= docker

BUNDLE_MANIFEST_DIR := bundle/manifests
BUNDLE_IMG ?= olm-bundle:latest
INDEX_IMG ?= olm-bundle-index:latest
OPM_VERSION ?= v1.17.4

GOLANGCI_LINT_BIN=$(BIN_DIR)/golangci-lint
GOLANGCI_LINT_VERSION=v1.42.1

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
	-timeout 1h \
	-count 1 \
	-v \
	-tags e2e \
	-run "$(TEST)" \
	./test/e2e

verify: lint
	hack/verify-gofmt.sh
	hack/verify-deps.sh
	hack/verify-generated.sh

##@ Build

GO=GO111MODULE=on GOFLAGS=-mod=vendor CGO_ENABLED=0 go

build-operator: ## Build operator binary, no additional checks or code generation
	$(GO) build -o $(BIN) $(MAIN_PACKAGE)

build: generate build-operator fmt vet ## Build operator binary.

run: manifests generate fmt vet ## Run a controller from your host.
	go run $(MAIN_PACKAGE)

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
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

.PHONY: olm-manifests
# A little helper command to generate the manifests of OLM bundle from the files in config/.
# The idea is that config/ is the main directory for the manifests and OLM manifests are secondary
# and is supposed to be updated afterwards.
# Note that ClusterServiceVersion is not touched as is supposed to be verified manually.
# The install strategy of ClusterServiceVersion contains the deployment which is not copied over from config/ either.
olm-manifests: manifests
	cp -f config/crd/bases/externaldns.olm.openshift.io_externaldnses.yaml $(BUNDLE_MANIFEST_DIR)/externaldns.olm.openshift.io_crd.yaml
	cp -f config/rbac/role.yaml $(BUNDLE_MANIFEST_DIR)/external-dns-operator_rbac.authorization.k8s.io_v1_clusterrole.yaml
	cp -f config/rbac/role_binding.yaml $(BUNDLE_MANIFEST_DIR)/external-dns-operator_rbac.authorization.k8s.io_v1_clusterrolebinding.yaml
	cp -f config/rbac/auth_proxy_role.yaml $(BUNDLE_MANIFEST_DIR)/external-dns-operator-auth-proxy_rbac.authorization.k8s.io_v1_clusterrole.yaml
	cp -f config/rbac/auth_proxy_role_binding.yaml $(BUNDLE_MANIFEST_DIR)/external-dns-operator-auth-proxy_rbac.authorization.k8s.io_v1_clusterrolebinding.yaml
	cp -f config/rbac/auth_proxy_service.yaml $(BUNDLE_MANIFEST_DIR)/external-dns-operator-auth-proxy_v1_service.yaml
	# opm is unable to find CRD if the standard yaml --- is at the top
	sed -i -e '/^---$$/d' -e '/^$$/d' $(BUNDLE_MANIFEST_DIR)/*.yaml
	# as per the recommendation of 'operator-sdk bundle validate' command
	# strip the namespaces from the bundle manifests
	sed -i '/namespace:/d' $(BUNDLE_MANIFEST_DIR)/*.yaml

.PHONY: bundle-image-build
bundle-image-build: olm-manifests
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

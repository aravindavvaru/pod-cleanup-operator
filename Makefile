# Image URL to use for all operations
IMG ?= pod-cleanup-operator:latest

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded
ENVTEST_K8S_VERSION = 1.29.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests.
	go test ./... -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run

##@ Build

.PHONY: build
build: fmt vet ## Build manager binary.
	go build -o bin/manager ./cmd/main.go

.PHONY: run
run: fmt vet ## Run the controller from your host against the current cluster.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl apply -k config/crd/bases/

.PHONY: uninstall
uninstall: ## Uninstall CRDs from the K8s cluster.
	kubectl delete --ignore-not-found=$(ignore-not-found) -k config/crd/bases/

.PHONY: deploy
deploy: ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	sed -i.bak "s|image: .*pod-cleanup-operator.*|image: ${IMG}|" config/manager/manager.yaml && rm -f config/manager/manager.yaml.bak
	kubectl apply -k config/

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster.
	kubectl delete --ignore-not-found=$(ignore-not-found) -k config/

.PHONY: sample
sample: ## Apply the sample PodCleanupPolicy to the cluster.
	kubectl apply -f config/samples/

##@ Dependencies

.PHONY: golangci-lint
GOLANGCI_LINT = $(GOBIN)/golangci-lint
golangci-lint: ## Download golangci-lint locally if necessary.
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,v1.54.2)

# go-install-tool will 'go install' any package with custom target and name.
# Usage: $(call go-install-tool,<location>,<pkg>,<version>)
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3); \
echo "Downloading $${package}"; \
GOBIN=$(GOBIN) go install $${package}; \
}
endef

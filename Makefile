# OPAL Makefile
# Adapted from kubebuilder project scaffold

# Image settings
IMG ?= ghcr.io/opal-io/opal:latest
PLATFORMS ?= linux/arm64,linux/amd64

# Go settings
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GO_BUILD_FLAGS ?= -trimpath

# Tool versions
ENVTEST_VERSION ?= release-0.17
CONTROLLER_GEN_VERSION ?= v0.14.0
GOLANGCI_LINT_VERSION ?= v1.57.2
HELM_DOCS_VERSION ?= v1.13.1

# Directories
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate CRD manifests and RBAC.
	$(CONTROLLER_GEN) rbac:roleName=opal-manager-role crd webhook paths="./..." \
		output:crd:artifacts:config=config/crd/bases \
		output:rbac:artifacts:config=config/rbac

.PHONY: generate
generate: controller-gen ## Generate deep copy and other code.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint.
	$(GOLANGCI_LINT) run --timeout 5m

##@ Testing

.PHONY: test
test: manifests generate fmt vet envtest ## Run unit tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-path $(LOCALBIN) -p path)" \
		go test ./... -coverprofile cover.out -v

.PHONY: test-integration
test-integration: ## Run integration tests (requires cluster).
	go test ./... -tags=integration -v

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests.
	go test ./test/e2e/... -v -timeout 30m

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build $(GO_BUILD_FLAGS) -o bin/manager ./cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run manager locally against current cluster.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build Docker image.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push Docker image.
	docker push ${IMG}

.PHONY: docker-buildx
docker-buildx: ## Build and push multi-platform Docker image.
	docker buildx build --platform=$(PLATFORMS) --push -t ${IMG} .

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests ## Install CRDs into the cluster.
	kubectl apply -f config/crd/bases/

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from the cluster.
	kubectl delete --ignore-not-found=$(ignore-not-found) -f config/crd/bases/

.PHONY: deploy
deploy: manifests ## Deploy OPAL to the cluster.
	kubectl apply -f config/manager/

.PHONY: undeploy
undeploy: ## Undeploy OPAL from the cluster.
	kubectl delete --ignore-not-found=$(ignore-not-found) -f config/manager/

.PHONY: deploy-agents
deploy-agents: ## Deploy OPAL kagent agents to the cluster.
	kubectl apply -f agents/

.PHONY: deploy-samples
deploy-samples: ## Deploy sample WorkloadOutcome manifests.
	kubectl apply -f config/samples/

##@ Helm

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart.
	helm lint charts/opal

.PHONY: helm-template
helm-template: ## Render Helm chart templates.
	helm template opal charts/opal --namespace opal-system

.PHONY: helm-install
helm-install: ## Install via Helm (requires LLM secret to exist).
	helm install opal charts/opal \
		--namespace opal-system \
		--create-namespace \
		--wait

.PHONY: helm-upgrade
helm-upgrade: ## Upgrade via Helm.
	helm upgrade opal charts/opal --namespace opal-system --wait

##@ Tool Installation

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_GEN_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
}
endef

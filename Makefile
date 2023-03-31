GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT?=$(shell git rev-parse --short HEAD)
GIT_TAG?=$(shell git describe --abbrev=0 --tags 2>/dev/null || echo "v0.0.0" )
TAG?=${GIT_TAG}-${GIT_COMMIT_SHORT}
REPO?=quay.io/costoolkit/elemental-operator-ci
REPO_REGISTER?=quay.io/costoolkit/elemental-register-ci
TAG_SEEDIMAGE?=${TAG}
REPO_SEEDIMAGE?=quay.io/costoolkit/seedimage-builder-ci
export ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
CHART?=$(shell find $(ROOT_DIR) -type f  -name "elemental-operator*.tgz" -print)
CHART_VERSION?=$(subst v,,$(GIT_TAG))
KUBE_VERSION?="v1.24.6"
CLUSTER_NAME?="operator-e2e"
RAWCOMMITDATE=$(shell git log -n1 --format="%at")
GO_TPM_TAG?=$(shell grep google/go-tpm-tools go.mod | awk '{print $$2}')
E2E_CONF_FILE ?= $(ROOT_DIR)/tests/e2e/config/config.yaml

ifeq ($(shell go env GOOS),darwin) # Use the darwin/amd64 binary until an arm64 version is available
	KUBEBUILDER_ASSETS ?= $(shell $(SETUP_ENVTEST) use --use-env -p path --arch amd64 $(KUBEBUILDER_ENVTEST_KUBERNETES_VERSION))
	COMMITDATE?=$(shell gdate -d @"${RAWCOMMITDATE}" "+%FT%TZ")
else
	KUBEBUILDER_ASSETS ?= $(shell $(SETUP_ENVTEST) use --use-env -p path $(KUBEBUILDER_ENVTEST_KUBERNETES_VERSION))
	COMMITDATE?=$(shell date -d @"${RAWCOMMITDATE}" "+%FT%TZ")
endif

LDFLAGS := -w -s
LDFLAGS += -X "github.com/rancher/elemental-operator/pkg/version.Version=${GIT_TAG}"
LDFLAGS += -X "github.com/rancher/elemental-operator/pkg/version.Commit=${GIT_COMMIT}"
LDFLAGS += -X "github.com/rancher/elemental-operator/pkg/version.CommitDate=${COMMITDATE}"

# Directories
BUILD_DIR := build
ABS_TOOLS_DIR :=  $(abspath tools/)

GO_INSTALL := ./scripts/go_install.sh

# Binaries.
# Need to use abspath so we can invoke these from subdirectories
CONTROLLER_GEN_VER := v0.9.2
CONTROLLER_GEN := $(ABS_TOOLS_DIR)/controller-gen-$(CONTROLLER_GEN_VER)
CONTROLLER_GEN_PKG := sigs.k8s.io/controller-tools/cmd/controller-gen

GINKGO_VER := v2.3.1
GINKGO := $(ABS_TOOLS_DIR)/ginkgo-$(GINKGO_VER)
GINKGO_PKG := github.com/onsi/ginkgo/v2/ginkgo

SETUP_ENVTEST_VER := v0.0.0-20211110210527-619e6b92dab9
SETUP_ENVTEST := $(ABS_TOOLS_DIR)/setup-envtest-$(SETUP_ENVTEST_VER)
SETUP_ENVTEST_PKG := sigs.k8s.io/controller-runtime/tools/setup-envtest

KUSTOMIZE_VER := v4.5.2
KUSTOMIZE := $(ABS_TOOLS_DIR)/kustomize-$(KUSTOMIZE_VER)
KUSTOMIZE_PKG := sigs.k8s.io/kustomize/kustomize/v4

$(CONTROLLER_GEN): 
	GOBIN=$(ABS_TOOLS_DIR) $(GO_INSTALL) $(CONTROLLER_GEN_PKG) controller-gen $(CONTROLLER_GEN_VER)

$(GINKGO):
	GOBIN=$(ABS_TOOLS_DIR) $(GO_INSTALL) $(GINKGO_PKG) ginkgo $(GINKGO_VER)

$(SETUP_ENVTEST): 
	GOBIN=$(ABS_TOOLS_DIR) $(GO_INSTALL) $(SETUP_ENVTEST_PKG) setup-envtest $(SETUP_ENVTEST_VER)

$(KUSTOMIZE):
	CGO_ENABLED=0 GOBIN=$(ABS_TOOLS_DIR) $(GO_INSTALL) $(KUSTOMIZE_PKG) kustomize $(KUSTOMIZE_VER)

.PHONY: build
build: operator register support

.PHONY: operator
operator:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/elemental-operator $(ROOT_DIR)/cmd/operator

.PHONY: register
register:
	CGO_ENABLED=1 go build -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/elemental-register $(ROOT_DIR)/cmd/register

.PHONY: support
support:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/elemental-support $(ROOT_DIR)/cmd/support

.PHONY: httpfy
httpfy:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/elemental-httpfy $(ROOT_DIR)/utils/httpfy

.PHONY: build-docker-operator
build-docker-operator:
	DOCKER_BUILDKIT=1 docker build \
		-f Dockerfile \
		--target elemental-operator \
		--build-arg "TAG=${GIT_TAG}" \
		--build-arg "COMMIT=${GIT_COMMIT}" \
		--build-arg "COMMITDATE=${COMMITDATE}" \
		-t ${REPO}:${TAG} .

.PHONY: build-docker-register
build-docker-register:
	DOCKER_BUILDKIT=1 docker build \
		-f Dockerfile \
		--target elemental-register \
		--build-arg "TAG=${GIT_TAG}" \
		--build-arg "COMMIT=${GIT_COMMIT}" \
		--build-arg "COMMITDATE=${COMMITDATE}" \
		-t ${REPO_REGISTER}:${TAG} .

.PHONY: build-docker-seedimage-builder
build-docker-seedimage-builder:
	DOCKER_BUILDKIT=1 docker build \
		-f Dockerfile.seedimage \
		-t ${REPO_SEEDIMAGE}:${TAG} .

.PHONY: build-docker-push-operator
build-docker-push-operator: build-docker-operator
	docker push ${REPO}:${TAG}

.PHONY: build-docker-push-register
build-docker-push-register: build-docker-register
	docker push ${REPO_REGISTER}:${TAG}

.PHONY: build-docker-push-seedimage-builder
build-docker-push-seedimage-builder: build-docker-seedimage-builder
	docker push ${REPO_SEEDIMAGE}:${TAG}

.PHONY: chart
chart:
	mkdir -p  $(ROOT_DIR)/build
	cp -rf $(ROOT_DIR)/chart $(ROOT_DIR)/build/chart
	yq -i '.image.tag = "${TAG}"' $(ROOT_DIR)/build/chart/values.yaml
	yq -i '.image.repository = "${REPO}"' $(ROOT_DIR)/build/chart/values.yaml
	yq -i '.seedImage.tag = "${TAG_SEEDIMAGE}"' $(ROOT_DIR)/build/chart/values.yaml
	yq -i '.seedImage.repository = "${REPO_SEEDIMAGE}"' $(ROOT_DIR)/build/chart/values.yaml
	helm package --version ${CHART_VERSION} --app-version ${GIT_TAG} -d $(ROOT_DIR)/build/ $(ROOT_DIR)/build/chart
	rm -Rf $(ROOT_DIR)/build/chart

validate:
	scripts/validate

.PHONY: unit-tests
unit-tests: $(SETUP_ENVTEST) $(GINKGO)
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" $(GINKGO) -v -r --trace --race --covermode=atomic --coverprofile=coverage.out --coverpkg=github.com/rancher/elemental-operator/... ./pkg/... ./controllers/...

e2e-tests: $(GINKGO) setup-kind
	export EXTERNAL_IP=`kubectl get nodes -o jsonpath='{.items[].status.addresses[?(@.type == "InternalIP")].address}'` && \
	export BRIDGE_IP="172.18.0.1" && \
	export CONFIG_PATH=$(E2E_CONF_FILE) && \
	cd $(ROOT_DIR)/tests && $(GINKGO) -r -v ./e2e

# Only setups the kind cluster
setup-kind:
	KUBE_VERSION=${KUBE_VERSION} $(ROOT_DIR)/scripts/setup-kind-cluster.sh

# setup the cluster but not run any test!
# This will build the image locally, the chart with that image,
# setup kind, load the local built image into the cluster,
# and run a test that does nothing but installs everything for
# the elemental operator (nginx, rancher, operator, etc..) as part of the BeforeSuite
# So you end up with a clean cluster in which nothing has run
setup-full-cluster: build-docker-operator chart setup-kind
	@export EXTERNAL_IP=`kubectl get nodes -o jsonpath='{.items[].status.addresses[?(@.type == "InternalIP")].address}'` && \
	export BRIDGE_IP="172.18.0.1" && \
	export CHART=$(CHART) && \
	export CONFIG_PATH=$(E2E_CONF_FILE) && \
	kind load docker-image --name $(CLUSTER_NAME) ${REPO}:${TAG} && \
	cd $(ROOT_DIR)/tests && $(GINKGO) -r -v --label-filter="do-nothing" ./e2e

kind-e2e-tests: build-docker-operator chart setup-kind
	export CONFIG_PATH=$(E2E_CONF_FILE) && \
	kind load docker-image --name $(CLUSTER_NAME) ${REPO}:${TAG}
	$(MAKE) e2e-tests

# This builds the docker image, generates the chart, loads the image into the kind cluster and upgrades the chart to latest
# useful to test changes into the operator with a running system, without clearing the operator namespace
# thus losing any registration/inventories/os CRDs already created
reload-operator: build-docker-operator chart
	kind load docker-image --name $(CLUSTER_NAME) ${REPO}:${TAG}
	helm upgrade -n cattle-elemental-system elemental-operator $(CHART)

.PHONY: vendor
vendor:
	go mod tidy
	go mod vendor
	curl -L 'https://github.com/google/go-tpm-tools/archive/refs/tags/$(GO_TPM_TAG).tar.gz' --output go-tpm-tools.tar.gz
	tar xaf go-tpm-tools.tar.gz --strip-components=1 -C vendor/github.com/google/go-tpm-tools
	rm go-tpm-tools.tar.gz

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate code
	$(MAKE) generate-go
	$(MAKE) generate-manifests

.PHONY: generate-manifests
generate-manifests: $(CONTROLLER_GEN) ## Generate manifests for the operator e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) \
		paths=./api/... \
		paths=./controllers/... \
		paths=./cmd/operator/operator/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./config/crd/bases \
		output:rbac:dir=./config/rbac/bases \
		output:webhook:dir=./config/webhook \
		webhook

.PHONY: generate-go
generate-go: $(CONTROLLER_GEN) ## Runs Go related generate targets for the operator
	$(CONTROLLER_GEN) \
		object:headerFile=$(ROOT)scripts/boilerplate.go.txt \
		paths=./api/...

build-crds: $(KUSTOMIZE)
	$(KUSTOMIZE) build config/crd > chart/templates/crds.yaml

build-rbac: $(KUSTOMIZE)
	$(KUSTOMIZE) build config/rbac > chart/templates/cluster_role.yaml

build-manifests: $(KUSTOMIZE) generate
	$(MAKE) build-crds
	$(MAKE) build-rbac

ALL_VERIFY_CHECKS = manifests vendor

.PHONY: verify
verify: $(addprefix verify-,$(ALL_VERIFY_CHECKS))

.PHONY: verify-manifests
verify-manifests: build-manifests
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi

.PHONY: verify-vendor
verify-vendor: vendor
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi

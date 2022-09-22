GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT?=$(shell git rev-parse --short HEAD)
GIT_TAG?=$(shell git describe --abbrev=0 --tags 2>/dev/null || echo "v0.0.0" )
TAG?=${GIT_TAG}-${GIT_COMMIT_SHORT}
REPO?=quay.io/costoolkit/elemental-operator-ci
REPO_REGISTER?=quay.io/costoolkit/elemental-register-ci
export ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
CHART?=$(shell find $(ROOT_DIR) -type f  -name "elemental-operator*.tgz" -print)
CHART_VERSION?=$(subst v,,$(GIT_TAG))
KUBE_VERSION?="v1.22.7"
CLUSTER_NAME?="operator-e2e"
RAWCOMMITDATE=$(shell git log -n1 --format="%at")

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
BIN_DIR := bin
TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(abspath $(TOOLS_DIR)/$(BIN_DIR))

GO_INSTALL := ./scripts/go_install.sh

# Binaries.
# Need to use abspath so we can invoke these from subdirectories
CONTROLLER_GEN_VER := v0.9.2
CONTROLLER_GEN_BIN := controller-gen
CONTROLLER_GEN := $(abspath $(TOOLS_BIN_DIR)/$(CONTROLLER_GEN_BIN)-$(CONTROLLER_GEN_VER))
CONTROLLER_GEN_PKG := sigs.k8s.io/controller-tools/cmd/controller-gen

GINKGO_VER := v1.16.5
GINKGO_BIN := ginkgo
GINKGO := $(TOOLS_BIN_DIR)/$(GINKGO_BIN)-$(GINKGO_VER)
GINKGO_PKG := github.com/onsi/ginkgo/ginkgo

SETUP_ENVTEST_VER := v0.0.0-20211110210527-619e6b92dab9
SETUP_ENVTEST_BIN := setup-envtest
SETUP_ENVTEST := $(abspath $(TOOLS_BIN_DIR)/$(SETUP_ENVTEST_BIN)-$(SETUP_ENVTEST_VER))
SETUP_ENVTEST_PKG := sigs.k8s.io/controller-runtime/tools/setup-envtest

KUSTOMIZE_VER := v4.5.2
KUSTOMIZE_BIN := kustomize
KUSTOMIZE := $(abspath $(TOOLS_BIN_DIR)/$(KUSTOMIZE_BIN)-$(KUSTOMIZE_VER))
KUSTOMIZE_PKG := sigs.k8s.io/kustomize/kustomize/v4

.PHONY: $(CONTROLLER_GEN_BIN)
$(CONTROLLER_GEN_BIN): $(CONTROLLER_GEN) ## Build a local copy of controller-gen.

.PHONY: $(SETUP_ENVTEST_BIN)
$(SETUP_ENVTEST_BIN): $(SETUP_ENVTEST) ## Build a local copy of setup-envtest.

.PHONY: $(KUSTOMIZE_BIN)
$(KUSTOMIZE_BIN): $(KUSTOMIZE) ## Build a local copy of kustomize.

$(CONTROLLER_GEN): # Build controller-gen from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(CONTROLLER_GEN_PKG) $(CONTROLLER_GEN_BIN) $(CONTROLLER_GEN_VER)

$(GINKGO): ## Build ginkgo from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(GINKGO_PKG) $(GINKGO_BIN) $(GINKGO_VER)

$(SETUP_ENVTEST): # Build setup-envtest from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(SETUP_ENVTEST_PKG) $(SETUP_ENVTEST_BIN) $(SETUP_ENVTEST_VER)
	
$(KUSTOMIZE): # Build kustomize from tools folder.
	CGO_ENABLED=0 GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(KUSTOMIZE_PKG) $(KUSTOMIZE_BIN) $(KUSTOMIZE_VER)

.PHONY: build
build: operator register support

.PHONY: operator
operator:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o build/elemental-operator $(ROOT_DIR)/cmd/operator

operator-ctrl-runtime:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o build/elemental-operator-ctrl-runtime $(ROOT_DIR)/cmd/operator-ctrl-runtime

.PHONY: register
register:
	CGO_ENABLED=1 go build -ldflags '$(LDFLAGS)' -o build/elemental-register $(ROOT_DIR)/cmd/register

.PHONY: support
support:
	CGO_ENABLED=0 go build -ldflags '$(LDFLAGS)' -o build/elemental-support $(ROOT_DIR)/cmd/support


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

.PHONY: build-docker-push-operator
build-docker-push-operator: build-docker-operator
	docker push ${REPO}:${TAG}

.PHONY: build-docker-push-register
build-docker-push-register: build-docker-register
	docker push ${REPO_REGISTER}:${TAG}

.PHONY: chart
chart:
	mkdir -p  $(ROOT_DIR)/build
	cp -rf $(ROOT_DIR)/chart $(ROOT_DIR)/build/chart
	sed -i -e 's/tag:.*/tag: '${TAG}'/' $(ROOT_DIR)/build/chart/values.yaml
	sed -i -e 's|repository:.*|repository: '${REPO}'|' $(ROOT_DIR)/build/chart/values.yaml
	helm package --version ${CHART_VERSION} --app-version ${GIT_TAG} -d $(ROOT_DIR)/build/ $(ROOT_DIR)/build/chart
	rm -Rf $(ROOT_DIR)/build/chart

validate:
	scripts/validate

.PHONY: unit-tests
unit-tests: $(SETUP_ENVTEST) $(GINKGO)
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" $(GINKGO) -v -p -r --trace ./pkg/... ./controllers/...

e2e-tests: $(GINKGO) setup-kind
	export EXTERNAL_IP=`kubectl get nodes -o jsonpath='{.items[].status.addresses[?(@.type == "InternalIP")].address}'` && \
	export BRIDGE_IP="172.18.0.1" && \
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
	kind load docker-image --name $(CLUSTER_NAME) ${REPO}:${TAG} && \
	cd $(ROOT_DIR)/tests && $(GINKGO) -r -v --label-filter="do-nothing" ./e2e

kind-e2e-tests: build-docker-operator chart setup-kind
	kind load docker-image --name $(CLUSTER_NAME) ${REPO}:${TAG}
	CHART=$(CHART) $(MAKE) e2e-tests

# This builds the docker image, generates the chart, loads the image into the kind cluster and upgrades the chart to latest
# useful to test changes into the operator with a running system, without clearing the operator namespace
# thus losing any registration/inventories/os CRDs already created
reload-operator: build-docker-operator chart
	kind load docker-image --name $(CLUSTER_NAME) ${REPO}:${TAG}
	helm upgrade -n cattle-elemental-system elemental-operator $(CHART)

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate code
	$(MAKE) generate-go
	$(MAKE) generate-manifests

.PHONY: generate-manifests
generate-manifests: $(CONTROLLER_GEN) ## Generate manifests for the operator e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) \
		paths=./api/... \
		paths=./controllers/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./config/crd/bases \
		output:rbac:dir=./config/rbac \
		output:webhook:dir=./config/webhook \
		webhook

.PHONY: generate-go
generate-go: $(CONTROLLER_GEN) ## Runs Go related generate targets for the operator
	$(CONTROLLER_GEN) \
		object:headerFile=$(ROOT)scripts/boilerplate.go.txt \
		paths=./api/...

build-crds: $(KUSTOMIZE)
	$(KUSTOMIZE) build config/crd > chart/templates/crds.yaml
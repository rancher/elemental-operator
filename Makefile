GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT?=$(shell git rev-parse --short HEAD)
GIT_TAG?=$(shell git describe --abbrev=0 --tags 2>/dev/null || echo "v0.0.0" )
CHART_VERSION?=$(shell echo $(GIT_TAG) | sed 's/[a-z]*\([0-9]\(\.[0-9]\)\{0,2\}\).*/\1/g')
TAG?=${GIT_TAG}
REPO?=rancher/elemental-operator
TAG_SEEDIMAGE?=${CHART_VERSION}
REPO_SEEDIMAGE?=rancher/seedimage-builder
TAG_CHANNEL?=${CHART_VERSION}
REPO_CHANNEL?=rancher/elemental-channel
REGISTRY_URL?=registry.opensuse.org/isv/rancher/elemental/dev/containers
DOCKER_ARGS=
LOCAL_BUILD?=false
ifneq ($(REGISTRY_URL),)
	REGISTRY_HEADER := $(REGISTRY_URL)/
else
	REGISTRY_HEADER := ""
endif

export ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
CHART?=$(shell find $(ROOT_DIR) -type f  -name "elemental-operator-$(CHART_VERSION).tgz" -print)
CHART_CRDS?=$(shell find $(ROOT_DIR) -type f  -name "elemental-operator-crds-$(CHART_VERSION).tgz" -print)
KUBE_VERSION?="v1.27.10"
CLUSTER_NAME?="operator-e2e"
COMMITDATE?=$(shell git log -n1 --format="%as")
GO_TPM_TAG?=$(shell grep google/go-tpm-tools go.mod | awk '{print $$2}')
E2E_CONF_FILE ?= $(ROOT_DIR)/tests/e2e/config/config.yaml

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
CONTROLLER_GEN_VER := v0.14.0
CONTROLLER_GEN := $(ABS_TOOLS_DIR)/controller-gen-$(CONTROLLER_GEN_VER)
CONTROLLER_GEN_PKG := sigs.k8s.io/controller-tools/cmd/controller-gen

GINKGO_VER := $(shell go list -m github.com/onsi/ginkgo/v2 | awk '{print $$2}')
GINKGO := $(ABS_TOOLS_DIR)/ginkgo-$(GINKGO_VER)
GINKGO_PKG := github.com/onsi/ginkgo/v2/ginkgo

SETUP_ENVTEST_VER := v0.0.0-20240213082838-4282ca1767dc
SETUP_ENVTEST := $(ABS_TOOLS_DIR)/setup-envtest-$(SETUP_ENVTEST_VER)
SETUP_ENVTEST_PKG := sigs.k8s.io/controller-runtime/tools/setup-envtest

# See: https://storage.googleapis.com/kubebuilder-tools
ENVTEST_K8S_VERSION := 1.27.1

KUSTOMIZE_VER := v5.3.0
KUSTOMIZE := $(ABS_TOOLS_DIR)/kustomize-$(KUSTOMIZE_VER)
KUSTOMIZE_PKG := sigs.k8s.io/kustomize/kustomize/v5

MOCKGEN_PKG := go.uber.org/mock/mockgen
MOCKGEN_VER := v0.4.0
MOCKGEN := $(ABS_TOOLS_DIR)/mockgen-$(MOCKGEN_VER)

$(CONTROLLER_GEN): 
	GOBIN=$(ABS_TOOLS_DIR) $(GO_INSTALL) $(CONTROLLER_GEN_PKG) controller-gen $(CONTROLLER_GEN_VER)

$(GINKGO):
	GOBIN=$(ABS_TOOLS_DIR) $(GO_INSTALL) $(GINKGO_PKG) ginkgo $(GINKGO_VER)

$(SETUP_ENVTEST): 
	GOBIN=$(ABS_TOOLS_DIR) $(GO_INSTALL) $(SETUP_ENVTEST_PKG) setup-envtest $(SETUP_ENVTEST_VER)

$(KUSTOMIZE):
	CGO_ENABLED=0 GOBIN=$(ABS_TOOLS_DIR) $(GO_INSTALL) $(KUSTOMIZE_PKG) kustomize $(KUSTOMIZE_VER)

$(MOCKGEN):
	GOBIN=$(ABS_TOOLS_DIR) $(GO_INSTALL) $(MOCKGEN_PKG) mockgen $(MOCKGEN_VER)	

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
		${DOCKER_ARGS} \
		-t ${REGISTRY_HEADER}${REPO}:${CHART_VERSION} .

.PHONY: build-docker-register
build-docker-register:
	DOCKER_BUILDKIT=1 docker build \
		-f Dockerfile \
		--target elemental-register \
		--build-arg "TAG=${GIT_TAG}" \
		--build-arg "COMMIT=${GIT_COMMIT}" \
		--build-arg "COMMITDATE=${COMMITDATE}" \
		${DOCKER_ARGS} \
		-t docker.io/local/elemental-register:dev .

.PHONY: build-docker-seedimage-builder
build-docker-seedimage-builder:
	DOCKER_BUILDKIT=1 docker build \
		-f Dockerfile.seedimage \
		${DOCKER_ARGS} \
		-t ${REGISTRY_HEADER}${REPO_SEEDIMAGE}:${TAG_SEEDIMAGE} .

.PHONY: build-docker-push-operator
build-docker-push-operator: build-docker-operator
	docker push ${REGISTRY_HEADER}${REPO}:${CHART_VERSION}

.PHONY: build-docker-push-seedimage-builder
build-docker-push-seedimage-builder: build-docker-seedimage-builder
	docker push ${REGISTRY_HEADER}${REPO_SEEDIMAGE}:${TAG_SEEDIMAGE}

.PHONY: chart
chart: build-manifests
	mkdir -p  $(ROOT_DIR)/build
	cp -rf $(ROOT_DIR)/.obs/chartfile/elemental-operator-crds-helm $(ROOT_DIR)/build/crds
	mv $(ROOT_DIR)/build/crds/_helmignore $(ROOT_DIR)/build/crds/.helmignore
	yq -i '.version = "${CHART_VERSION}"' $(ROOT_DIR)/build/crds/Chart.yaml
	yq -i '.appVersion = "${GIT_TAG}"' $(ROOT_DIR)/build/crds/Chart.yaml
	ls -R $(ROOT_DIR)/build/crds
	helm package -d $(ROOT_DIR)/build/ $(ROOT_DIR)/build/crds
	rm -Rf $(ROOT_DIR)/build/crds
	cp -rf $(ROOT_DIR)/.obs/chartfile/elemental-operator-helm $(ROOT_DIR)/build/operator
	mv $(ROOT_DIR)/build/operator/_helmignore $(ROOT_DIR)/build/operator/.helmignore
	yq -i '.version = "${CHART_VERSION}"' $(ROOT_DIR)/build/operator/Chart.yaml
	yq -i '.appVersion = "${GIT_TAG}"' $(ROOT_DIR)/build/operator/Chart.yaml
	yq -i '.annotations."catalog.cattle.io/upstream-version" = "${GIT_TAG}"' $(ROOT_DIR)/build/operator/Chart.yaml
	yq -i '.image.tag = "${CHART_VERSION}"' $(ROOT_DIR)/build/operator/values.yaml
	yq -i '.image.repository = "${REPO}"' $(ROOT_DIR)/build/operator/values.yaml
	yq -i '.seedImage.tag = "${TAG_SEEDIMAGE}"' $(ROOT_DIR)/build/operator/values.yaml
	yq -i '.seedImage.repository = "${REPO_SEEDIMAGE}"' $(ROOT_DIR)/build/operator/values.yaml
	yq -i '.channel.tag = "${TAG_CHANNEL}"' $(ROOT_DIR)/build/operator/values.yaml
	yq -i '.channel.image = "${REGISTRY_URL}/${REPO_CHANNEL}"' $(ROOT_DIR)/build/operator/values.yaml
	yq -i '.questions[0].subquestions[0].default = "${REGISTRY_URL}/${REPO_CHANNEL}"' $(ROOT_DIR)/build/operator/questions.yaml
	yq -i '.questions[0].subquestions[1].default = "${TAG_CHANNEL}"' $(ROOT_DIR)/build/operator/questions.yaml
	yq -i '.registryUrl = "${REGISTRY_URL}"' $(ROOT_DIR)/build/operator/values.yaml
# See OBS _service for reference
ifeq ($(LOCAL_BUILD),true)
	echo "Applying LOCAL_BUILD tweaks"
	sed -i 's/%%IMG_REPO%%/registry.suse.com/g' $(ROOT_DIR)/build/operator/values.yaml
	sed -i 's/%VERSION%/${GIT_TAG}/g' $(ROOT_DIR)/build/operator/values.yaml
	sed -i 's/%VERSION%/${GIT_TAG}/g' $(ROOT_DIR)/build/operator/questions.yaml
	sed -i 's/%VERSION%/${GIT_TAG}/g' $(ROOT_DIR)/build/operator/Chart.yaml
endif
	helm package -d $(ROOT_DIR)/build/ $(ROOT_DIR)/build/operator
	rm -Rf $(ROOT_DIR)/build/operator

validate:
	scripts/validate

.PHONY: unit-tests
unit-tests: $(SETUP_ENVTEST) $(GINKGO)
	KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(ABS_TOOLS_DIR) -p path)" $(GINKGO) -v -r --trace --race --covermode=atomic --coverprofile=coverage.out --coverpkg=github.com/rancher/elemental-operator/... ./pkg/... ./controllers/... ./cmd/...

.PHONY: airgap-script-test
airgap-script-test:
	scripts/elemental-airgap-test.sh

e2e-tests: $(GINKGO) build-docker-operator build-docker-seedimage-builder chart setup-kind
	kubectl cluster-info --context kind-$(CLUSTER_NAME)
	kubectl label nodes --all --overwrite ingress-ready=true
	kubectl label nodes --all --overwrite node-role.kubernetes.io/master=
	kubectl get nodes -o wide
	export EXTERNAL_IP=$$(kubectl get nodes -o jsonpath='{.items[].status.addresses[?(@.type == "InternalIP")].address}') && \
	export CHART=$(CHART) && \
	export BRIDGE_IP="172.18.0.1" && \
	export CONFIG_PATH=$(E2E_CONF_FILE) && \
	kind load docker-image --name $(CLUSTER_NAME) ${REGISTRY_HEADER}${REPO}:${CHART_VERSION} && \
	kind load docker-image --name $(CLUSTER_NAME) ${REGISTRY_HEADER}${REPO_SEEDIMAGE}:${TAG_SEEDIMAGE} && \
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
setup-full-cluster: $(GINKGO) build-docker-operator build-docker-seedimage-builder chart setup-kind
	@export EXTERNAL_IP=$$(kubectl get nodes -o jsonpath='{.items[].status.addresses[?(@.type == "InternalIP")].address}') && \
	export BRIDGE_IP="172.18.0.1" && \
	export CHART=$(CHART) && \
	export CONFIG_PATH=$(E2E_CONF_FILE) && \
	kind load docker-image --name $(CLUSTER_NAME) ${REGISTRY_HEADER}${REPO}:${CHART_VERSION} && \
	kind load docker-image --name $(CLUSTER_NAME) ${REGISTRY_HEADER}${REPO_SEEDIMAGE}:${TAG_SEEDIMAGE} && \
	cd $(ROOT_DIR)/tests && $(GINKGO) -r -v --label-filter="do-nothing" ./e2e
	$(ROOT_DIR)/scripts/install-ipam-provider.sh

# This generates the chart, builds the docker image, loads the image into the kind cluster and upgrades the chart to latest
# useful to test changes into the operator with a running system, without clearing the operator namespace
# thus losing any registration/inventories/os CRDs already created
reload-operator: chart build-docker-operator
	kind load docker-image --name $(CLUSTER_NAME) ${REGISTRY_HEADER}${REPO}:${CHART_VERSION}
	helm upgrade -n cattle-elemental-system elemental-operator-crds $(CHART_CRDS)
	helm upgrade -n cattle-elemental-system elemental-operator $(CHART)
	kubectl -n cattle-elemental-system rollout restart deployment/elemental-operator

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

.PHONY: generate-mocks
generate-mocks: $(MOCKGEN)
	$(MOCKGEN) -copyright_file=scripts/boilerplate.go.txt -destination=pkg/register/mocks/client.go -package=mocks github.com/rancher/elemental-operator/pkg/register Client
	$(MOCKGEN) -copyright_file=scripts/boilerplate.go.txt -destination=pkg/register/mocks/state.go -package=mocks github.com/rancher/elemental-operator/pkg/register StateHandler
	$(MOCKGEN) -copyright_file=scripts/boilerplate.go.txt -destination=pkg/install/mocks/install.go -package=mocks github.com/rancher/elemental-operator/pkg/install Installer
	$(MOCKGEN) -copyright_file=scripts/boilerplate.go.txt -destination=pkg/upgrade/mocks/upgrade.go -package=mocks github.com/rancher/elemental-operator/pkg/upgrade Upgrader
	$(MOCKGEN) -copyright_file=scripts/boilerplate.go.txt -destination=pkg/elementalcli/mocks/elementalcli.go -package=mocks github.com/rancher/elemental-operator/pkg/elementalcli Runner
	$(MOCKGEN) -copyright_file=scripts/boilerplate.go.txt -destination=pkg/network/mocks/network.go -package=mocks github.com/rancher/elemental-operator/pkg/network Configurator
	$(MOCKGEN) -copyright_file=scripts/boilerplate.go.txt -destination=pkg/util/mocks/command_runner.go -package=mocks github.com/rancher/elemental-operator/pkg/util CommandRunner
	$(MOCKGEN) -copyright_file=scripts/boilerplate.go.txt -destination=pkg/util/mocks/nsenter.go -package=mocks github.com/rancher/elemental-operator/pkg/util NsEnter

.PHONY: generate-go
generate-go: generate-mocks $(CONTROLLER_GEN) ## Runs Go related generate targets for the operator
	$(CONTROLLER_GEN) \
		object:headerFile=$(ROOT)scripts/boilerplate.go.txt \
		paths=./api/...

build-crds: $(KUSTOMIZE)
	$(KUSTOMIZE) build config/crd > .obs/chartfile/elemental-operator-crds-helm/templates/crds.yaml

build-rbac: $(KUSTOMIZE)
	$(KUSTOMIZE) build config/rbac > .obs/chartfile/elemental-operator-helm/templates/rbac.yaml

build-manifests: $(KUSTOMIZE) generate
	$(MAKE) build-crds
	$(MAKE) build-rbac

ALL_VERIFY_CHECKS = manifests vendor generate

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

.PHONY: verify-generate
verify-generate: generate
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi

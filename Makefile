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
COMMITDATE?=$(shell date -d @"${RAWCOMMITDATE}" "+%FT%TZ")

LDFLAGS := -w -s
LDFLAGS += -X "github.com/rancher/elemental-operator/pkg/version.Version=${GIT_TAG}"
LDFLAGS += -X "github.com/rancher/elemental-operator/pkg/version.Commit=${GIT_COMMIT}"
LDFLAGS += -X "github.com/rancher/elemental-operator/pkg/version.CommitDate=${COMMITDATE}"

.PHONY: build
build: operator register

.PHONY: operator
operator:
	go build -ldflags '$(LDFLAGS)' -o build/elemental-operator $(ROOT_DIR)/cmd/operator


.PHONE: register
register:
	go build -ldflags '$(LDFLAGS)' -o build/elemental-register $(ROOT_DIR)/cmd/register


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

unit-tests-deps:
	go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo@latest
	go install github.com/onsi/gomega/...

.PHONY: unit-tests
unit-tests: unit-tests-deps
	ginkgo -r -v  --covermode=atomic --coverprofile=coverage.out -p -r ./pkg/...

e2e-tests: setup-kind
	export EXTERNAL_IP=`kubectl get nodes -o jsonpath='{.items[].status.addresses[?(@.type == "InternalIP")].address}'` && \
	export BRIDGE_IP="172.18.0.1" && \
	cd $(ROOT_DIR)/tests && ginkgo -r -v ./e2e

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
	cd $(ROOT_DIR)/tests && ginkgo -r -v --label-filter="do-nothing" ./e2e

kind-e2e-tests: build-docker-operator chart setup-kind
	kind load docker-image --name $(CLUSTER_NAME) ${REPO}:${TAG}
	CHART=$(CHART) $(MAKE) e2e-tests

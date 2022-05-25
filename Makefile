GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT?=$(shell git rev-parse --short HEAD)
GIT_TAG?=$(shell git describe --abbrev=0 --tags 2>/dev/null || echo "v0.0.0" )
TAG?=${GIT_TAG}-${GIT_COMMIT_SHORT}
REPO?=quay.io/costoolkit/elemental-operator-ci
export ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
ROS_CHART?=$(shell find $(ROOT_DIR) -type f  -name "elemental-operator*.tgz" -print)
CHART_VERSION?=$(subst v,,$(GIT_TAG))
KUBE_VERSION?="v1.22.7"
CLUSTER_NAME?="ros-e2e"

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags "-extldflags -static -s" -o build/elemental-operator

.PHONY: build-docker
build-docker:
	DOCKER_BUILDKIT=1 docker build \
		--target ros-operator \
		-t ${REPO}:${TAG} .

.PHONY: build-docker-push
build-docker-push: build-docker
	docker push ${REPO}:${TAG}

.PHONY: chart
chart:
	mkdir -p  $(ROOT_DIR)/build
	cp -rf $(ROOT_DIR)/chart $(ROOT_DIR)/build/chart
	sed -i -e 's/tag:.*/tag: '${TAG}'/' $(ROOT_DIR)/build/chart/values.yaml
	sed -i -e 's|repository:.*|repository: '${REPO}'|' $(ROOT_DIR)/build/chart/values.yaml
	helm package --version ${CHART_VERSION} --app-version ${GIT_TAG} -d $(ROOT_DIR)/build/ $(ROOT_DIR)/build/chart
	rm -Rf $(ROOT_DIR)/build/chart

.PHONY: test_deps
test_deps:
	go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo
	go install github.com/onsi/gomega/...

.PHONY: unit-tests
unit-tests: test_deps
	ginkgo -r -v  --covermode=atomic --coverprofile=coverage.out -p -r ./pkg/...

e2e-tests:
	KUBE_VERSION=${KUBE_VERSION} $(ROOT_DIR)/scripts/e2e-tests.sh

kind-e2e-tests: build-docker chart
	kind load docker-image --name $(CLUSTER_NAME) ${REPO}:${TAG}
	ROS_CHART=${ROOT_DIR}/build/elemental-operator-${CHART_VERSION}.tgz $(MAKE) e2e-tests
GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT?=$(shell git rev-parse --short HEAD)
GIT_TAG?=$(shell git describe --abbrev=0 --tags 2>/dev/null || echo "v0.0.0" )
TAG?=${GIT_TAG}-${GIT_COMMIT_SHORT}
REPO?=quay.io/costoolkit/rancheros-operator-ci
ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
ROS_CHART?=$(shell find $(ROOT_DIR) -type f  -name "rancheros-operator*.tgz" -print)
CHART_VERSION?=$(subst v,,$(GIT_TAG))

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags "-extldflags -static -s" -o build/rancheros-operator

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
	go install github.com/onsi/ginkgo/v2/ginkgo
	go install github.com/onsi/gomega/...

.PHONY: test_vm_up
test_vm_up:
	cd $(ROOT_DIR)/tests && vagrant up

.PHONY: test_vm_down
test_vm_down:
	cd $(ROOT_DIR)/tests && vagrant destroy -f

.PHONY: integration-tests
integration-tests: test_vm_up
	cd $(ROOT_DIR)/tests && ginkgo -r -v ./smoke

.PHONY: unit-tests
unit-tests: test_deps
	ginkgo -r -v  --covermode=atomic --coverprofile=coverage.out -p -r ./pkg/...

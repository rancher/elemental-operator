GIT_COMMIT?=$(shell git rev-parse HEAD)
GIT_COMMIT_SHORT?=$(shell git rev-parse --short HEAD)
GIT_TAG?=$(shell git describe --abbrev=0 --tags 2>/dev/null || echo "v0.0.0" )
TAG?=${GIT_TAG}
REPO?=rancher/elemental-operator
export ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
CHART?=$(shell find $(ROOT_DIR) -type f  -name "elemental-operator*.tgz" -print)
CHART_VERSION?=$(subst v,,$(GIT_TAG))

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags "-extldflags -static -s -X 'github.com/rancher/elemental-operator/version.Version=$TAG'" -o build/elemental-operator

.PHONY: build-docker
build-docker:
	DOCKER_BUILDKIT=1 docker build \
		-f package/Dockerfile \
		--target elemental-operator \
		-t ${REPO}:${TAG} .

.PHONY: build-docker-push
build-docker-push: build-docker
	docker push ${REPO}:${TAG}

.PHONY: chart
chart:
	mkdir -p  $(ROOT_DIR)/build
	helm package --version ${CHART_VERSION} --app-version ${GIT_TAG} -d $(ROOT_DIR)/build/ $(ROOT_DIR)/chart

validate:
	scripts/validate

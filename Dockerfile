FROM golang:1.17.12-alpine as build
WORKDIR /src
COPY go.mod go.sum /src/
RUN go mod download
COPY pkg /src/pkg
COPY cmd /src/cmd
# Set arg/env after go mod download, otherwise we invalidate the cached layers due to the commit changing easily
ARG TAG=v0.0.0
ARG COMMIT=""
ARG COMMITDATE=""
ENV CGO_ENABLED=1
RUN apk add gcc openssl-dev musl-dev
RUN go build \
    -ldflags "-w -s \
    -X github.com/rancher/elemental-operator/pkg/version.Version=$TAG \
    -X github.com/rancher/elemental-operator/pkg/version.Commit=$COMMIT \
    -X github.com/rancher/elemental-operator/pkg/version.CommitDate=$COMMITDATE" \
    -o /usr/sbin/elemental-operator /src/cmd/operator
RUN /usr/sbin/elemental-operator -h


FROM alpine as elemental-operator
# Invalidate the copy on each commit, otherwise it may use the cache!
ARG TAG=v0.0.0
ARG COMMIT=""
ARG COMMITDATE=""
COPY --from=build /usr/sbin/elemental-operator /usr/sbin/elemental-operator
ENTRYPOINT ["/usr/sbin/elemental-operator"]
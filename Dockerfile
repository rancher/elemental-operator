FROM golang:1.17-alpine as build
ENV CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum /src/
RUN go mod download
COPY cmd/operator/main.go /src/
COPY pkg /src/pkg
COPY cmd/operator /src/cmd/operator
COPY cmd/register /src/cmd/register
# Set arg/env after go mod download, otherwise we invalidate the cached layers due to the commit changing easily
ARG TAG=v0.0.0
ARG COMMIT=""
ARG COMMITDATE=""
RUN go build  \
    -ldflags "-w -s  \
    -X github.com/rancher/elemental-operator/pkg/version.Version=$TAG  \
    -X github.com/rancher/elemental-operator/pkg/version.Commit=$COMMIT  \
    -X github.com/rancher/elemental-operator/pkg/version.CommitDate=$COMMITDATE"  \
    -o /usr/sbin/elemental-operator ./cmd/operator

RUN go build  \
    -ldflags "-w -s  \
    -X github.com/rancher/elemental-operator/pkg/version.Version=$TAG  \
    -X github.com/rancher/elemental-operator/pkg/version.Commit=$COMMIT  \
    -X github.com/rancher/elemental-operator/pkg/version.CommitDate=$COMMITDATE"  \
    -o /usr/sbin/elemental-register ./cmd/register


FROM scratch as elemental-operator
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /usr/sbin/elemental-operator /usr/sbin/elemental-operator
ENTRYPOINT ["/usr/sbin/elemental-operator"]

FROM scratch as elemental-register
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /usr/sbin/elemental-register /usr/sbin/elemental-register
ENTRYPOINT ["/usr/sbin/elemental-register"]

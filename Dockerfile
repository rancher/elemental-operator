FROM registry.suse.com/bci/golang:1.18 AS build
RUN zypper -n install -l openssl-devel
WORKDIR /src
COPY go.mod go.sum /src/
RUN go mod download
COPY cmd/operator/main.go /src/
COPY api /src/api
COPY pkg /src/pkg
COPY cmd/operator /src/cmd/operator
COPY cmd/register /src/cmd/register
COPY cmd/support /src/cmd/support
# Set arg/env after go mod download, otherwise we invalidate the cached layers due to the commit changing easily

FROM build AS build-operator
ARG TAG=v0.0.0
ARG COMMIT=""
ARG COMMITDATE=""
ENV CGO_ENABLED=0
RUN go build  \
    -ldflags "-w -s  \
    -X github.com/rancher/elemental-operator/pkg/version.Version=$TAG  \
    -X github.com/rancher/elemental-operator/pkg/version.Commit=$COMMIT  \
    -X github.com/rancher/elemental-operator/pkg/version.CommitDate=$COMMITDATE"  \
    -o /usr/sbin/elemental-operator ./cmd/operator

FROM build AS build-register
ARG TAG=v0.0.0
ARG COMMIT=""
ARG COMMITDATE=""
ENV CGO_ENABLED=1
RUN go build  \
    -ldflags "-w -s  \
    -X github.com/rancher/elemental-operator/pkg/version.Version=$TAG  \
    -X github.com/rancher/elemental-operator/pkg/version.Commit=$COMMIT  \
    -X github.com/rancher/elemental-operator/pkg/version.CommitDate=$COMMITDATE"  \
    -o /usr/sbin/elemental-register ./cmd/register
ENV CGO_ENABLED=0
RUN go build  \
    -ldflags "-w -s  \
    -X github.com/rancher/elemental-operator/pkg/version.Version=$TAG  \
    -X github.com/rancher/elemental-operator/pkg/version.Commit=$COMMIT  \
    -X github.com/rancher/elemental-operator/pkg/version.CommitDate=$COMMITDATE"  \
    -o /usr/sbin/elemental-support ./cmd/support


FROM scratch AS elemental-operator
COPY --from=build /var/lib/ca-certificates/ca-bundle.pem /etc/ssl/certs/ca-certificates.crt
COPY --from=build-operator /usr/sbin/elemental-operator /usr/sbin/elemental-operator
ENTRYPOINT ["/usr/sbin/elemental-operator"]

FROM scratch AS elemental-register
COPY --from=build /var/lib/ca-certificates/ca-bundle.pem /etc/ssl/certs/ca-certificates.crt
COPY --from=build-register /usr/sbin/elemental-register /usr/sbin/elemental-register
COPY --from=build-register /usr/sbin/elemental-support /usr/sbin/elemental-support
ENTRYPOINT ["/usr/sbin/elemental-register"]

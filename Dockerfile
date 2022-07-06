FROM golang:1.17-alpine as build
ENV CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum /src/
RUN go mod download
COPY main.go /src/
COPY pkg /src/pkg
COPY cmd /src/cmd
# Set arg/env after go mod download, otherwise we invalidate the cached layers due to the commit changing easily
ARG VERSION=0.0.0
ENV VERSION=${VERSION}
RUN go build -ldflags "-extldflags -static -s -X github.com/rancher/elemental-operator/version.Version=$VERSION" -o /usr/sbin/elemental-operator

FROM scratch as elemental-operator
COPY --from=build /usr/sbin/elemental-operator /usr/sbin/elemental-operator
ENTRYPOINT ["/usr/sbin/elemental-operator"]
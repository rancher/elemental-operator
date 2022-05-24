FROM golang:1.17-alpine as build
WORKDIR /src
COPY go.mod go.sum /src/
RUN go mod download
COPY main.go /src/
COPY cmd /src/cmd
COPY pkg /src/pkg
RUN CGO_ENABLED=0 go build -ldflags "-extldflags -static -s" -o /usr/bin/elemental-operator

FROM registry.suse.com/suse/sle15:15.3 as elemental-operator
COPY --from=build /usr/bin/elemental-operator /usr/bin/elemental-operator

ENTRYPOINT ["elemental-operator"]
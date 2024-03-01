################################################################################

ARG BUILD_TARGET=producer # one of {"producer", "aggregator", "consumer"}

################################################################################

FROM golang:alpine AS base-producer
ENV CGO_ENABLED 0
ENV GOOS linux
RUN apk update --no-cache
WORKDIR /build
COPY producer/go.mod producer/go.sum ./
RUN go mod download && go mod verify
COPY producer/ .
RUN ls
RUN go build -v -ldflags="-s -w" -o /usr/local/bin/app ./cmd/main.go

################################################################################

FROM golang:alpine AS base-aggregator
ENV CGO_ENABLED 0
ENV GOOS linux
RUN apk update --no-cache
WORKDIR /build
COPY aggregator/go.mod aggregator/go.sum ./
RUN go mod download && go mod verify
COPY aggregator/ .
RUN go build -v -ldflags="-s -w" -o /usr/local/bin/app ./cmd/main.go

################################################################################

FROM golang:alpine AS base-consumer
ENV CGO_ENABLED 0
ENV GOOS linux
RUN apk update --no-cache
WORKDIR /build
COPY consumer/go.mod consumer/go.sum ./
RUN go mod download && go mod verify
COPY consumer/ .
RUN go build -v -ldflags="-s -w" -o /usr/local/bin/app ./cmd/main.go

################################################################################

FROM base-${BUILD_TARGET} AS base

FROM alpine:3.18.6 AS app-release
COPY --from=base /usr/local/bin/app /usr/local/bin/app

################################################################################

FROM app-release AS app
WORKDIR /
CMD ["app"]
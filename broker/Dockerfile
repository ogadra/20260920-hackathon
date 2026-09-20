# checkov:skip=CKV_DOCKER_2:health check is managed by compose/orchestrator
FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build
WORKDIR /app
COPY broker/go.mod broker/go.sum ./
RUN go mod download
COPY broker/ ./

FROM build AS test
SHELL ["/bin/ash", "-o", "pipefail", "-c"]
RUN CGO_ENABLED=0 go test -v -coverpkg=./... -coverprofile=cover.out ./... \
    && total=$(go tool cover -func=cover.out | grep ^total: | awk '{print $NF}') \
    && echo "Total coverage: ${total}" \
    && test "${total}" = "100.0%" || { echo "FAIL: coverage ${total} is below 100.0%"; exit 1; }

# integration test stage
FROM build AS test-integration
CMD ["go", "test", "-v", "-count=1", "./..."]

# test with race detector (requires cgo + gcc)
FROM build AS test-race
RUN apk add --no-cache gcc musl-dev \
    && CGO_ENABLED=1 go test -race -v ./...

# binary build
# not FROM build: that stage runs on the target platform, which puts the compiler under QEMU
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS binary
WORKDIR /app
COPY broker/go.mod broker/go.sum ./
RUN go mod download
COPY broker/ ./
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /server .

# production image
FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
RUN adduser -D -H -s /sbin/nologin appuser
RUN apk add --no-cache bash ca-certificates curl
COPY --from=binary /server /server
USER appuser
EXPOSE 8080
CMD ["/server"]

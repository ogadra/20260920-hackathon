# checkov:skip=CKV_DOCKER_2:health check is managed by compose/orchestrator
FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build
WORKDIR /app
COPY broker/go.mod broker/go.sum ./
RUN go mod download
COPY broker/ ./

# broker/store/firestoreadapter は Firestore SDK 具象型を直接触るため unit test で mock 不能。
# integration test で担保するため -coverpkg から除外する。

FROM build AS test
SHELL ["/bin/ash", "-o", "pipefail", "-c"]
RUN COVERPKG=$(go list ./... | grep -v /firestoreadapter | tr '\n' ',' | sed 's/,$//') \
    && CGO_ENABLED=0 go test -v -coverpkg="$COVERPKG" -coverprofile=cover.out ./... \
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
ARG TARGETARCH
RUN adduser -D -H -s /sbin/nologin appuser
COPY broker/apk-checksums.${TARGETARCH}.sha256 /tmp/apks.sha256
RUN set -euxo pipefail; \
    mkdir -p /tmp/apks; \
    apk update; \
    apk fetch --recursive -o /tmp/apks \
        bash=5.3.9-r1 \
        ca-certificates=20260611-r0 \
        curl=8.22.0-r0; \
    apk verify /tmp/apks/*.apk; \
    find /tmp/apks -type f -name '*.apk' -exec basename {} \; | sort > /tmp/apks.actual; \
    awk '{print $2}' /tmp/apks.sha256 | sort > /tmp/apks.expected; \
    diff -u /tmp/apks.expected /tmp/apks.actual; \
    (cd /tmp/apks && sha256sum -c /tmp/apks.sha256); \
    apk --network=no add --no-cache /tmp/apks/*.apk; \
    rm -rf /tmp/apks /tmp/apks.actual /tmp/apks.expected /tmp/apks.sha256 /var/cache/apk/*
COPY --from=binary /server /server
USER appuser
EXPOSE 8080
CMD ["/server"]

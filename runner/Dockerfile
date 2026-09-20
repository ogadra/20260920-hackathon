# checkov:skip=CKV_DOCKER_3:runner executes user commands and requires root
# checkov:skip=CKV_DOCKER_2:health check is managed by compose/orchestrator
FROM golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36 AS build
WORKDIR /app
COPY runner/go.mod runner/go.sum ./
RUN go mod download
COPY runner/*.go ./

# test stage
FROM build AS test
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN CGO_ENABLED=0 go test -v -coverprofile=cover.out -timeout=30s ./... \
    && total=$(go tool cover -func=cover.out | grep ^total: | awk '{print $NF}') \
    && echo "Total coverage: ${total}" \
    && test "${total}" = "100.0%" || { echo "FAIL: coverage ${total} is below 100.0%"; exit 1; }

# race detection stage
FROM build AS test-race
RUN CGO_ENABLED=1 go test -race -v -count=1 ./...

# binary build
FROM build AS binary
RUN CGO_ENABLED=0 go build -o /server .

# The full image rather than -slim: perl, the standard dpkg tooling and the
# documentation paths are already there, so `apt-get install` behaves the way
# it does on a normal box. The apt lists are dropped so a runtime
# `apt-get update` fetches current indexes instead of replaying a stale
# snapshot from build time.
FROM debian:bookworm@sha256:f37a335e82bca302e955fa39f9dfe28f1be618f016f8a2b56318e5a5111afc26

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        curl \
        dnsutils \
        iproute2 \
        iputils-ping \
        jq \
        procps \
        python3 \
        wget \
    && rm -rf /var/lib/apt/lists/*

COPY --from=binary /server /server
EXPOSE 3000
CMD ["/server"]

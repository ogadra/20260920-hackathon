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

FROM debian:bookworm-slim@sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251

# The slim image ships no ca-certificates package, and the runner needs a trust
# store to reach the Jev API over TLS. Taking the bundle from the build stage
# keeps the final image free of an apt transaction.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=binary /server /server
EXPOSE 3000
CMD ["/server"]

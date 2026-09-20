# checkov:skip=CKV_DOCKER_2:test-only container, no health check needed
FROM golang:1.26-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83
RUN adduser -D -s /sbin/nologin appuser
WORKDIR /app
COPY integration/go.mod ./
RUN go mod download
COPY integration/ ./
USER appuser
CMD ["go", "test", "-v", "-count=1", "-tags=integration", "-timeout=120s", "./..."]

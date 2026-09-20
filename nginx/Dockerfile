# checkov:skip=CKV_DOCKER_2:health check is not needed for the front build stage
FROM node:26-slim AS front-build
# Go through corepack instead of installing pnpm directly.
# That keeps the pnpm version pinned by package.json's packageManager.
RUN npm install -g corepack@0.36.0 && corepack enable
WORKDIR /app
COPY front/package.json front/pnpm-lock.yaml ./
RUN chown -R node:node /app
USER node
RUN pnpm install --frozen-lockfile
COPY --chown=node:node front/ ./
RUN pnpm build

# checkov:skip=CKV_DOCKER_2:health check is not needed for config test stage
FROM openresty/openresty:1.31.1.1-alpine@sha256:e260a5b63b78700eb8047e934950a17b4ab3980a77ef254ec486fff8ea4617e6 AS config-test
RUN apk add --no-cache ca-certificates gettext
COPY nginx/errors.conf /etc/nginx/errors.conf
COPY nginx/security-headers.conf /etc/nginx/security-headers.conf
COPY nginx/lua/ /usr/local/openresty/nginx/lua/
COPY nginx/nginx.conf /etc/nginx/nginx.conf.template
ARG NGINX_RESOLVER=127.0.0.11
ARG BROKER_HOST=broker
RUN envsubst '${NGINX_RESOLVER} ${BROKER_HOST}' < /etc/nginx/nginx.conf.template > /usr/local/openresty/nginx/conf/nginx.conf \
    && STACK_NAME=config-test INTERNAL_DOMAIN=config-test.invalid BUNSHIN_STACKS=config-test RUNNER_API_PORT=3000 openresty -t \
    && for t in /usr/local/openresty/nginx/lua/*_test.lua; do /usr/local/openresty/luajit/bin/luajit "$t" || exit 1; done \
    && rm -f /tmp/nginx.pid
# USER is added to satisfy trivy DS-0002; openresty -t requires root so this must come after RUN
USER nobody

# checkov:skip=CKV_DOCKER_2:health check is managed by ECS task definition / Kubernetes probes
FROM openresty/openresty:1.31.1.1-alpine@sha256:e260a5b63b78700eb8047e934950a17b4ab3980a77ef254ec486fff8ea4617e6
# html/はrootとして公開するので、openresty同梱の50x.htmlごと捨ててdistだけを置く。
RUN apk add --no-cache ca-certificates gettext \
    && adduser -D -H -s /sbin/nologin nginx \
    && rm -rf /usr/local/openresty/nginx/html \
    && chown -R nginx:nginx /usr/local/openresty/nginx /run
COPY --chown=nginx:nginx nginx/errors.conf /etc/nginx/errors.conf
COPY --chown=nginx:nginx nginx/security-headers.conf /etc/nginx/security-headers.conf
COPY --chown=nginx:nginx nginx/lua/ /usr/local/openresty/nginx/lua/
COPY --chown=nginx:nginx nginx/nginx.conf /etc/nginx/nginx.conf.template
COPY --from=front-build --chown=nginx:nginx /app/dist/ /usr/local/openresty/nginx/html/
RUN rm -f /usr/local/openresty/nginx/lua/*_test.lua
USER nginx
CMD ["/bin/sh", "-c", "envsubst '${NGINX_RESOLVER} ${BROKER_HOST}' < /etc/nginx/nginx.conf.template > /usr/local/openresty/nginx/conf/nginx.conf && exec openresty -g 'daemon off;'"]

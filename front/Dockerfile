# checkov:skip=CKV_DOCKER_2:health check is managed by compose/orchestrator
FROM node:24-slim
RUN corepack enable
WORKDIR /app
COPY front/package.json front/pnpm-lock.yaml ./
RUN chown -R node:node /app
USER node
RUN pnpm install --frozen-lockfile
COPY --chown=node:node front/ ./
EXPOSE 5173
CMD ["pnpm", "dev", "--host", "0.0.0.0"]

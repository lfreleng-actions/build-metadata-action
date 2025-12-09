# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2025 The Linux Foundation

FROM node:18-alpine AS builder

LABEL org.opencontainers.image.created="2025-01-15T12:00:00Z"
LABEL org.opencontainers.image.version="1.2.3"
LABEL org.opencontainers.image.title="Example App"
LABEL org.opencontainers.image.description="A complete example Docker image"
LABEL org.opencontainers.image.authors="DevTeam <dev@example.com>"
LABEL org.opencontainers.image.source="https://github.com/example/app"
LABEL org.opencontainers.image.url="https://example.com"
LABEL org.opencontainers.image.licenses="Apache-2.0"

ARG NODE_VERSION=18
ARG BUILD_DATE

WORKDIR /build

COPY package*.json ./
RUN npm ci --only=production && \
    npm cache clean --force

COPY . .
RUN npm run build

FROM node:18-alpine AS runtime

ENV NODE_ENV=production
ENV PORT=3000
ENV LOG_LEVEL=info

WORKDIR /app

COPY --from=builder /build/dist ./dist
COPY --from=builder /build/node_modules ./node_modules

RUN addgroup -g 1001 -S nodejs && \
    adduser -S nodejs -u 1001

USER nodejs

EXPOSE 3000
EXPOSE 9090

VOLUME ["/app/data", "/app/logs"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD node healthcheck.js

ENTRYPOINT ["node"]
CMD ["dist/index.js"]

FROM node:24-bookworm-slim AS build
WORKDIR /app
COPY package.json package-lock.json .npmrc ./
RUN npm ci
COPY . .
RUN npm run build

FROM golang:1.27-bookworm AS triage-build
WORKDIR /src/triage
COPY triage/ ./
RUN CGO_ENABLED=0 go build -o /out/triage ./cmd/triage

FROM node:24-bookworm-slim
ENV NODE_ENV=production \
    HOST=0.0.0.0 \
    PORT=3000 \
    TRIAGE_DB_PATH=/data/triage.sqlite \
    GRAPH_CACHE_TTL_MS=60000
WORKDIR /app
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY package.json package-lock.json .npmrc ./
RUN npm ci --omit=dev && npm cache clean --force
COPY --from=build /app/build ./build
COPY --from=triage-build /out/triage ./triage/bin/triage
COPY scripts ./scripts
COPY mcp ./mcp
RUN chmod -R a+rX /app \
 && mkdir -p /data \
 && chown node:node /data
USER node
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD node -e "fetch('http://127.0.0.1:3000/api/status').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"
CMD ["node", "build"]

# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS frontend-build

WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
# HeroUI Pro is distributed through a private setup flow. The key is mounted
# only for this build step and is never persisted in an image layer.
RUN --mount=type=secret,id=heroui_key,target=/run/secrets/heroui_key,required=true \
    HEROUI_KEY="$(cat /run/secrets/heroui_key)" \
    npx -y hpsetup@latest react --auto
RUN npm run build

FROM golang:1.22-alpine AS backend-build

WORKDIR /src/backend
ARG TARGETOS
ARG TARGETARCH
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/napnotifier .

FROM nginx:1.27-alpine

RUN apk add --no-cache ca-certificates
COPY --from=frontend-build /src/frontend/dist/ /usr/share/nginx/html/
COPY --from=backend-build /out/napnotifier /usr/local/bin/napnotifier
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY docker/entrypoint.sh /usr/local/bin/group-monitor-entrypoint
RUN chmod 0755 /usr/local/bin/group-monitor-entrypoint \
    && mkdir -p /data \
    && chown -R nginx:nginx /data

ENV NAP_ADDR=127.0.0.1:8787 \
    NAP_CONFIG=/data/config.json

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/auth/status >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/group-monitor-entrypoint"]

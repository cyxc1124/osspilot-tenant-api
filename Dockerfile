# osspilot-tenant-api — migrate / api / worker 同一镜像
FROM golang:1.26-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM alpine:3.22 AS runtime

ARG GIT_TAG=""
ARG GIT_COMMIT=""

RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 app

WORKDIR /app
COPY --from=builder /out/api /out/migrate /out/worker /app/
COPY --chmod=755 deploy/docker-entrypoint.sh /docker-entrypoint.sh

LABEL org.opencontainers.image.source=https://github.com/cyxc1124/osspilot-tenant-api
LABEL org.opencontainers.image.description="OssPilot 租户 API"
LABEL org.opencontainers.image.title="osspilot-tenant-api"
LABEL org.opencontainers.image.vendor="cyxc1124"
LABEL org.opencontainers.image.version=${GIT_TAG}
LABEL org.opencontainers.image.revision=${GIT_COMMIT}

USER app
EXPOSE 8000

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["api"]

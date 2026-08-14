FROM node:22-alpine AS frontend-build
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm config set registry https://registry.npmmirror.com \
    && npm ci --no-audit --no-fund --fetch-timeout=120000 --fetch-retries=2
COPY frontend ./
RUN npm run typecheck && npm run build

FROM golang:1.23-alpine AS backend-build
ARG GOPROXY=https://proxy.golang.org,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN GOPROXY="$GOPROXY" go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go test -p 1 ./... \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /xing-shu ./cmd/router

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend-build /xing-shu /app/xing-shu
COPY --from=frontend-build /src/web-dist /app/web
ENV TZ=Asia/Shanghai LISTEN=:12100
EXPOSE 12100
ENTRYPOINT ["/app/xing-shu"]

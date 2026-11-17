# VouchChain 社区版 · 多阶段构建（Docker Compose 一键启动）
#
# 产物：单二进制 vouchchain + web/dist 旁置（与本地分发一致，未 go:embed）
# 铁律：CGO_ENABLED=0（跨平台静态二进制）；web/dist 由 Stage1 在镜像内构建，无需宿主机预装 Node
#
# Stage 1: 前端构建（web/dist）
FROM node:22-alpine AS web
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# Stage 2: Go 后端（CGO_ENABLED=0 单二进制）
FROM golang:1.27-alpine AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn GOSUMDB=off CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 把 Stage1 的 web/dist 拷进构建上下文，供 go build 时旁置
COPY --from=web /app/dist /src/web/dist
RUN go build -trimpath -o /out/vouchchain .

# Stage 3: 运行时（alpine 精简运行时，非 root 用户）
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 1000 app \
    && mkdir -p /app/config /app/data
WORKDIR /app
COPY --from=build /out/vouchchain /app/vouchchain
COPY --from=web /app/dist /app/web/dist
# 运行目录需含 config/config.yaml；link 会被 VOUCHCHAIN_DB_LINK 环境变量覆盖（占位符无碍）
COPY config/config.yaml.example /app/config/config.yaml
USER app
EXPOSE 8000
ENTRYPOINT ["/app/vouchchain"]

# 不写 `# syntax=docker/dockerfile:1`：显式指定会让 BuildKit 额外去拉 docker/dockerfile 镜像，
# 在国内加速器（如飞牛的 docker.fnnas.com）上可能直接 401 Unauthorized；
# 而下面用到的 RUN --mount=type=cache 由 Docker 内建的 BuildKit frontend 已支持，无需显式指定。

ARG TARGETARCH
# 版本单一来源：根目录 VERSION 文件，本地改版本号只需改此文件
ARG VERSION_FILE=VERSION

# 本地项目：基础镜像默认走国内镜像源（fnOS 等环境直连 Docker Hub 会被加速器拦截 401）。
# 海外/可直连环境可用 --build-arg 或 .env 覆盖回官方名：NODE_IMAGE=node:22-alpine 等
ARG NODE_IMAGE=docker.m.daocloud.io/library/node:22-alpine
ARG GO_IMAGE=docker.m.daocloud.io/library/golang:1.23-bookworm
ARG RUNTIME_IMAGE=docker.m.daocloud.io/library/debian:bookworm-slim
ARG UPDATER_IMAGE=docker.m.daocloud.io/library/docker:27-cli
# Go 依赖代理：国内默认 goproxy.cn（官方 proxy.golang.org 国内不可达），海外可覆盖回 direct
ARG GOPROXY=https://goproxy.cn,direct

# 版本注入 stage：把根目录 VERSION 文件载入构建图，供前后端构建 stage 读取
FROM ${NODE_IMAGE} AS version
ARG VERSION_FILE
COPY ${VERSION_FILE} /VERSION

FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS web-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY web/ ./
COPY --from=version /VERSION /src/web/.version
RUN VITE_APP_VERSION=$(tr -d ' \t\n\r' < /src/web/.version) npm run build

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS go-builder
ARG TARGETARCH
ARG GOPROXY
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=version /VERSION ./VERSION
# 先交叉编译 havline-agent 到 embed 目录（替换仓库中的占位文件），主程序构建时自动内嵌
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o internal/frp/agentbin/havline-agent-linux-amd64 ./cmd/havline-agent \
    && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o internal/frp/agentbin/havline-agent-linux-arm64 ./cmd/havline-agent
COPY --from=web-builder /src/web/dist ./cmd/havline/web/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags "-X github.com/havline/havline/internal/version.Version=$(cat /src/VERSION | tr -d ' \t\n\r')" \
    -o /havline ./cmd/havline

# 一键升级 sidecar：独立容器持有 Docker Socket，主程序只通过共享 Unix Socket 调用它
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS updater-builder
ARG TARGETARCH
ARG GOPROXY
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -trimpath -o /havline-updater ./cmd/havline-updater

# updater 运行时：Docker CLI / Compose 与 Git 只在 sidecar 内可用
FROM ${UPDATER_IMAGE} AS updater
RUN apk add --no-cache git docker-cli-compose docker-cli-buildx ca-certificates
COPY --from=updater-builder /havline-updater /usr/local/bin/havline-updater
ENTRYPOINT ["/usr/local/bin/havline-updater"]

# 运行时 stage
FROM ${RUNTIME_IMAGE}
ARG TARGETARCH
# frpc 不预装：需要内网穿透时在「应用配置」弹窗内按需下载（存入数据目录）
RUN apt-get update \
    && apt-get install -y --no-install-recommends nginx ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=go-builder /havline /app/havline
COPY --from=go-builder /src/web/assets/image /app/web/assets/image
COPY migrations /app/migrations

# 时区：Go 的 time.Local 取决于 TZ 环境变量，且需要镜像内存在 zoneinfo（故装上 tzdata）
ENV TZ=Asia/Shanghai \
    HAVLINE_DATA_DIR=/data \
    HAVLINE_LISTEN=:6893 \
    HAVLINE_NGINX_MIME_TYPES=/etc/nginx/mime.types

EXPOSE 6893 80 443

VOLUME ["/data"]

CMD ["/app/havline"]

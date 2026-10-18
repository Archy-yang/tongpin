# 同频后端：两阶段构建，最终镜像只含二进制与迁移文件
FROM golang:1.26-alpine AS build
WORKDIR /src
# 国内网络环境用 goproxy.cn；已可达 proxy.golang.org 时可去掉
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/server ./server
COPY --from=build /out/migrate ./migrate
COPY migrations/ ./migrations/
COPY docker/entrypoint.sh ./entrypoint.sh
RUN chmod +x ./entrypoint.sh

EXPOSE 8080
ENTRYPOINT ["./entrypoint.sh"]

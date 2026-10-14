.PHONY: run build tidy migrate compose-up compose-down

run:        ## 本地启动开发服务（需先 migrate 且 PostgreSQL 可用）
	go run ./cmd/server

build:      ## 构建二进制
	go build -o bin/server ./cmd/server

tidy:       ## 整理依赖
	go mod tidy

migrate:    ## 执行数据库迁移（需 DATABASE_URL 可用）
	go run ./cmd/migrate

compose-up: ## 启动本地 PostgreSQL
	docker compose up -d

compose-down:
	docker compose down

# 常用命令速查
## 1、后端命令
 进入后端目录
  cd backend

 运行开发服务器
 go run ./cmd/server

 编译二进制（包含前端）
  VERSION="$(./scripts/resolve-version.sh)"
  go build -tags embed -ldflags="-X main.Version=${VERSION}" -o sub2api.exe ./cmd/server

 查看版本
  go run ./cmd/server --version

 运行测试
  go test ./...

 重新生成 Ent 和 Wire 代码（修改 schema 后）
  go generate ./ent
  go generate ./cmd/server

## 2、前端命令
 进入前端目录
  cd frontend

 安装依赖
  pnpm install

 启动开发服务器
  pnpm run dev

 构建生产版本（输出到 ../backend/internal/web/dist/）
  pnpm run build

 预览生产构建
  pnpm run preview

 代码检查
  pnpm run lint

 类型检查
  pnpm run typecheck

 运行测试
  pnpm run test

## 3、Docker 服务管理
 查看容器状态
  docker ps

 启动/停止 PostgreSQL
  docker start sub2api-postgres
  docker stop sub2api-postgres

 启动/停止 Redis
  docker start sub2api-redis
  docker stop sub2api-redis

 查看 PostgreSQL 日志
  docker logs sub2api-postgres

 查看 Redis 日志
  docker logs sub2api-redis

 连接 PostgreSQL
  docker exec -it sub2api-postgres psql -U sub2api

 连接 Redis
  docker exec -it sub2api-redis redis-cli
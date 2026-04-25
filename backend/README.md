# Backend

## 启动
```bash
go mod tidy
go run ./cmd/server
```

## 环境变量
- `HTTP_ADDR`：监听地址，默认 `:8080`
- `DB_PATH`：SQLite 路径，默认 `data/arenea.db`
- `API_BASIC_USER`：可选，开启 Basic Auth 用户名
- `API_BASIC_PASS`：可选，开启 Basic Auth 密码

## 核心接口
- `GET /healthz`
- `GET/POST /api/v1/agents`
- `GET/POST /api/v1/sessions`
- `GET/POST /api/v1/chat/messages`

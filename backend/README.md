# Backend

Aranea 控制平面只发一个二进制 `aranea`：

- 不带子命令进入 ADK 交互式控制台（`__system_admin__` Agent）。
- `aranea web` 启动 HTTP 后端（REST `/api/v1/*` + SSE 聊天流）。
- `aranea agent|skill|tool|plugin|mcp|cron|channel|monitor|session|system|config|version` 等子命令直接通过 REST 操作后端。

## 启动

```bash
go mod tidy
go run ./cmd web
```

> 等价的二进制流程：`go build -o aranea ./cmd && ./aranea web`。
> Windows 下 `go build -o aranea.exe ./cmd` 同理。

## 常见用法

```bash
# 1) 进入交互式控制台（默认 console 子启动器）
go run ./cmd

# 2) 在 :8080 启动后端
go run ./cmd web

# 3) 嵌入式 playground，绑定到随机端口、静音日志
go run ./cmd web --addr :0 --quiet

# 4) 远程运维（在另一台机器/终端连接同一后端）
ARANEA_BASE_URL=http://your-host:8080 go run ./cmd agent ls
ARANEA_BASE_URL=http://your-host:8080 go run ./cmd skill install https://github.com/owner/repo
```

## 环境变量

- `HTTP_ADDR`：监听地址，默认 `:8080`（被 `aranea web --addr` 覆盖）
- `DB_PATH`：SQLite 路径，默认 `data/arenea.db`（被 `aranea web --db` 覆盖）
- `API_BASIC_USER` / `API_BASIC_PASS`：可选，开启 Basic Auth
- `ARANEA_BASE_URL` / `ARANEA_TOKEN`：CLI 子命令用于连接远程后端

## 核心接口

- `GET  /healthz`
- `GET/POST /api/v1/agents`
- `GET/POST /api/v1/sessions`
- `GET/POST /api/v1/chat/messages`
- 完整列表见 `docs/25 cli.md` §6.2。

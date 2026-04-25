# Smoke Test

## 1. 健康检查
```bash
curl http://localhost:8080/healthz
```

## 2. 创建 Agent
```bash
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{"agent_key":"demo","display_name":"Demo Agent","provider":"openrouter","model":"gpt-4.1-mini"}'
```

## 3. 创建 Session
```bash
curl -X POST http://localhost:8080/api/v1/sessions \
  -H "Content-Type: application/json" \
  -d '{"agent_id":"<agent_id>","title":"first"}'
```

## 4. 发送消息
```bash
curl -X POST http://localhost:8080/api/v1/chat/messages \
  -H "Content-Type: application/json" \
  -d '{"session_id":"<session_id>","agent_key":"demo","content":"你好"}'
```

param(
  [Parameter(Mandatory = $true)][string]$SqliteFile,
  [Parameter(Mandatory = $true)][string]$PostgresUrl
)

Write-Output "迁移入口（占位）"
Write-Output "SQLite: $SqliteFile"
Write-Output "PostgreSQL: $PostgresUrl"
Write-Output "建议实现步骤："
Write-Output "1) 导出 SQLite 核心表为 CSV（agents/sessions/messages/audit_logs）。"
Write-Output "2) 导入 PostgreSQL 临时表。"
Write-Output "3) 校验行数、主键唯一性、时间字段格式。"
Write-Output "4) 切换 DB_DRIVER=postgres 并灰度验证。"

param(
  [string]$Source = "backend/data/arenea.db",
  [string]$OutputDir = "deploy/backups"
)

if (!(Test-Path $Source)) {
  Write-Error "SQLite 文件不存在: $Source"
  exit 1
}

if (!(Test-Path $OutputDir)) {
  New-Item -ItemType Directory -Path $OutputDir | Out-Null
}

$ts = Get-Date -Format "yyyyMMdd-HHmmss"
$target = Join-Path $OutputDir "arenea-$ts.db"
Copy-Item $Source $target
Write-Output "备份完成: $target"

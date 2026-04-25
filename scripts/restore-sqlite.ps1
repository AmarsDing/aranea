param(
  [Parameter(Mandatory = $true)][string]$BackupFile,
  [string]$Target = "backend/data/arenea.db"
)

if (!(Test-Path $BackupFile)) {
  Write-Error "备份文件不存在: $BackupFile"
  exit 1
}

$dir = Split-Path -Parent $Target
if (!(Test-Path $dir)) {
  New-Item -ItemType Directory -Path $dir | Out-Null
}

Copy-Item $BackupFile $Target -Force
Write-Output "恢复完成: $Target"

# start-dev.ps1 - 一键启动 sub2api 开发环境
# 用法：在项目根目录执行 .\start-dev.ps1
# 依赖：backend/config.yaml 配置文件已存在（已创建）

$ErrorActionPreference = "Stop"

# 启动后端（新窗口）
# 使用 backend/config.yaml 配置文件，仅需 2 个环境变量
# 注意：值必须用单引号括起来，避免 Start-Process 传递时双引号被吞掉
$startBackend = @'
Set-Location 'd:\working\AI\sub2api\backend'
$env:SKIP_SETUP = 'true'
$env:GOTOOLCHAIN = 'auto'
go run ./cmd/server/
'@
Start-Process powershell -ArgumentList '-NoExit', '-Command', $startBackend

# 启动前端（新窗口）
$startFrontend = @'
Set-Location 'd:\working\AI\sub2api\frontend'
node node_modules/vite/bin/vite.js
'@
Start-Process powershell -ArgumentList '-NoExit', '-Command', $startFrontend

Write-Host '前端: http://localhost:3000'
Write-Host '后端: http://localhost:8080'

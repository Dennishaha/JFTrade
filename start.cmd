@echo off
setlocal

cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
  echo go 未安装或不在 PATH 中
  exit /b 1
)

where npm >nul 2>nul
if errorlevel 1 (
  echo npm 未安装或不在 PATH 中
  exit /b 1
)

if not exist node_modules (
  echo 安装前端依赖...
  call npm install
  if errorlevel 1 exit /b 1
)

echo 运行 Go 测试...
go test ./...
if errorlevel 1 exit /b 1

echo 运行前端类型检查...
call npm run typecheck
if errorlevel 1 exit /b 1

echo 构建前端...
call npm run build:web
if errorlevel 1 exit /b 1

echo 启动后端服务...
start "jftrade-backend" cmd /k "cd /d %CD% && go run ./cmd/jftrade run --config ./config/jftrade.yaml"

echo 启动前端预览服务: http://127.0.0.1:4173
call npm --workspace @jftrade/web run preview
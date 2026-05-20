@echo off
setlocal

cd /d "%~dp0"

rem --- Default runtime configuration --------------------------------------
rem Default the sidecar to FutuOpenD's native API port. Full bbgo engine runs can
rem still use `go run ./cmd/jftrade run --config ./config/jftrade.yaml` directly.
if "%JFTRADE_API_BIND%"=="" set JFTRADE_API_BIND=127.0.0.1:3000
if "%JFTRADE_FUTU_API_PORT%"=="" set JFTRADE_FUTU_API_PORT=11110
if "%JFTRADE_FUTU_WEBSOCKET_PORT%"=="" set JFTRADE_FUTU_WEBSOCKET_PORT=11111
if "%FUTU_OPEND_ADDR%"=="" set FUTU_OPEND_ADDR=127.0.0.1:%JFTRADE_FUTU_API_PORT%
if "%DISABLE_MARKETS_CACHE%"=="" set DISABLE_MARKETS_CACHE=1
rem Suppress Node DEP0205 deprecation noise from vite plugins.
if "%NODE_OPTIONS%"=="" set NODE_OPTIONS=--no-deprecation

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
start "jftrade-backend" cmd /k "cd /d %CD% && go run ./cmd/jftrade api"

echo JFTrade API: http://%JFTRADE_API_BIND%
echo 启动前端预览服务: http://127.0.0.1:6688
call npm --workspace @jftrade/web run preview
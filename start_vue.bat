@echo off
rem ============================================================
rem VouchChain 启动脚本 - 含自动打开浏览器功能
rem ============================================================
echo.
echo 正在启动 VouchChain 后端服务...
echo.

rem 启动 Go 后端服务
start "" /b vouchchain.exe

rem 等待服务启动（最多等待 10 秒）
timeout /t 5 /nobreak > nul

rem ----- 关键部分：使用 PowerShell 更可靠地打开浏览器 -----
rem 获取本机 IPv4（排除 127.0.0.1）
for /f "tokens=2 delims=[]" %a in ('ipconfig ^| findstr "IPv4"') do (
    set "LOCAL_IP=%~a"
)
rem 确保取到了有效 IP
if "%LOCAL_IP%"=="" set "LOCAL_IP=127.0.0.1"

rem 使用 PowerShell Start-Process 打开浏览器
rem 优先使用局域网IP，失败回退 localhost
powershell -NoProfile -ExecutionPolicy Bypass -Command "
try {
    $ip = '%LOCAL_IP%'
    if (-not [System.Net.IPAddress]::Parse('$ip').IsLoopback) {
        Write-Host 'Opening browser to http://$ip:8000'
        Start-Process 'http://$ip:8000'
    } else {
        Write-Host 'IP is loopback, opening http://localhost:8000'
        Start-Process 'http://localhost:8000'
    }
} catch {
    Write-Host 'Failed to open browser, trying localhost: http://localhost:8000'
    Start-Process 'http://localhost:8000'
}
"

echo.
echo 服务已启动。
echo 如果浏览器没有自动打开，请手动访问：
echo   http://localhost:8000
echo   或者使用 IP: http://%LOCAL_IP%:8000
pause
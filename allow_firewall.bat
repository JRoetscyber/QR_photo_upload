@echo off
:: Batch script to allow port 5167 and wedding_server.exe through Windows Defender Firewall
echo ============================================================
echo   Wedding Server - Windows Firewall Port Opener (Port 5167)
echo ============================================================
echo.

:: Check for administrative permissions
net session >nul 2>&1
if %errorLevel% == 0 (
    echo [OK] Running with Administrator privileges.
) else (
    echo [!] Requesting Administrator privileges...
    powershell -Command "Start-Process cmd -ArgumentList '/c \"\"%~f0\"\"' -Verb RunAs"
    exit /b
)

echo Adding inbound firewall rule for TCP Port 5167...
netsh advfirewall firewall add rule name="Wedding Photo Server (Port 5167)" dir=in action=allow protocol=TCP localport=5167 profile=any

echo Adding inbound firewall rule for wedding_server.exe...
netsh advfirewall firewall add rule name="Wedding Server App" dir=in action=allow program="%~dp0wedding_server.exe" profile=any

echo.
echo ============================================================
echo [SUCCESS] Port 5167 is now open on your local Wi-Fi!
echo Your phone can now connect to: http://192.168.50.191:5167
echo ============================================================
echo.
pause

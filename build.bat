@echo off
REM Build FreeWheel for Windows (no console window)
cd /d "%~dp0"
set PATH=C:\msys64\mingw64\bin;C:\Program Files\Go\bin;%PATH%
set CGO_ENABLED=1
go build -ldflags "-H windowsgui" -o freewheel.exe .
if %ERRORLEVEL% equ 0 (
    echo Built freewheel.exe successfully
) else (
    echo Build failed!
)
pause

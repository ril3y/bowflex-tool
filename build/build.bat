@echo off
REM Build FreeWheel for Windows (no console window)
cd /d "%~dp0\.."
set PATH=C:\msys64\mingw64\bin;C:\Program Files\Go\bin;%PATH%
set CGO_ENABLED=1

REM Copy latest APKs from velo-platform build outputs into assets/
set VELO=..\velo-platform

REM SerialBridge requires platform signing (android.uid.system)
REM Use platform-signed APK from sign.sh, NOT the debug build
if exist "%VELO%\freewheelbridge\build\outputs\apk\freewheelbridge.apk" (
    copy /Y "%VELO%\freewheelbridge\build\outputs\apk\freewheelbridge.apk" assets\serialbridge.apk >nul
    echo Copied freewheelbridge (platform-signed) -^> assets\serialbridge.apk
) else (
    echo WARNING: Platform-signed freewheelbridge.apk not found!
    echo   Run: cd ..\velo-platform ^&^& ./gradlew :freewheelbridge:assembleDebug ^&^& freewheelbridge/sign.sh
    echo   Using existing assets\serialbridge.apk
)

REM VeloLauncher uses debug signing (no platform key needed)
if exist "%VELO%\launcher\build\outputs\apk\debug\launcher-debug.apk" (
    copy /Y "%VELO%\launcher\build\outputs\apk\debug\launcher-debug.apk" assets\velolauncher.apk >nul
    echo Copied launcher -^> assets\velolauncher.apk
) else (
    echo WARNING: launcher APK not found -- using existing assets\velolauncher.apk
)

go build -ldflags "-H windowsgui" -o freewheel.exe ./cmd/freewheel
if %ERRORLEVEL% equ 0 (
    echo Built freewheel.exe successfully
) else (
    echo Build failed!
)
pause

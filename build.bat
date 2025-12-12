@echo off
echo Building AndroidSafeLocal...
set CGO_ENABLED=1
go build -ldflags "-H=windowsgui" -o AndroidSafeLocal.exe ./cmd/android-safe-local
if %ERRORLEVEL% EQU 0 (
    echo Build Successful! Run AndroidSafeLocal.exe to start.
) else (
    echo Build Failed!
    exit /b %ERRORLEVEL%
)

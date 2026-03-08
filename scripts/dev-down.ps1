& powershell -ExecutionPolicy Bypass -File "$PSScriptRoot/dev-stack.ps1" -Action down
exit $LASTEXITCODE

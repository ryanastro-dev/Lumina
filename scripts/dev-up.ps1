param(
  [switch]$Build
)

$args = @("-ExecutionPolicy", "Bypass", "-File", "$PSScriptRoot/dev-stack.ps1", "-Action")
if ($Build) {
  $args += "rebuild"
} else {
  $args += "up"
}

& powershell @args
exit $LASTEXITCODE

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath $projectRoot
try {
    New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
    go build -trimpath -ldflags '-H=windowsgui -s -w' -o 'dist/DynamicIsland.exe' .
    if ($LASTEXITCODE -ne 0) {
        throw 'Build failed.'
    }
    Write-Host "Built: $projectRoot\dist\DynamicIsland.exe"
} finally {
    Pop-Location
}

$ErrorActionPreference = 'Stop'
Push-Location -LiteralPath $PSScriptRoot
try {
    New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
    go build -trimpath -ldflags '-H=windowsgui -s -w' -o 'dist/DynamicIsland.exe' .
    if ($LASTEXITCODE -ne 0) {
        throw 'Build failed.'
    }
    Write-Host "Built: $PSScriptRoot\dist\DynamicIsland.exe"
} finally {
    Pop-Location
}

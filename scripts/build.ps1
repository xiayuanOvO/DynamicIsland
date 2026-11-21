$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath $projectRoot
try {
    $architecture = go env GOARCH
    if ($LASTEXITCODE -ne 0) {
        throw 'Could not determine the build architecture.'
    }
    go tool mygo build -platform "windows/$architecture" -skip-installer
    if ($LASTEXITCODE -ne 0) {
        throw 'Build failed.'
    }
    Copy-Item -LiteralPath "dist\windows-$architecture\DynamicIsland.exe" -Destination 'dist\DynamicIsland.exe' -Force
    Write-Host "Built: $projectRoot\dist\DynamicIsland.exe"
} finally {
    Pop-Location
}

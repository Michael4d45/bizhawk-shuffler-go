# build.ps1 - build server and client into bin\
$root = Split-Path -Parent $MyInvocation.MyCommand.Definition
Push-Location $root

$bin = Join-Path $root 'bin'
if (-not (Test-Path $bin)) { New-Item -ItemType Directory -Path $bin | Out-Null }

# choose extension for Windows
$ext = ''
if ($env:OS -and $env:OS -match "Windows") { $ext = '.exe' }

Write-Output "Building server..."
& go build -o (Join-Path $bin "server/bizshuffle-server$ext") ./cmd/server
if ($LASTEXITCODE -ne 0) { Write-Error "Server build failed (exit $LASTEXITCODE)"; Pop-Location; exit $LASTEXITCODE }
# copy web assets into bin/server so the binary can serve them from ./web
$destWeb = Join-Path $bin 'server/web'
if (Test-Path $destWeb) { Remove-Item -Recurse -Force $destWeb }
Copy-Item -Recurse -Force (Join-Path $root 'web') $destWeb

Write-Output "Building client..."
& go build -o (Join-Path $bin "client/bizshuffle-client$ext") ./cmd/client
if ($LASTEXITCODE -ne 0) { Write-Error "Client build failed (exit $LASTEXITCODE)"; Pop-Location; exit $LASTEXITCODE }
# ensure client runtime assets are present (scripts, config, etc.)
$clientDir = Join-Path $bin 'client'

# Copy the shuffler Lua script into the client scripts directory with the expected filename
$srcLua = Join-Path $root 'server.lua'
$dstLua = Join-Path $clientDir 'server.lua'
if (Test-Path $srcLua) {
	Copy-Item -Force $srcLua $dstLua
	Write-Output "Copied $srcLua -> $dstLua"
} else {
	Write-Warning "Lua script $srcLua not found; skipping copy."
}

Write-Output "Build complete. Binaries in: $bin"
Get-ChildItem $bin | Format-Table Name, Length

Pop-Location

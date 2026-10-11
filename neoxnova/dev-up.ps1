# dev-up.ps1 - bring up the local Neo-XNova dev stack and run the server on
# Windows WITHOUT requiring `make`. Mirrors dev-up.sh / the Makefile:
#
#   1. docker compose up -d              (Postgres 16 + Redis 7)
#   2. wait for Postgres to accept connections
#   3. apply every migrations\*.sql except 0002_seed.sql, in order (idempotent)
#   4. optionally seed (-Seed)
#   5. go run ./cmd/server
#
# Usage (from neoxnova\):
#   pwsh -File dev-up.ps1                 # migrate + run
#   pwsh -File dev-up.ps1 -Seed           # also apply 0002_seed.sql first
#   pwsh -File dev-up.ps1 -MigrateOnly    # docker + migrations only, no server
#   pwsh -File dev-up.ps1 -SeedOnly       # docker + migrations + seed, no server
#
# Env overrides: DB_CONTAINER (neoxnova_postgres), DB_USER (postgres), DB_NAME (neoxnova).

[CmdletBinding()]
param(
  [switch]$Seed,
  [switch]$SeedOnly,
  [switch]$MigrateOnly
)

$ErrorActionPreference = 'Stop'
Set-Location -Path $PSScriptRoot

$DbContainer = if ($env:DB_CONTAINER) { $env:DB_CONTAINER } else { 'neoxnova_postgres' }
$DbUser = if ($env:DB_USER) { $env:DB_USER } else { 'postgres' }
$DbName = if ($env:DB_NAME) { $env:DB_NAME } else { 'neoxnova' }

function Assert-Command([string]$name) {
  if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
    throw "$name not found on PATH"
  }
}

function Invoke-SqlFile([string]$path) {
  Write-Host "      -> $path"
  Get-Content -Raw -LiteralPath $path |
    docker exec -i $DbContainer psql -U $DbUser -d $DbName -v ON_ERROR_STOP=1
  if ($LASTEXITCODE -ne 0) { throw "psql failed applying $path" }
}

Assert-Command docker
Assert-Command go

# .env is auto-loaded by the server (godotenv) and supplies the required
# DATABASE_URL; create it from the example if missing.
if (-not (Test-Path .env) -and (Test-Path .env.example)) {
  Copy-Item .env.example .env
  Write-Host '[i] created .env from .env.example'
}

Write-Host '[1/4] docker compose up -d'
docker compose up -d
if ($LASTEXITCODE -ne 0) { throw 'docker compose up failed' }

Write-Host "[2/4] waiting for Postgres ($DbContainer) ..."
$ready = $false
for ($i = 1; $i -le 60; $i++) {
  docker exec $DbContainer pg_isready -U $DbUser -d $DbName *> $null
  if ($LASTEXITCODE -eq 0) { $ready = $true; break }
  Start-Sleep -Seconds 1
}
if (-not $ready) { throw 'Postgres did not become ready in time' }
Write-Host '      ready'

Write-Host '[3/4] applying migrations'
Get-ChildItem -Path migrations -Filter *.sql |
  Where-Object { $_.Name -ne '0002_seed.sql' } |
  Sort-Object Name |
  ForEach-Object { Invoke-SqlFile $_.FullName }

if ($Seed -or $SeedOnly) {
  Write-Host '[i] seeding 0002_seed.sql'
  Invoke-SqlFile (Join-Path $PSScriptRoot 'migrations/0002_seed.sql')
}

if ($MigrateOnly -or $SeedOnly) {
  Write-Host '[done]'
} else {
  Write-Host '[4/4] go run ./cmd/server  (Ctrl-C to stop)'
  go run ./cmd/server
}

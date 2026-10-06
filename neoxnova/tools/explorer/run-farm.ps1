# run-farm.ps1 - start/stop/status/logs for the acc1 rolling-farm loops on Windows.
#
#   pwsh -File run-farm.ps1 start
#   pwsh -File run-farm.ps1 status
#   pwsh -File run-farm.ps1 logs
#   pwsh -File run-farm.ps1 stop
#
# acc1 runs locally (no tmux); each loop self-logs to data/farm-<kind>-<acc>.log.
[CmdletBinding()]
param(
  [ValidateSet('start', 'stop', 'status', 'logs')]
  [string]$Action = 'start',
  [string]$Acc = 'acc1'
)

$ErrorActionPreference = 'Stop'
Set-Location -Path $PSScriptRoot

$Bash = (Get-Command bash.exe -ErrorAction SilentlyContinue).Source
if (-not $Bash) { $Bash = 'C:\Program Files\Git\usr\bin\bash.exe' }

function Get-FarmProcs {
  Get-CimInstance Win32_Process |
    Where-Object { $_.CommandLine -and $_.CommandLine -match 'run-farm-(build|send)\.sh' -and $_.CommandLine -match $Acc }
}

function Start-Loop([string]$Script) {
  $running = Get-FarmProcs | Where-Object { $_.CommandLine -match [regex]::Escape($Script) }
  if ($running) { Write-Host "[=] $Script $Acc already running"; return }
  Start-Process -FilePath $Bash -ArgumentList @($Script, $Acc) -WorkingDirectory $PSScriptRoot -WindowStyle Hidden
  Write-Host "[+] started $Script $Acc"
}

switch ($Action) {
  'start' {
    New-Item -ItemType Directory -Force -Path data | Out-Null
    Start-Loop 'run-farm-build.sh'
    Start-Sleep -Seconds 3
    Start-Loop 'run-farm-send.sh'
    Write-Host "[*] logs: data/farm-build-$Acc.log data/farm-send-$Acc.log"
  }
  'status' {
    $rows = Get-FarmProcs | ForEach-Object {
      if ($_.CommandLine -match 'run-farm-build\.sh') { 'build' } elseif ($_.CommandLine -match 'run-farm-send\.sh') { 'send' }
    }
    if ($rows) { $rows | Sort-Object } else { Write-Host '[*] no farm loops running' }
  }
  'stop' {
    $any = $false
    foreach ($p in (Get-FarmProcs)) {
      Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue
      Write-Host "[-] stopped farm loop pid $($p.ProcessId)"; $any = $true
    }
    if (-not $any) { Write-Host '[*] nothing to stop' }
  }
  'logs' {
    $files = Get-ChildItem "data/farm-build-$Acc.log", "data/farm-send-$Acc.log" -ErrorAction SilentlyContinue
    if (-not $files) { Write-Host '[*] no logs yet'; break }
    Get-Content $files.FullName -Tail 20 -Wait
  }
}

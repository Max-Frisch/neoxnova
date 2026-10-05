# run-colonies.ps1 - start/stop/status/logs for acc1 colony build daemons on Windows.
#
#   pwsh -File run-colonies.ps1 start
#   pwsh -File run-colonies.ps1 status
#   pwsh -File run-colonies.ps1 logs
#   pwsh -File run-colonies.ps1 stop
#
# Optional: -Coords "3:124:9,2:191:9,..."  -Plan "plans/acc1-colo-grow.json"
[CmdletBinding()]
param(
  [ValidateSet('start', 'stop', 'status', 'logs')]
  [string]$Action = 'start',
  [string]$Coords = '',
  [string]$Plan = 'plans/acc1-colo-grow.json'
)

$ErrorActionPreference = 'Stop'
Set-Location -Path $PSScriptRoot
if (-not $Coords) { $Coords = '3:124:9,3:124:10,3:124:11,2:191:9,2:191:10,2:191:11' }

function Get-ColonyCps {
  $out = & node --max-old-space-size=96 httpbot.mjs planets 2>$null
  $j = $out | ConvertFrom-Json
  foreach ($w in $Coords.Split(',')) {
    $p = $j.planets | Where-Object { $_.coords -eq $w } | Select-Object -First 1
    if ($p) { [pscustomobject]@{ id = $p.id; coords = $w; name = $p.name } }
    else { Write-Warning "no planet found for $w" }
  }
}

function Get-DaemonProcs {
  Get-CimInstance Win32_Process |
    Where-Object { $_.CommandLine -and $_.CommandLine -match 'run-colony-daemon\.ps1' }
}

switch ($Action) {
  'start' {
    New-Item -ItemType Directory -Force -Path data | Out-Null
    $run = Get-DaemonProcs
    $cps = Get-ColonyCps
    if (-not $cps) { Write-Error 'no matching planets'; exit 1 }
    foreach ($c in $cps) {
      if ($run | Where-Object { $_.CommandLine -match "-Cp\s+$($c.id)(\s|$)" }) {
        Write-Host "[=] colo-$($c.id) already running"; continue
      }
      Start-Process pwsh -WindowStyle Hidden -ArgumentList @(
        '-NoProfile', '-File', "$PSScriptRoot\run-colony-daemon.ps1",
        '-Plan', $Plan, '-Cp', $c.id
      ) | Out-Null
      Write-Host "[+] started colo-$($c.id) ($($c.coords) $($c.name)) -> data/colo-$($c.id).log"
      Start-Sleep -Seconds 5
    }
    if (-not ($run | Where-Object { $_.CommandLine -match '-Bonus' })) {
      Start-Process pwsh -WindowStyle Hidden -ArgumentList @(
        '-NoProfile', '-File', "$PSScriptRoot\run-colony-daemon.ps1", '-Bonus'
      ) | Out-Null
      Write-Host "[+] started bonus -> data/bonus.log"
    }
    Write-Host "[*] done."
  }
  'status' {
    $rows = Get-DaemonProcs | ForEach-Object {
      if ($_.CommandLine -match '-Bonus') { 'bonus' }
      elseif ($_.CommandLine -match '-Cp\s+(\d+)') { "colo-$($matches[1])" }
    }
    if ($rows) { $rows | Sort-Object } else { Write-Host '[*] no colony daemons running' }
  }
  'stop' {
    $any = $false
    foreach ($p in (Get-DaemonProcs)) {
      Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue
      Write-Host "[-] stopped daemon pid $($p.ProcessId)"; $any = $true
    }
    # orphaned node children of killed wrappers
    Get-CimInstance Win32_Process |
      Where-Object { $_.CommandLine -and $_.CommandLine -match 'httpbot\.mjs (resolve|get)' } |
      ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue; $any = $true }
    if (-not $any) { Write-Host '[*] nothing to stop' }
  }
  'logs' {
    $files = Get-ChildItem data/colo-*.log, data/bonus.log -ErrorAction SilentlyContinue
    if (-not $files) { Write-Host '[*] no logs yet'; break }
    Get-Content $files.FullName -Tail 20 -Wait
  }
}

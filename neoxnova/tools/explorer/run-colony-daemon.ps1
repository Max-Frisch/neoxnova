# run-colony-daemon.ps1 - one detached restart-loop daemon for a single colony.
# Launched by run-colonies.ps1 (acc1 runs locally on Windows; no tmux).
[CmdletBinding()]
param(
  [string]$Plan = 'plans/acc1-colo-grow.json',
  [int]$Cp = 0,
  [switch]$Bonus,
  [int]$Bump = 360,
  [int]$Sats = 200,
  [int]$Wait = 12000,
  [int]$Timeout = 60000,
  [int]$MinDelay = 1200,
  [int]$MaxDelay = 2500,
  [int]$BonusEvery = 900
)

Set-Location -Path $PSScriptRoot
$env:EXPLORER_BUILDER_BUMP_SEC = "$Bump"
$env:EXPLORER_ENERGY_SATS = "$Sats"
$env:EXPLORER_QUEUE_WAIT_MS = "$Wait"
$env:EXPLORER_FETCH_TIMEOUT_MS = "$Timeout"
$env:EXPLORER_MIN_DELAY_MS = "$MinDelay"
$env:EXPLORER_MAX_DELAY_MS = "$MaxDelay"

function Stamp { (Get-Date -Format 'yyyy-MM-dd HH:mm:ss') }

if ($Bonus) {
  $log = 'data/bonus.log'
  while ($true) {
    & node --max-old-space-size=96 httpbot.mjs get 'game.php?page=bonus' *>> $log
    Add-Content -Path $log -Value "[$(Stamp)] online bonus checked"
    Start-Sleep -Seconds $BonusEvery
  }
} else {
  $log = "data/colo-$Cp.log"
  while ($true) {
    & node --max-old-space-size=96 httpbot.mjs resolve --goals $Plan --cp $Cp --steps 1000000 *>> $log
    Add-Content -Path $log -Value "[$(Stamp)] resolver exited; restart in 30s"
    Start-Sleep -Seconds 30
  }
}

# refresh-levels.ps1 — pull account #2's current building/research/ship levels
# from the Azure VM and save them locally to neoxnova/data/account2-levels.json.
#
# Uses the browser-less httpbot on the VM (tiny memory footprint), so it is safe
# to run alongside the resolver bot.

$ErrorActionPreference = 'Stop'
$key    = 'C:\code\projects\neoxnova\neoxnova\secrets\ssh\VM-GW-Automation_key.pem'
$remote = '~/neoxnova/neoxnova/tools/explorer'
$out    = 'C:\code\projects\neoxnova\neoxnova\data\account2-levels.json'

New-Item -ItemType Directory -Force -Path (Split-Path $out) | Out-Null
ssh -i $key azureuser@70.153.144.215 "cd $remote && node --max-old-space-size=96 httpbot.mjs levels --out ~/levels2.json >/dev/null 2>&1 && cat ~/levels2.json" |
  Set-Content -NoNewline -Encoding utf8 $out

Write-Host "Updated $out"

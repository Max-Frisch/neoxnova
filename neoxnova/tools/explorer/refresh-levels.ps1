# refresh-levels.ps1 — pull account #2's current building/research/ship levels
# from the Azure VM and save them locally to neoxnova/data/account2-levels.json.
#
# NOTE: this launches a browser on the VM. If the account-2 build bot is running
# in tmux, prefer stopping it first (tmux kill-session -t exp2) to save memory,
# or just accept the brief overlap on the 1 GB VM.

$ErrorActionPreference = 'Stop'
$key    = 'C:\code\projects\neoxnova\neoxnova\secrets\ssh\VM-GW-Automation_key.pem'
$chrome = '/home/azureuser/.cache/ms-playwright/chromium_headless_shell-1243/chrome-headless-shell-linux64/chrome-headless-shell'
$remote = '~/neoxnova/neoxnova/tools/explorer'
$out    = 'C:\code\projects\neoxnova\neoxnova\data\account2-levels.json'

New-Item -ItemType Directory -Force -Path (Split-Path $out) | Out-Null
ssh -i $key azureuser@70.153.144.215 "cd $remote && EXPLORER_EXECUTABLE_PATH=$chrome node explorer.mjs levels --out ~/levels2.json >/dev/null 2>&1 && cat ~/levels2.json" |
  Set-Content -NoNewline -Encoding utf8 $out

Write-Host "Updated $out"

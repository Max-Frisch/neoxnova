# Backup & Two-Machine Workflow (PC ↔ laptop)

Canonical source of truth: **GitHub** (`Max-Frisch/neoxnova`). Everything that
matters is tracked; only the tiny `neoxnova/secrets/` folder is carried manually.

## What is tracked vs. ignored

| Item | Size | In git? | Needed on the other machine? |
|---|---|---|---|
| Source, docs, plans (`git ls-files`) | — | yes | yes → `git clone` / `pull` |
| `neoxnova/secrets/` (`explorer.env`, `ssh/` key+config) | ~6 KB | **no** | **yes** — carry encrypted |
| `neoxnova/tools/explorer/node_modules/` | ~14 MB | no | no → `npm ci` |
| `neoxnova/tools/explorer/data/` (captures, logs, snapshots) | ~30 MB | no | mostly no → regenerates |
| `neoxnova/data/` (e.g. `account2-levels.json`) | ~8 KB | no | optional |
| `neoxnova/bin/` (server.exe) | ~13 MB | no | no → `go build` |
| `*.HAR` raw captures | — | no | **never** (session cookies) |

Never copy `node_modules/`, `bin/` or the big `tools/explorer/data` captures
between machines — reinstall/regenerate. The only manual transfer is the ~6 KB
`secrets/` folder (keep it in an encrypted archive / password manager).

## Switching: commit on the machine you leave

```bash
cd C:\code\projects\neoxnova
git status -sb            # want: clean, aligned with origin/main
git push                  # tracked changes only; secrets/data stay local
```

## Arriving on a machine that is behind (nothing to keep)

```bash
cd C:\code\projects\neoxnova
git remote -v                       # expect origin = Max-Frisch/neoxnova
git fetch origin

git checkout -f main                # discard local tracked edits
git clean -fd neoxnova              # drop untracked NON-ignored leftovers
git reset --hard origin/main        # match GitHub exactly

git log --oneline -1
git status -sb

cd neoxnova\tools\explorer
npm ci                              # rebuild node_modules from package-lock.json
```

Notes:
- `clean -fd` (no `-x`) does **not** delete ignored paths, so
  `secrets/`, `data/`, `node_modules/`, `bin/` are preserved. Scope it to
  `neoxnova` to be safe.
- If `secrets/explorer.env` is missing on the machine, restore the 6 KB
  `secrets/` folder from the encrypted bundle.
- If `git reset` still refuses ("untracked working tree files would be
  overwritten"), run `git clean -fd neoxnova` again, or as a last resort rename
  the folder and `git clone` fresh (then restore `secrets/`).

## Arriving with local work to keep

```bash
git fetch origin
git stash push -u -m wip
git merge --ff-only origin/main     # or: git pull --rebase origin main
git stash pop
```

## Fresh machine from scratch

```bash
git clone https://github.com/Max-Frisch/neoxnova.git
# restore neoxnova\secrets\ from the encrypted bundle
cd neoxnova\neoxnova\tools\explorer && npm ci
```

## Other requirements (install once per machine)

- Go 1.24 (auto-downloaded by older toolchains) and `make` — to build/run the
  server; run Go/make from `neoxnova/`, not the repo root.
- Node + npm — for `tools/explorer`.
- Docker Desktop — for `make up` (Postgres + Redis).
- The Azure VM SSH key lives in `neoxnova/secrets/ssh/` (see `AGENTS.md`).

## Rules of thumb

- Push before leaving; pull before starting.
- Carry only `secrets/` (encrypted). Never sync `node_modules`, `bin`, HARs, or
  the exploration `data/` captures.
- `neoxnova/tools/explorer/data/techtree-graph.json` and the latest
  `levels*.json` are the only `data/` files worth optionally copying for
  continuity; everything else regenerates.

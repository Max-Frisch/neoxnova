// analyze-combat.mjs — print per-unit loss tables for selected scenarios.
import fs from 'node:fs';
const dir = 'data/combat';
function units(rep, side) {
  const out = {};
  for (const r of rep.rounds) for (const u of r[side] || []) {
    if (!out[u.code]) out[u.code] = { initial: u.count, lost: 0 };
    out[u.code].initial = Math.max(out[u.code].initial, u.count);
    out[u.code].lost += u.lost || 0;
  }
  return out;
}
function line(id) {
  const p = `${dir}/${id}.report.json`;
  if (!fs.existsSync(p)) { console.log(id, 'MISSING'); return; }
  const rep = JSON.parse(fs.readFileSync(p, 'utf8'));
  const A = units(rep, 'attacker'), D = units(rep, 'defender');
  const fmt = (o) => Object.entries(o).map(([c, v]) => `${c}:${v.initial - v.lost}/${v.initial}(-${v.lost})`).join(' ');
  console.log(`${id.padEnd(22)} R${String(rep.roundCount).padStart(2)} ${String(rep.result).padEnd(8)} | A: ${fmt(A)} | D: ${fmt(D)}`);
}
const groups = process.argv[2] ? process.argv.slice(2) : [
  'ms-pure', 'ms-s1', 'ms-s3', 'ms-s6', 'ms-def-pure', 'ms-def-s1', 'ms-def-s3',
  'db-AvsB', 'db-BvsA', 'db-mirror-200', 'db-tech-atk', 'db-tech-def',
  'regen-gauss-hl', 'regen-gauss-ion', 'regen-lf-ion', 'regen-lf-plasma',
  'overkill-carry-lc', 'overkill-carry-lf', 'ok-bs-lf', 'ok-bf-lf', 'ok-sf-lf', 'ok-bs-hf',
  'rf-cru-lf', 'rf-bs-lf', 'rf-bc-lf', 'rf-bc-bs', 'rf-sf-lf', 'rf-bm-lf',
];
for (const id of groups) line(id);

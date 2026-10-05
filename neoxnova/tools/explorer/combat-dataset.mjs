// combat-dataset.mjs — fold the per-scenario combat captures under data/combat/
// into one committed replay dataset for the future Go combat tests.
//
// Usage (from tools/explorer/): node combat-dataset.mjs [--out file]
import fs from 'node:fs';
import path from 'node:path';

const OUT = (() => { const i = process.argv.indexOf('--out'); return i > 0 ? process.argv[i + 1] : '../../testdata/niburus_combat.json'; })();
const DIR = path.join(process.cwd(), 'data', 'combat');
const idx = JSON.parse(fs.readFileSync(path.join(DIR, 'index.json'), 'utf8'));

function totals(report, side) {
  const out = {};
  for (const r of report.rounds) {
    for (const u of r[side] || []) {
      if (!out[u.code]) out[u.code] = { name: u.name, initial: u.count, lost: 0 };
      out[u.code].initial = Math.max(out[u.code].initial, u.count);
      out[u.code].lost += u.lost || 0;
    }
  }
  for (const k of Object.keys(out)) out[k].remaining = Math.max(0, out[k].initial - out[k].lost);
  return out;
}

const scenarios = [];
const baseStats = {};
const errors = [];
for (const row of idx) {
  const rp = path.join(DIR, row.id + '.report.json');
  if (!fs.existsSync(rp)) continue;
  const rep = JSON.parse(fs.readFileSync(rp, 'utf8'));
  if (rep.phpError) { errors.push({ id: row.id, attacker: row.attacker, defender: row.defender }); continue; }
  const entry = {
    id: row.id,
    notes: row.notes || undefined,
    slots: row.slots,
    attacker: row.attacker,
    defender: row.defender,
    result: rep.result,
    rounds: rep.roundCount,
    debris: rep.debris,
    moonChance: rep.moonChance,
    attackerUnits: totals(rep, 'attacker'),
    defenderUnits: totals(rep, 'defender'),
  };
  scenarios.push(entry);
  // harvest tech-0 base stats
  const code = row.id.startsWith('base-') ? Number(row.id.split('-')[2]) : null;
  if (code && rep.rounds[0]) {
    const u = [...(rep.rounds[0].attacker || []), ...(rep.rounds[0].defender || [])].find((x) => x.code === code);
    if (u) baseStats[code] = { name: u.name, attack: u.firepower, shield: u.shield, hull: u.armour };
  }
}

const techBonus = {};
for (let l = 1; l <= 20; l++) techBonus[l] = Math.round((l * (l + 2)) / 4);

const dataset = {
  meta: {
    source: 'niburuspace.com battleSimulator + CombatReport (universe_6_niburu)',
    capturedAt: new Date().toISOString(),
    techBonusFormula: 'round(L*(L+2)/4) percent, same for weapons(109)/shield(110)/armour(111)',
    note: 'derived stats = round(base * (1 + bonus/100)); debris = 50% base M+C of destroyed SHIPS only; defenses make no debris',
  },
  techBonus,
  baseStats,
  phpErrorScenarios: errors,
  scenarios,
};
const outPath = path.resolve(process.cwd(), OUT);
fs.mkdirSync(path.dirname(outPath), { recursive: true });
fs.writeFileSync(outPath, JSON.stringify(dataset, null, 2));
console.log(`wrote ${outPath}: ${scenarios.length} scenarios, ${Object.keys(baseStats).length} base units, ${errors.length} php-errors`);

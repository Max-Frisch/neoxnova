// farm-plan.mjs — recompute the endogenous farm size S and emit goal plans.
//
// Usage (from neoxnova/tools/explorer):
//   node farm-plan.mjs --acc acc1 plan   # read main levels+state -> goal JSONs + state
//   node farm-plan.mjs --acc acc1 sent   # pre-send snapshot -> grow S, phase=build
//   node farm-plan.mjs --acc acc1 show   # print state JSON
//
// S is per-fleet BB count. A full rotation maintains 7 fleets:
//   BB 7*S, HC 35*S (5:1 per fleet), BR 7*br (br=round(S/250)), 1 of each small ship.
// Each build site produces its divided share; main additionally fields the small ships.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PLANS = path.join(__dirname, 'plans');
const DATA = path.join(__dirname, 'data');

const DEFAULT_S = 42000;
// One of each small ship per fleet (all ships except Spy Probe 210 can be sent;
// the set is rejected only when one is short on the planet). The total count
// scales with the number of expedition slots (fleets per rotation) — see `slots`.

const arg = (name, def) => { const i = process.argv.indexOf(name); return i > 0 ? process.argv[i + 1] : def; };
const acc = arg('--acc');
if (!acc) { console.error('usage: farm-plan.mjs --acc acc1|acc2 [plan|sent|show]'); process.exit(2); }

const MODES = ['plan', 'sent', 'show', 'starter'];
const mode = process.argv.find((a) => MODES.includes(a)) || 'plan';

const cfg = JSON.parse(fs.readFileSync(path.join(PLANS, 'farm-sites.json'), 'utf8'))[acc];
if (!cfg) { console.error(`no farm-sites.json entry for ${acc}`); process.exit(2); }
const sites = cfg.sites.map(String);
const n = sites.length;

const brOf = (s) => Math.round(s / 250);
const statePath = path.join(DATA, `farm-state-${acc}.json`);
const readState = () => fs.existsSync(statePath) ? JSON.parse(fs.readFileSync(statePath, 'utf8')) : {};
const writeState = (st) => { fs.mkdirSync(DATA, { recursive: true }); fs.writeFileSync(statePath, JSON.stringify(st, null, 2)); };

const st = readState();
st.cycle = st.cycle || 0;
st.S = st.S || DEFAULT_S;
st.phase = st.phase || 'build';
st.br = brOf(st.S);
// Fleets per rotation = expedition slots. acc2 has 8, acc1 upgrades later; the
// send loop writes the live-detected value to st.slots, cfg.slots is the fallback.
const slots = Math.max(1, Number(st.slots || cfg.slots || 7));
const small = { '202': slots, '204': slots, '205': slots, '206': slots };

const mainFile = path.join(DATA, `farm-main-${acc}.json`);
const have = { bb: 0, hc: 0, br: 0, small: false };
if (fs.existsSync(mainFile)) {
  const s = (JSON.parse(fs.readFileSync(mainFile, 'utf8')).ships) || {};
  have.bb = Number(s['207']) || 0;
  have.hc = Number(s['203']) || 0;
  have.br = Number(s['219']) || 0;
  have.small = Object.keys(small).every((c) => (Number(s[c]) || 0) >= small[c]);
}

if (mode === 'show') {
  console.log(JSON.stringify({ ...st, have }, null, 2));
  process.exit(0);
}

if (mode === 'starter') {
  // One-off bootstrap goal: the FULL set on a single (resource-rich)
  // planet, before handing over to the distributed rolling daemons.
  const ships = { '207': slots * st.S, '203': 5 * slots * st.S, '219': slots * st.br, ...small };
  fs.writeFileSync(path.join(PLANS, `farm-${acc}-starter.json`), JSON.stringify({ ships }, null, 2) + '\n');
  console.log(`[farm-plan] ${acc} starter S=${st.S} br=${st.br} -> BB=${ships['207']} HC=${ships['203']} BR=${ships['219']}`);
  process.exit(0);
}

if (mode === 'sent') {
  // Called immediately BEFORE firing; main still holds the pre-send total.
  // Cap S by the scarcest fleet component (BB:HC = 1:5) so a bloated stockpile
  // of one ship cannot balloon S beyond what the other can actually field.
  st.S = Math.max(DEFAULT_S, Math.min(Math.floor(have.bb / slots), Math.floor(have.hc / (5 * slots))));
  st.br = brOf(st.S);
  st.cycle += 1;
  st.phase = 'build';
  st.lastSendAt = new Date().toISOString();
  writeState(st);
  console.log(`[farm-plan] ${acc} sent: cycle=${st.cycle} S=${st.S} br=${st.br} (haveBB=${have.bb})`);
  process.exit(0);
}

// plan mode: decide readiness, then emit the two goal files + state.
const needBB = slots * st.S, needHC = 5 * slots * st.S, needBR = slots * st.br;
const full = have.bb >= needBB && have.hc >= needHC && have.br >= needBR && have.small;
if (st.phase === 'build' && full) {
  st.phase = 'ready';
  console.log(`[farm-plan] ${acc} READY cycle=${st.cycle} bb=${have.bb}/${needBB} hc=${have.hc}/${needHC} br=${have.br}/${needBR} small=${have.small}`);
} else if (st.phase === 'ready' && !full) {
  // ships drawn down again (send/top-up) -> resume building
  st.phase = 'build';
}

const share = (total) => Math.ceil(total / n);
const bbShare = share(needBB), hcShare = share(needHC);
// BB/HC are spread across sites; BR is crystal-heavy and the (crystal-poor)
// colonies cannot supply their share, which deadlocked the ready gate. Keep the
// whole BR need on the crystal-rich main; sites build BB/HC only.
fs.writeFileSync(path.join(PLANS, `farm-${acc}-main.json`),
  JSON.stringify({ ships: { '207': bbShare, '203': hcShare, '219': needBR, ...small } }, null, 2) + '\n');
// Optional per-site building goals (e.g. balance crystal mine to the metal-mine
// level, bump Light conveyor for HC batch rate). resolve queues buildings before
// units each step, so with a crystal lump the mine is secured ahead of ships.
const sitePlan = { ships: { '207': bbShare, '203': hcShare } };
if (cfg.mineGoals) sitePlan.buildings = cfg.mineGoals;
fs.writeFileSync(path.join(PLANS, `farm-${acc}-site.json`),
  JSON.stringify(sitePlan, null, 2) + '\n');
writeState(st);
console.log(`[farm-plan] ${acc} plan cycle=${st.cycle} S=${st.S} phase=${st.phase} br=${st.br} ` +
  `have BB=${have.bb} HC=${have.hc} BR=${have.br} -> main BB=${bbShare} HC=${hcShare} BR=${needBR}; ` +
  `site BB=${bbShare} HC=${hcShare} (${n} sites)`);

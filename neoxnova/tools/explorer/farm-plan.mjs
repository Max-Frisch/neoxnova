// farm-plan.mjs — recompute the endogenous farm size S and emit goal plans.
//
// Usage (from neoxnova/tools/explorer):
//   node farm-plan.mjs --acc acc1 plan   # read main levels+state -> goal JSONs + state
//   node farm-plan.mjs --acc acc1 sent   # pre-send snapshot -> grow S, phase=build
//   node farm-plan.mjs --acc acc1 show   # print state JSON
//
// Composition is configurable per account in plans/farm-sites.json under "comp":
//   main     primary flying hull, S per fleet (default "207" Battleship)
//   wall     optional fodder hull + wallPer (main:wall count); omit for no wall
//   cargo    optional freighter hull + cargoPer (main per freighter)
//   recycler debris/collector hull + recyclerPer (main per recycler)
//   ramp     optional {code, per} — build a replacement hull in parallel (per =
//            main units a ramp unit is worth by points) and AUTO-FLIP `main` to it
//            once the ramp fleet matches the current main fleet's points.
//
// Meta (2026-10-08): a single heavy hull, NO fodder wall. The enemy mirrors our
// composition 0.66x, so a wall only absorbs our own alpha strike and lets the
// pirates' heavies live longer — it helps only the weaker side (see cmd/exposim).
// We are phasing Battleship -> Frigate: drop the wall now (immediate ~0% losses,
// frees the Heavy-Cargo cap so all slots fly) while ramping Frigates, then flip.
// A full rotation = `slots` fleets, each `S` main + cargo + recycler + 1 of each
// small ship. Each site produces its divided share; main fields the small ships
// and the crystal-heavy recyclers.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PLANS = path.join(__dirname, 'plans');
const DATA = path.join(__dirname, 'data');

const arg = (name, def) => { const i = process.argv.indexOf(name); return i > 0 ? process.argv[i + 1] : def; };
const acc = arg('--acc');
if (!acc) { console.error('usage: farm-plan.mjs --acc acc1|acc2 [plan|sent|show|starter]'); process.exit(2); }

const MODES = ['plan', 'sent', 'show', 'starter'];
const mode = process.argv.find((a) => MODES.includes(a)) || 'plan';

const cfg = JSON.parse(fs.readFileSync(path.join(PLANS, 'farm-sites.json'), 'utf8'))[acc];
if (!cfg) { console.error(`no farm-sites.json entry for ${acc}`); process.exit(2); }
const sites = cfg.sites.map(String);
const n = sites.length;

// Base (metal+crystal) points per unit — keeps the freighter/recycler ratios
// scale-invariant across a hull flip (a Frigate is worth ~690 Battleships).
const PTS = {
  '202': 4000, '203': 12000, '204': 4000, '205': 11000, '206': 26500, '207': 58000,
  '208': 450000, '209': 18000, '211': 120000, '213': 125000, '214': 9500000, '215': 100000,
  '216': 12500000, '217': 56500, '219': 1800000, '225': 1600000, '226': 5000000, '227': 40000000, '228': 30000000,
};

const comp = cfg.comp || {};
const WALL = comp.wall != null ? String(comp.wall) : null;
const WALL_PER = Number(comp.wallPer || 5);
const CARGO = comp.cargo != null ? String(comp.cargo) : null;
const RECY = String(comp.recycler || '219');
const RAMP = comp.ramp ? { code: String(comp.ramp.code), per: Number(comp.ramp.per || 1) } : null;

const statePath = path.join(DATA, `farm-state-${acc}.json`);
const readState = () => fs.existsSync(statePath) ? JSON.parse(fs.readFileSync(statePath, 'utf8')) : {};
const writeState = (st) => { fs.mkdirSync(DATA, { recursive: true }); fs.writeFileSync(statePath, JSON.stringify(st, null, 2)); };

const st = readState();
st.cycle = st.cycle || 0;
st.phase = st.phase || 'build';
// `main` lives in state so the ramp can flip it; config is the initial value.
const MAIN = String(st.main || comp.main || '207');
// Freighter/recycler ratios expressed in fleet points, converted to "main units
// per freighter" so they stay valid when the hull (and S's scale) changes.
const mainPts = PTS[MAIN] || 1;
const RECY_PER = Math.max(1, Math.round(Number(comp.recyclerPoints || 14.4e6) / mainPts));
const CARGO_PER = Math.max(1, Math.round(Number(comp.cargoPoints || 500e6) / mainPts));
const slots = Math.max(1, Number(st.slots || cfg.slots || 7));
const small = { '202': slots, '204': slots, '205': slots, '206': slots };

// Any composition change (drop the wall, flip the hull, tune ratios) resets S to
// the fleet's real capacity instead of ratcheting from the old constraint.
const compSig = [MAIN, WALL, WALL_PER, CARGO, CARGO_PER, RECY, RECY_PER, RAMP ? RAMP.code : ''].join('|');

const mainFile = path.join(DATA, `farm-main-${acc}.json`);
const runsPath = path.join(DATA, 'expedition-runs.json');
const parseShipsCsv = (s) => {
  const o = {};
  for (const p of String(s || '').split(',')) { const [c, v] = p.split(':'); if (c) o[c] = (o[c] || 0) + (Number(v) || 0); }
  return o;
};
const readShips = (file) => {
  if (!fs.existsSync(file)) return {};
  return (JSON.parse(fs.readFileSync(file, 'utf8')).ships) || {};
};
const mainShips = readShips(mainFile);
const num = (m, c) => Number(m[c]) || 0;

// Account-wide counts = main + every build site + the fleets currently flying.
// The production gate and targets MUST use the whole account, not just main: the
// main-only view is a partial snapshot (most ships are in flight / on colonies).
const tally = (active) => {
  const codes = [MAIN, WALL, CARGO, RECY, RAMP && RAMP.code].filter(Boolean);
  const main = {}, sitesTot = {}, inf = {};
  for (const c of codes) { main[c] = num(mainShips, c); sitesTot[c] = 0; inf[c] = 0; }
  for (const cp of sites) {
    if (String(cp) === String(cfg.mainCp)) continue;
    const s = readShips(path.join(DATA, `farm-site-${acc}-${cp}.json`));
    for (const c of codes) sitesTot[c] += num(s, c);
  }
  if (active > 0 && fs.existsSync(runsPath)) {
    const fleets = [];
    for (const r of JSON.parse(fs.readFileSync(runsPath, 'utf8'))) {
      const s = parseShipsCsv(r.ships);
      for (let i = 0; i < (Number(r.num) || 1); i++) fleets.push(s);
    }
    for (const s of fleets.slice(-active)) for (const c of codes) inf[c] += num(s, c);
  }
  const total = {};
  for (const c of codes) total[c] = main[c] + sitesTot[c] + inf[c];
  return {
    ...total,
    mainCount: total[MAIN] || 0,
    // Small ships launch from main, so the "one of each" check stays main-only.
    small: Object.keys(small).every((c) => num(mainShips, c) >= small[c]),
    parts: { main, sites: sitesTot, inflight: inf },
  };
};

let have = tally(Number(st.active) || 0);

// Auto-flip: once the ramp fleet matches the current main fleet by points
// (rampCount >= mainCount / per), switch the flying hull to the ramp.
let flipped = false;
if (RAMP && st.ramp !== false && (have[RAMP.code] || 0) >= (have[MAIN] || 0) / RAMP.per) {
  st.main = RAMP.code;
  st.ramp = false; // done ramping
  flipped = true;
}

if (mode === 'show') {
  console.log(JSON.stringify({ ...st, comp: { MAIN, WALL, WALL_PER, CARGO, CARGO_PER, RECY, RECY_PER, RAMP }, have }, null, 2));
  process.exit(0);
}

const brOf = (s) => Math.max(1, Math.round(s / RECY_PER));
const cargoOf = (s) => (CARGO ? Math.max(1, Math.round(s / CARGO_PER)) : 0);
const goal = (counts, withSmall) => {
  const ships = {};
  for (const [c, v] of Object.entries(counts)) if (v > 0) ships[c] = v;
  if (withSmall) Object.assign(ships, small);
  return ships;
};

if (mode === 'starter') {
  const counts = { [MAIN]: slots * st.S, [CARGO]: cargoOf(st.S) * slots, [RECY]: brOf(st.S) * slots };
  if (WALL) counts[WALL] = WALL_PER * slots * st.S;
  fs.writeFileSync(path.join(PLANS, `farm-${acc}-starter.json`), JSON.stringify({ ships: goal(counts, true) }, null, 2) + '\n');
  console.log(`[farm-plan] ${acc} starter S=${st.S} -> ${JSON.stringify(goal(counts, true))}`);
  process.exit(0);
}

if (mode === 'sent') {
  // GROW: S is bounded only by the WHOLE fleet (main + sites + in-flight), so it
  // rises as the shipyards add ships. A composition change resets S to capacity;
  // otherwise S ratchets so combat losses rebuild toward the high-water mark.
  const active = Math.max(0, Number(arg('--active', 0)) || 0);
  have = tally(active);
  const caps = [
    Math.floor((have[MAIN] || 0) / slots),
    WALL ? Math.floor((have[WALL] || 0) / (WALL_PER * slots)) : Infinity,
    CARGO ? Math.floor(((have[CARGO] || 0) * CARGO_PER) / slots) : Infinity,
    Math.floor(((have[RECY] || 0) * RECY_PER) / slots),
  ];
  const capS = Math.max(1, Math.min(...caps));
  if (st.compSig !== compSig || flipped) { st.S = capS; st.compSig = compSig; }
  else { st.S = Math.max(1, Math.max(st.S || 1, capS)); }
  st.br = brOf(st.S);
  st.cargo = cargoOf(st.S);
  st.active = active;
  st.phase = 'build';
  st.lastSendAt = new Date().toISOString();
  writeState(st);
  const p = have.parts;
  console.log(`[farm-plan] ${acc} grow: main=${MAIN} S=${st.S} (cap=${capS}) ` +
    `have main=${have[MAIN] || 0} wall=${have[WALL] || 0} cargo=${have[CARGO] || 0} recy=${have[RECY] || 0} ramp=${RAMP ? (have[RAMP.code] || 0) : '-'} ` +
    `[main ${p.main[MAIN] || 0} sites ${p.sites[MAIN] || 0} inflight ${p.inflight[MAIN] || 0} active=${active}] br=${st.br} cargo=${st.cargo}${flipped ? ' FLIP->' + MAIN : ''}`);
  process.exit(0);
}

// plan mode: emit the build goals. Targets sit ABOVE the current fleet (grow) so
// the shipyards never finish and keep producing flat out. `grow` is headroom.
const grow = Math.max(1, Number(cfg.growth || 3));
const targetS = Math.ceil(st.S * grow);
const share = (total) => Math.ceil(total / n);

// Ramp phase: build the replacement hull instead of more main, until it matches.
const ramping = RAMP && st.ramp !== false && (have[RAMP.code] || 0) < (have[MAIN] || 0) / RAMP.per;
const needMain = slots * targetS;
const needCargo = cargoOf(st.S) * slots;
const needRecy = brOf(st.S) * slots;
const rampTarget = ramping ? Math.ceil(grow * (have[MAIN] || 0) / RAMP.per) : 0;

const buildMain = !ramping && (cfg.forceMain || (!cfg.pauseMain && !(WALL && have[WALL] < WALL_PER * have[MAIN])));
const mainShare = buildMain ? share(needMain) : 0;
const wallShare = WALL && !ramping ? share(WALL_PER * needMain) : 0;
const rampShare = ramping ? share(rampTarget) : 0;

const full = have[MAIN] >= needMain && (!WALL || have[WALL] >= WALL_PER * needMain) && (!CARGO || have[CARGO] >= needCargo) && have[RECY] >= needRecy && have.small;
if (st.phase === 'build' && full) st.phase = 'ready';
else if (st.phase === 'ready' && !full) st.phase = 'build';

// Cargo (217) is cheap and spread across sites; recycler (219) is crystal-heavy
// and the crystal-poor colonies cannot supply their share, so keep the whole
// recycler need on the crystal-rich main. Build the counts explicitly: after the
// ramp flips, MAIN === RAMP.code, so a literal would let rampShare (0) clobber
// mainShare under the shared key.
const counts = { [MAIN]: mainShare, [CARGO]: share(needCargo) };
if (WALL) counts[WALL] = wallShare;
if (RAMP && RAMP.code !== MAIN) counts[RAMP.code] = rampShare;
const mainGoal = goal({ ...counts, [RECY]: needRecy }, true);
const siteGoal = goal(counts, false);
fs.writeFileSync(path.join(PLANS, `farm-${acc}-main.json`), JSON.stringify({ ships: mainGoal }, null, 2) + '\n');
fs.writeFileSync(path.join(PLANS, `farm-${acc}-site.json`), JSON.stringify({ ships: siteGoal }, null, 2) + '\n');
writeState(st);
console.log(`[farm-plan] ${acc} plan cycle=${st.cycle} main=${MAIN} S=${st.S} phase=${st.phase} br=${st.br} cargo=${st.cargo} ` +
  `have main=${have[MAIN] || 0} recy=${have[RECY] || 0} ${ramping ? `RAMP ${RAMP.code}=${have[RAMP.code] || 0}/${Math.floor((have[MAIN] || 0) / RAMP.per)} ` : ''}` +
  `-> main ${JSON.stringify(mainGoal)}; site ${JSON.stringify(siteGoal)} (${n} sites)`);

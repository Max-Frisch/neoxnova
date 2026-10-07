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

// Account-wide BB/HC/BR = main + every build site + the fleets currently flying.
// The production gate and targets MUST use the whole account, not just main: the
// main-only view is a partial snapshot (most ships are in flight / on colonies),
// which let HC balloon on acc1 and left BB idle on acc2.
const tally = (active) => {
  const main = { bb: num(mainShips, '207'), hc: num(mainShips, '203'), br: num(mainShips, '219') };
  const sitesTot = { bb: 0, hc: 0, br: 0 };
  for (const cp of sites) {
    if (String(cp) === String(cfg.mainCp)) continue;
    const s = readShips(path.join(DATA, `farm-site-${acc}-${cp}.json`));
    sitesTot.bb += num(s, '207'); sitesTot.hc += num(s, '203'); sitesTot.br += num(s, '219');
  }
  const inf = { bb: 0, hc: 0, br: 0 };
  if (active > 0 && fs.existsSync(runsPath)) {
    const fleets = [];
    for (const r of JSON.parse(fs.readFileSync(runsPath, 'utf8'))) {
      const s = parseShipsCsv(r.ships);
      for (let i = 0; i < (Number(r.num) || 1); i++) fleets.push(s);
    }
    for (const s of fleets.slice(-active)) { inf.bb += num(s, '207'); inf.hc += num(s, '203'); inf.br += num(s, '219'); }
  }
  return {
    bb: main.bb + sitesTot.bb + inf.bb,
    hc: main.hc + sitesTot.hc + inf.hc,
    br: main.br + sitesTot.br + inf.br,
    // Small ships launch from main, so the "one of each" check stays main-only.
    small: Object.keys(small).every((c) => num(mainShips, c) >= small[c]),
    parts: { main, sites: sitesTot, inflight: inf },
  };
};

let have = tally(Number(st.active) || 0);

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
  // GROW: S is bounded only by the size of the WHOLE fleet, so it rises as fast as
  // the shipyards add ships. We sum every ship the account owns — main + all build
  // sites + the fleets currently flying (`--active N`, reconstructed from the most
  // recent expedition-runs) — then S = fleet / slots. `st.S` ratchets up (monotonic)
  // so combat losses are rebuilt toward the high-water mark instead of shrinking the
  // plan; only the fleet growing raises it. No conservative per-planet cap.
  const active = Math.max(0, Number(arg('--active', 0)) || 0);
  have = tally(active);
  const capS = Math.max(DEFAULT_S, Math.min(Math.floor(have.bb / slots), Math.floor(have.hc / (5 * slots))));
  const grew = capS > st.S;
  st.S = Math.max(st.S, capS);
  st.br = brOf(st.S);
  if (grew) st.cycle += 1;
  st.phase = 'build';
  st.active = active;
  st.lastSendAt = new Date().toISOString();
  writeState(st);
  const p = have.parts;
  console.log(`[farm-plan] ${acc} grow: S=${st.S} (cap=${capS}) Fleet BB=${have.bb} HC=${have.hc} ` +
    `[main ${p.main.bb}/${p.main.hc} sites ${p.sites.bb}/${p.sites.hc} inflight ${p.inflight.bb}/${p.inflight.hc} active=${active}] br=${st.br}`);
  process.exit(0);
}

// plan mode: emit the build goals. Targets are `st.S * grow` so they always sit
// ABOVE the current fleet — the shipyards never finish the goal and keep producing
// flat out, so real growth is limited by build throughput (all sites), not by a cap.
// `grow` is just headroom; raise it if a shipyard ever idles waiting for the goal.
const grow = Math.max(1, Number(cfg.growth || 3));
const targetS = Math.ceil(st.S * grow);
const needBB = slots * targetS, needHC = 5 * slots * targetS, needBR = slots * st.br;
const full = have.bb >= needBB && have.hc >= needHC && have.br >= needBR && have.small;
if (st.phase === 'build' && full) {
  st.phase = 'ready';
  console.log(`[farm-plan] ${acc} READY cycle=${st.cycle} bb=${have.bb}/${needBB} hc=${have.hc}/${needHC} br=${have.br}/${needBR} small=${have.small}`);
} else if (st.phase === 'ready' && !full) {
  // ships drawn down again (send/top-up) -> resume building
  st.phase = 'build';
}

const share = (total) => Math.ceil(total / n);
// Symmetric ratio gate over the WHOLE account: the fleet needs HC:BB = 5:1, so
// build ONLY the deficient type until the ratio is restored — HC-only while
// HC < 5*BB, BB-only while HC > 5*BB, both when balanced. Previously only the
// HC-short side was gated, so the (cheaper) HC always kept building and ballooned
// while BB idled. (cfg.pauseBB/pauseHC force one off; cfg.forceBB/forceHC on.)
const bbDeficitMultiplier = 5;
const hcShort = have.hc < bbDeficitMultiplier * have.bb; // need more HC
const bbShort = have.hc > bbDeficitMultiplier * have.bb; // need more BB
const buildBB = cfg.forceBB || (!cfg.pauseBB && !hcShort);
const buildHC = cfg.forceHC || (!cfg.pauseHC && !bbShort);
const bbShare = buildBB ? share(needBB) : 0, hcShare = buildHC ? share(needHC) : 0;
// BB/HC are spread across sites; BR is crystal-heavy and the (crystal-poor)
// colonies cannot supply their share, which deadlocked the ready gate. Keep the
// whole BR need on the crystal-rich main; sites build BB/HC only.
fs.writeFileSync(path.join(PLANS, `farm-${acc}-main.json`),
  JSON.stringify({ ships: { '207': bbShare, '203': hcShare, '219': needBR, ...small } }, null, 2) + '\n');
// Ship production only — mines/conveyors are managed manually (not by the farm).
fs.writeFileSync(path.join(PLANS, `farm-${acc}-site.json`),
  JSON.stringify({ ships: { '207': bbShare, '203': hcShare } }, null, 2) + '\n');
writeState(st);
const gate = `${buildBB ? 'BB' : ''}${buildHC ? 'HC' : ''}` || 'none';
console.log(`[farm-plan] ${acc} plan cycle=${st.cycle} S=${st.S} phase=${st.phase} br=${st.br} ` +
  `have BB=${have.bb} HC=${have.hc} BR=${have.br} ratio=${(have.bb ? (have.hc / have.bb).toFixed(2) : 'inf')} gate=${gate} -> ` +
  `main BB=${bbShare} HC=${hcShare} BR=${needBR}; site BB=${bbShare} HC=${hcShare} (${n} sites)`);

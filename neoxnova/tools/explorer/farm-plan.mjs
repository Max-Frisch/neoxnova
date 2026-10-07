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
  // GROW: S is bounded only by the size of the WHOLE fleet, so it rises as fast as
  // the shipyards add ships. We sum every ship the account owns — main + all build
  // sites + the fleets currently flying (`--active N`, reconstructed from the most
  // recent expedition-runs) — then S = fleet / slots. `st.S` ratchets up (monotonic)
  // so combat losses are rebuilt toward the high-water mark instead of shrinking the
  // plan; only the fleet growing raises it. No conservative per-planet cap.
  const active = Math.max(0, Number(arg('--active', 0)) || 0);
  const parseShipsCsv = (s) => {
    const o = {};
    for (const p of String(s || '').split(',')) { const [c, v] = p.split(':'); if (c) o[c] = (o[c] || 0) + (Number(v) || 0); }
    return o;
  };
  const siteShips = { bb: 0, hc: 0 };
  for (const cp of sites) {
    if (String(cp) === String(cfg.mainCp)) continue;
    const f = path.join(DATA, `farm-site-${acc}-${cp}.json`);
    if (!fs.existsSync(f)) continue;
    const s = (JSON.parse(fs.readFileSync(f, 'utf8')).ships) || {};
    siteShips.bb += Number(s['207']) || 0;
    siteShips.hc += Number(s['203']) || 0;
  }
  const inFlight = { bb: 0, hc: 0 };
  const runsPath = path.join(DATA, 'expedition-runs.json');
  if (active > 0 && fs.existsSync(runsPath)) {
    const fleets = [];
    for (const r of JSON.parse(fs.readFileSync(runsPath, 'utf8'))) {
      const s = parseShipsCsv(r.ships);
      for (let i = 0; i < (Number(r.num) || 1); i++) fleets.push(s);
    }
    for (const s of fleets.slice(-active)) { inFlight.bb += Number(s['207']) || 0; inFlight.hc += Number(s['203']) || 0; }
  }
  const totalBB = have.bb + siteShips.bb + inFlight.bb;
  const totalHC = have.hc + siteShips.hc + inFlight.hc;
  const capS = Math.max(DEFAULT_S, Math.min(Math.floor(totalBB / slots), Math.floor(totalHC / (5 * slots))));
  const grew = capS > st.S;
  st.S = Math.max(st.S, capS);
  st.br = brOf(st.S);
  if (grew) st.cycle += 1;
  st.phase = 'build';
  st.lastSendAt = new Date().toISOString();
  writeState(st);
  console.log(`[farm-plan] ${acc} grow: S=${st.S} (cap=${capS}) Fleet BB=${totalBB} HC=${totalHC} ` +
    `[main ${have.bb}/${have.hc} sites ${siteShips.bb}/${siteShips.hc} inflight ${inFlight.bb}/${inFlight.hc} active=${active}] br=${st.br}`);
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
// Adaptive BB: the fleet needs HC:BB = 5:1 and HC is the crystal-gated bottleneck,
// so while the main actually holds less than 5*BB HC, queue ZERO new BB — send all
// output to HC. BB production resumes automatically once the ratio is restored.
// (cfg.pauseBB forces it off regardless; cfg.forceBB forces it on.)
const bbDeficitMultiplier = 5;
const hcShort = have.hc < bbDeficitMultiplier * have.bb;
const buildBB = cfg.forceBB || (!cfg.pauseBB && !hcShort);
const bbShare = buildBB ? share(needBB) : 0, hcShare = share(needHC);
// BB/HC are spread across sites; BR is crystal-heavy and the (crystal-poor)
// colonies cannot supply their share, which deadlocked the ready gate. Keep the
// whole BR need on the crystal-rich main; sites build BB/HC only.
fs.writeFileSync(path.join(PLANS, `farm-${acc}-main.json`),
  JSON.stringify({ ships: { '207': bbShare, '203': hcShare, '219': needBR, ...small } }, null, 2) + '\n');
// Ship production only — mines/conveyors are managed manually (not by the farm).
fs.writeFileSync(path.join(PLANS, `farm-${acc}-site.json`),
  JSON.stringify({ ships: { '207': bbShare, '203': hcShare } }, null, 2) + '\n');
writeState(st);
console.log(`[farm-plan] ${acc} plan cycle=${st.cycle} S=${st.S} phase=${st.phase} br=${st.br} ` +
  `have BB=${have.bb} HC=${have.hc} BR=${have.br} -> main BB=${bbShare} HC=${hcShare} BR=${needBR}; ` +
  `site BB=${bbShare} HC=${hcShare} (${n} sites)`);

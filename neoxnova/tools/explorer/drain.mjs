#!/usr/bin/env node
// drain.mjs - burn-down expedition sender for one account. Flies a single
// combat hull (Frigate, falling back when depleted) plus Battle Recyclers and
// Battle Transporters for debris/cargo and one of each sub-Frigate ship while
// still on main. No rebuilding. The per-fleet hull size, the number of fleets
// AND the recycler/transporter counts all auto-scale to what is home on main, so
// the sender never stalls as the fleet is attrited to zero.
//
//   node drain.mjs acc1 [--dry]
//
// Config: plans/farm-sites.json <acc>.drain (see acc1).
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const DATA = path.join(__dirname, 'data');
const PLANS = path.join(__dirname, 'plans');

const acc = process.argv[2];
if (!acc || acc.startsWith('--')) { console.error('usage: drain.mjs <acc> [--dry]'); process.exit(2); }
const DRY = process.argv.includes('--dry');

const cfgAll = JSON.parse(fs.readFileSync(path.join(PLANS, 'farm-sites.json'), 'utf8'));
const c = cfgAll[acc] || {};
const d = c.drain || {};
if (!d.enabled) { console.error(`[drain] ${acc}: drain.enabled is not true`); process.exit(1); }

// One of each "weak" su—the sub-hull ships plus a Frigate (227), excluding the
// Black Moon (216) and the current hull/aux (filtered in plan()).
const SMALL = ['202', '204', '205', '206', '207', '211', '213', '215', '225', '226', '227'];
const PLAN_VERSION = 'bwlean-split-v6';

const MAIN_CP = String(d.mainCp || c.mainCp);
const MAIN_COORDS = String(d.mainCoords || c.mainCoords);
const ORDER = (d.mainOrder || ['227']).map(String);
const RECY = String(d.recycler || (c.comp && c.comp.recycler) || '219');
// Units of each auxiliary per hull in a sent fleet. Give the ratio directly with
// `recyclerPerHull`/`transporterPerHull` (e.g. 0.56 = 0.56 BR per Frigate), or the
// legacy one-per-N divisor (`recyclerPer`/`transporterPer`, e.g. 5 = 1 BR per 5 hulls).
const RECY_PER_HULL = d.recyclerPerHull != null
  ? Number(d.recyclerPerHull)
  : 1 / Math.max(1, Number(d.recyclerPer || 5));
const TRANS = String(d.transporter || (c.comp && c.comp.transporter) || '217');
const TRANS_PER_HULL = d.transporterPerHull != null
  ? Number(d.transporterPerHull)
  : 1 / Math.max(1, Number(d.transporterPer || 25));
// Flat per-fleet count ("at least 1 each"): when the account is too BT-poor to
// meet a ratio, still put one Battle Transporter in every fleet so the expo
// fleet-find rolls can yield them. Overrides the ratio when set.
const TRANS_PER_FLEET = d.transporterPerFleet != null ? Number(d.transporterPerFleet) : null;
const SLOT_FALLBACK = Math.max(1, Number(d.slots || c.slots || 9));
// Fraction of the home hull stock actually split across the fleets (leaves a
// small buffer home). Default 1; config `hullSplitFrac` (e.g. 0.97).
const SPLIT_FRAC = d.hullSplitFrac != null ? Math.min(1, Math.max(0, Number(d.hullSplitFrac))) : 1;
const EVERY = Math.max(5, Number(d.intervalSec || 30)) * 1000;
const SPEED = String(Math.max(1, Math.min(10, Number(d.speed || 10))));
const TIME = String(Math.max(1, Math.min(10, Number(d.time || 1))));

const MAIN_FILE = path.join(DATA, `drain-main-${acc}.json`);
const LOG = path.join(DATA, `drain-${acc}.log`);
const log = (...a) => {
  const line = `[${new Date().toISOString().slice(11, 19)}] ${a.join(' ')}`;
  fs.appendFileSync(LOG, line + '\n');
  console.log(line);
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const num = (v) => Number(String(v ?? '').replace(/[^0-9]/g, '')) || 0;

function httpbot(args) {
  return execFileSync(process.execPath, ['--max-old-space-size=96', 'httpbot.mjs', ...args],
    { cwd: __dirname, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
}

function expState() {
  const out = httpbot(['exp-state']);
  const i = out.indexOf('{'), j = out.lastIndexOf('}');
  if (i < 0 || j < 0) throw new Error('exp-state returned no JSON');
  const st = JSON.parse(out.slice(i, j + 1));
  const real = (st.fleets || []).filter((f) => /Expedition/i.test(f.mission || '') && !/Hostail/i.test(f.mission || ''));
  return { active: real.length, slots: num(st.expeditionSlots) || SLOT_FALLBACK };
}

function refreshMain() {
  httpbot(['levels', '--cp', MAIN_CP, '--out', MAIN_FILE]);
  return JSON.parse(fs.readFileSync(MAIN_FILE, 'utf8')).ships || {};
}

// Frigates only, no fallback hull: pick the first configured hull with any
// ships home (config is `mainOrder: ["227"]`). When the Frigates run out the
// sender simply waits for returns — the plan is updated by hand once the total
// becomes too small.
function pickHull(ships) {
  for (const code of ORDER) if ((+ships[code] || 0) > 0) return code;
  return null;
}

// Even share of a home stock across the fleets being sent: the dynamic ratio
// wants `Math.round(hullPerFleet * perHull)` (at least 1, so a single unit still
// flies to trigger debris collection) but never promises more than
// `floor(home / nFleets)` — the sender stays honest as the auxiliary stock is
// attrited. Returns 0 when nothing is home (the fleet just flies without it).
function share(hullPerFleet, perHull, home, nFleets) {
  if (!(perHull > 0)) return 0; // ratio of 0 disables the auxiliary entirely
  const want = Math.max(1, Math.round(hullPerFleet * perHull));
  const avail = Math.floor((+home || 0) / nFleets);
  return Math.max(0, Math.min(want, avail));
}

// Divide the home hull stock EVENLY across the free slots (one fleet per open
// slot, `floor(have/free)` each) so slots always refill with roughly equal
// fleets — never one mega-fleet, never a 1-ship/aux-only fleet. Any remainder
// (< free) stays home for the next cycle. The recycler/transporter counts are
// dynamic: scaled off the per-fleet hull count and clamped to the even share of
// the home stock (config `recyclerPerHull`/`transporterPerHull`, or the legacy
// one-per-N `recyclerPer`/`transporterPer`).
function plan(ships, free) {
  const hull = pickHull(ships);
  if (!hull) return null;
  const have = +ships[hull] || 0;
  // Split only SPLIT_FRAC of the home stock (default 100%), floored so no
  // fractional ships; the remainder stays home as a buffer.
  const per = Math.floor((have * SPLIT_FRAC) / free);
  const nFleets = free;
  if (per < 1) return null; // not enough hulls home for a full share; wait
  const br = share(per, RECY_PER_HULL, ships[RECY], nFleets);
  const bt = TRANS_PER_FLEET != null
    ? Math.min(Math.max(1, Math.round(TRANS_PER_FLEET)), Math.floor((+ships[TRANS] || 0) / nFleets))
    : share(per, TRANS_PER_HULL, ships[TRANS], nFleets);
  // Never duplicate the hull or an auxiliary as a "1 each" small: the expo form
  // is keyed by ship code, so a later `207:1` would clobber `207:<per>` and fly
  // a 1-ship/aux-only fleet when the hull code is also in the small set.
  const smalls = SMALL.filter((code) => code !== hull && code !== RECY && code !== TRANS && (+ships[code] || 0) >= nFleets);
  const set = [`${hull}:${per}`];
  if (br > 0) set.push(`${RECY}:${br}`);
  if (bt > 0) set.push(`${TRANS}:${bt}`);
  for (const code of smalls) set.push(`${code}:1`);
  return { hull, per, nFleets, br, bt, set: set.join(',') };
}

async function main() {
  fs.mkdirSync(DATA, { recursive: true });
  log(`drain ${acc} start plan=${PLAN_VERSION} main=${MAIN_CP} ${MAIN_COORDS} order=${ORDER} recy=${RECY} ${RECY_PER_HULL}/hull trans=${TRANS} ${TRANS_PER_FLEET != null ? `${TRANS_PER_FLEET}/fleet` : `${TRANS_PER_HULL}/hull`} every=${EVERY / 1000}s${DRY ? ' [DRY]' : ''}`);
  for (;;) {
    try {
      const { active, slots } = expState();
      const free = slots - active;
      if (free <= 0) {
        log(`slots ${active}/${slots} busy; wait`);
        if (DRY) return;
        await sleep(EVERY);
        continue;
      }

      const ships = refreshMain();
      const p = plan(ships, free);
      if (!p) {
        const detail = ORDER.map((code) => `${code}=${+ships[code] || 0}`).join(' ');
        log(`no frigs home for a full share (${detail}); wait`);
        if (DRY) return;
        await sleep(EVERY);
        continue;
      }

      log(`fire ${p.nFleets} fleet(s): free=${free}/${slots} per=${p.per} br=${p.br} bt=${p.bt} :: ${p.set}`);
      if (DRY) {
        log(`dry: would POST ${p.nFleets}x "httpbot expedition ${p.set} 1 ${TIME} ${SPEED} --cp ${MAIN_CP}"`);
        return;
      }
      for (let k = 0; k < p.nFleets; k++) httpbot(['expedition', p.set, '1', TIME, SPEED, '--cp', MAIN_CP]);
    } catch (e) {
      log('ERR', e.message);
      if (DRY) return;
    }
    await sleep(EVERY);
  }
}

main();

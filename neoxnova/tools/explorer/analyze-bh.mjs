#!/usr/bin/env node
// analyze-bh.mjs — black-hole study. Joins every expedition the drain sent
// (data/expedition-runs.json: send time, fleet points, and the fleet's exact
// Arrival(destination)/Back timestamps when available) to its outcome message
// (data/expeditions.json + data/messages*.json), then tests whether the
// black-hole roll is a clean uniform draw or has a server-side pattern.
//
//   node analyze-bh.mjs [--calibrate] [--window-min N] [--tz-min N]
//
// Join keys, strongest first:
//   back    — the message resolves at the fleet's return-home time (exact, ±min)
//   arrival — the message resolves when the fleet reaches the expedition point
//   model   — legacy runs without either: send + a calibrated constant delay
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { fleetPoints, parseServerDate } from './parse.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const DATA = path.join(__dirname, 'data');
const arg = (k, d) => { const i = process.argv.indexOf(k); return i >= 0 ? Number(process.argv[i + 1]) : d; };
const CALIBRATE = process.argv.includes('--calibrate');
const WINDOW_EXACT = arg('--window-min', 3);   // exact-key (back/arrival) tolerance
const WINDOW_MODEL = arg('--model-window-min', 8); // legacy constant-delay tolerance
const TZ_MIN = arg('--tz-min', 180);           // server clock = UTC+3; VM is UTC
const OUT = path.join(DATA, 'bh-study.json');

function readJson(p, dflt) { try { return JSON.parse(fs.readFileSync(p, 'utf8')); } catch { return dflt; } }
function loadMessages() {
  const all = {};
  for (const f of ['expeditions.json', 'messages.json', 'messages-bwlean.json'])
    for (const m of readJson(path.join(DATA, f), {}).messages || []) all[String(m.id)] = m;
  return Object.values(all);
}
const isFatal = (m) => m.outcome === 'blackhole' || m.flavor === 'lost-fleet';
const isLoot = (m) => m.flavor === 'blackhole-loot';
const isOutcome = (m) => (m.outcome && m.outcome !== 'return') || m.kind === 'outcome';

// --- load ---------------------------------------------------------------
const runs = readJson(path.join(DATA, 'expedition-runs.json'), [])
  .filter((r) => r && r.at && r.ok !== false)
  .map((r) => {
    const t = Date.parse(r.at);
    const back = r.back ? parseServerDate(r.back, TZ_MIN) : null;
    const arrival = r.arrival ? parseServerDate(r.arrival, TZ_MIN) : null;
    return {
      ...r, t, pts: r.points != null ? r.points : fleetPoints(r.ships),
      backT: back, arrT: arrival, keyType: back ? 'back' : arrival ? 'arrival' : 'model',
    };
  })
  .filter((r) => Number.isFinite(r.t))
  .sort((a, b) => a.t - b.t);

const outcomes = loadMessages()
  .map((m) => ({ m, t: parseServerDate(m.date, TZ_MIN), fatal: isFatal(m), loot: isLoot(m) }))
  .filter((o) => o.t != null && isOutcome(o.m))
  .sort((a, b) => a.t - b.t);

const nKey = { back: runs.filter((r) => r.keyType === 'back').length, arrival: runs.filter((r) => r.keyType === 'arrival').length, model: runs.filter((r) => r.keyType === 'model').length };
console.log(`[bh-study] runs=${runs.length} (ok)  outcome messages=${outcomes.length}`);
console.log(`[bh-study] join keys: back=${nKey.back} arrival=${nKey.arrival} model(legacy)=${nKey.model}`);

// --- calibrate the legacy constant delay D ------------------------------
function deltaHistogram(predOf) {
  const h = new Map();
  for (const o of outcomes) for (const r of runs) {
    const p = predOf(r); if (p == null) continue;
    const d = Math.round((o.t - p) / 60000);
    if (d >= -30 && d <= 300) h.set(d, (h.get(d) || 0) + 1);
  }
  return [...h.entries()].sort((a, b) => b[1] - a[1]);
}
// For runs without exact keys, recover the constant delay from (outcome - send).
const modelHist = deltaHistogram((r) => (r.keyType === 'model' ? r.t : null));
const D = modelHist.length ? modelHist[0][0] : 30; // minutes
if (CALIBRATE) {
  console.log('\n[calibrate] legacy (outcome - send) top deltas — true delay is the peak:');
  for (const [d, n] of modelHist.slice(0, 10)) console.log(`   ${String(d).padStart(4)} min : ${n}`);
  for (const key of ['back', 'arrival']) {
    const h = deltaHistogram((r) => (r.keyType === key ? (key === 'back' ? r.backT : r.arrT) : null));
    if (h.length) { console.log(`\n[calibrate] outcome - ${key} (exact-key runs):`); for (const [d, n] of h.slice(0, 6)) console.log(`   ${String(d).padStart(4)} min : ${n}`); }
  }
  console.log(`\n[calibrate] legacy D=${D}min; re-run without --calibrate to analyse.`);
  process.exit(0);
}

// --- join (two passes: exact keys first, then model runs) ----------------
const predExact = (r) => (r.keyType === 'back' ? r.backT : r.keyType === 'arrival' ? r.arrT : null);
const predModel = (r) => (r.keyType === 'model' ? r.t + D * 60000 : null);
const matched = [];
function pass(predOf, winMin, keyLabel) {
  const pairs = [];
  for (let oi = 0; oi < outcomes.length; oi++) {
    if (matched.some((p) => p.oi === oi)) continue;
    for (let ri = 0; ri < runs.length; ri++) {
      if (matched.some((p) => p.ri === ri)) continue;
      const p = predOf(runs[ri]); if (p == null) continue;
      const resid = Math.abs(outcomes[oi].t - p) / 60000;
      if (resid <= winMin) pairs.push({ oi, ri, resid });
    }
  }
  pairs.sort((a, b) => a.resid - b.resid);
  const uO = new Set(), uR = new Set();
  for (const p of pairs) { if (uO.has(p.oi) || uR.has(p.ri)) continue; uO.add(p.oi); uR.add(p.ri); matched.push({ ...p, keyType: keyLabel }); }
  return uO.size;
}
const nBackArr = pass(predExact, WINDOW_EXACT, 'exact');
const nModel = pass(predModel, WINDOW_MODEL, 'model');
const resids = matched.map((p) => p.resid).sort((a, b) => a - b);
const q = (p) => resids.length ? resids[Math.floor(p * (resids.length - 1))] : NaN;
console.log(`\n[join] matched ${matched.length}/${outcomes.length} outcomes (exact=${nBackArr} via back/arrival, model=${nModel})`);
console.log(`[join] residual min/med/p95/max = ${q(0).toFixed(1)}/${q(0.5).toFixed(1)}/${q(0.95).toFixed(1)}/${(resids[resids.length - 1] || 0).toFixed(1)} min`);
const mbh = matched.filter((p) => outcomes[p.oi].fatal);
console.log(`[join] matched fatal BH=${mbh.length}  blackhole-loot=${matched.filter((p) => outcomes[p.oi].loot).length}`);
const reliable = nKey.back + nKey.arrival > 0;
if (!reliable) console.log('[join] NOTE: no exact-key runs yet — size/hour tests below use the weaker modeled join.');

// --- stats helpers ------------------------------------------------------
function nCkLog(n, k) { if (k < 0 || k > n) return -Infinity; let s = 0; for (let i = 1; i <= k; i++) s += Math.log(n - k + i) - Math.log(i); return s; }
function binomSF(k, n, p) { if (k <= 0) return 1; let s = 0; for (let i = k; i <= n; i++) s += Math.exp(nCkLog(n, i) + i * Math.log(p) + (n - i) * Math.log(1 - p)); return Math.min(1, s); }
function lgamma(z) {
  const g = [76.18009172947146, -86.50532032941677, 24.01409824083091, -1.231739572450155, 0.1208650973866179e-2, -0.5395239384953e-5];
  let y = z, x = z, tmp = x + 5.5; tmp -= (x + 0.5) * Math.log(tmp);
  let ser = 1.000000000190015; for (let j = 0; j < 6; j++) ser += g[j] / ++y;
  return -tmp + Math.log(2.5066282746310005 * ser / x);
}
function gammaq(a, x) {
  const EPS = 1e-12, ITMAX = 200;
  if (x < a + 1) { let ap = a, sum = 1 / a, del = sum; for (let n = 1; n <= ITMAX; n++) { ap++; del *= x / ap; sum += del; if (Math.abs(del) < Math.abs(sum) * EPS) break; } return 1 - sum * Math.exp(-x + a * Math.log(x) - lgamma(a)); }
  let b = x + 1 - a, c = 1e30, d = 1 / b, h = d;
  for (let i = 1; i <= ITMAX; i++) { const an = -i * (i - a); b += 2; d = an * d + b; if (Math.abs(d) < 1e-30) d = 1e-30; c = b + an / c; if (Math.abs(c) < 1e-30) c = 1e-30; d = 1 / d; const del = d * c; h *= del; if (Math.abs(del - 1) < EPS) break; }
  return Math.exp(-x + a * Math.log(x) - lgamma(a)) * h;
}
const chi2sf = (x, df) => gammaq(df / 2, x / 2);
function chi2Uniform(counts) { const n = counts.reduce((a, b) => a + b, 0), k = counts.length, e = n / k; if (!n) return { chi2: 0, df: k - 1, p: 1 }; let c = 0; for (const o of counts) c += (o - e) ** 2 / e; return { chi2: c, df: k - 1, p: chi2sf(c, k - 1) }; }
const hourHist = (ts) => { const h = Array(24).fill(0); for (const t of ts) h[new Date(t).getUTCHours()]++; return h; };

const out = { generatedAt: new Date().toISOString(), D, windowExact: WINDOW_EXACT, windowModel: WINDOW_MODEL, tzMin: TZ_MIN, runs: runs.length, outcomes: outcomes.length, matched: matched.length, joinKeys: nKey };

// --- 1) overall rate ----------------------------------------------------
const nOut = outcomes.length, nFatal = outcomes.filter((o) => o.fatal).length, nLootCount = outcomes.filter((o) => o.loot).length;
console.log(`\n== overall ==`);
console.log(`fatal BH ${nFatal}/${nOut} = ${(100 * nFatal / nOut).toFixed(2)}%  (p0=1.68% -> P(X>=k) = ${binomSF(nFatal, nOut, 0.0168).toFixed(3)})`);
console.log(`BH-loot  ${nLootCount}/${nOut} = ${(100 * nLootCount / nOut).toFixed(2)}%`);
out.overall = { outcomes: nOut, fatal: nFatal, loot: nLootCount, fatalPct: 100 * nFatal / nOut, pVsModel: binomSF(nFatal, nOut, 0.0168) };

// --- 2) rate by resolve-hour (UTC) --------------------------------------
const bhHours = hourHist(outcomes.filter((o) => o.fatal).map((o) => o.t));
const hc = chi2Uniform(bhHours);
console.log(`\n== fatal BH by resolve hour (UTC) ==\n${bhHours.join(' ')}`);
console.log(`chi2=${hc.chi2.toFixed(1)} df=${hc.df} p=${hc.p.toFixed(3)} ${hc.p < 0.05 ? '<-- NOT uniform' : '(consistent with uniform)'}`);
out.byHour = { hist: bhHours, chi2: hc.chi2, p: hc.p };

// --- 3) weak-seed test: same send-minute -> same outcome? ---------------
// If the server pre-rolls at send with a coarse time-based seed (or batches a
// burst through one roll), fleets launched together share their outcome. Group
// matched runs by send-minute (the drain fires bursts) and measure BH-agreement.
function agreement(groups) {
  const multi = groups.filter((g) => g.length >= 2);
  const agree = multi.filter((g) => g.every((x) => x === g[0])).length;
  const exp = multi.reduce((a, g) => { const p = g.reduce((s, x) => s + x, 0) / g.length; return a + (p * p + (1 - p) * (1 - p)); }, 0);
  return { groups: multi.length, agree, exp };
}
function groupsBy(bucketMs) {
  const m = new Map();
  for (const p of matched) { const k = Math.floor(runs[p.ri].t / bucketMs); if (!m.has(k)) m.set(k, []); m.get(k).push(outcomes[p.oi].fatal ? 1 : 0); }
  return [...m.values()];
}
const gMin = agreement(groupsBy(60000));
const gSec = agreement(groupsBy(1000));
console.log(`\n== weak-seed test (fleet sent together -> shared outcome?) ==`);
console.log(`send-minute groups>=2: ${gMin.groups}  all-agree: ${gMin.agree} (independent expectation: ${gMin.exp.toFixed(1)})`);
console.log(`send-second groups>=2: ${gSec.groups}  all-agree: ${gSec.agree} (independent expectation: ${gSec.exp.toFixed(1)})`);
const seedFlag = (gMin.agree > gMin.exp + 2) || (gSec.agree > gSec.exp + 1);
console.log(seedFlag ? '  <-- more agreement than chance: possible weak/batched send-time seed' : '  (no excess same-batch agreement)');
out.weakSeed = { sendMinute: gMin, sendSecond: gSec };

// --- 4) rate vs fleet points -------------------------------------------
const pts = runs.map((r) => r.pts).filter((x) => x > 0).sort((a, b) => a - b);
if (pts.length >= 8) {
  const bins = 4, edges = [];
  for (let i = 1; i < bins; i++) edges.push(pts[Math.floor(i * pts.length / bins)]);
  const buckets = Array.from({ length: bins }, () => ({ n: 0, bh: 0, lo: 0, hi: 0 }));
  for (const p of matched) {
    const P = runs[p.ri].pts; let b = 0; while (b < edges.length && P >= edges[b]) b++;
    buckets[b].n++; if (outcomes[p.oi].fatal) buckets[b].bh++;
  }
  for (let i = 0; i < bins; i++) { buckets[i].lo = i === 0 ? pts[0] : edges[i - 1]; buckets[i].hi = i === bins - 1 ? pts[pts.length - 1] : edges[i]; }
  console.log(`\n== fatal BH rate vs fleet points (matched) ==`);
  for (const b of buckets) console.log(`  ${(b.lo / 1e9).toFixed(1)}–${(b.hi / 1e9).toFixed(1)}B pts : ${b.bh}/${b.n} = ${(100 * b.bh / (b.n || 1)).toFixed(1)}%`);
  out.byPoints = buckets;
}

// --- 5) resolve-minute clustering --------------------------------------
const mins = new Map();
for (const o of outcomes.filter((x) => x.fatal)) { const k = Math.floor(o.t / 60000); mins.set(k, (mins.get(k) || 0) + 1); }
const collisions = [...mins.values()].filter((c) => c >= 2).reduce((a, c) => a + c, 0);
console.log(`\n== resolve-minute clustering (fatal) ==`);
console.log(`${nFatal} BHs in ${mins.size} distinct minutes; ${collisions} landed in a shared minute`);
out.minuteClusters = { distinctMinutes: mins.size, sharingAMinute: collisions };

// --- 6) BH-family adjacent same-minute ----------------------------------
const fam = outcomes.filter((o) => o.fatal || o.loot).map((o) => Math.floor(o.t / 60000)).sort((a, b) => a - b);
let adjSame = 0; for (let i = 1; i < fam.length; i++) if (fam[i] === fam[i - 1]) adjSame++;
console.log(`BH-family adjacent same-minute events: ${adjSame}/${fam.length - 1} gaps`);
out.familySameMinute = { events: fam.length, adjacentSameMinute: adjSame };

fs.writeFileSync(OUT, JSON.stringify(out, null, 2));
console.log(`\n[bh-study] wrote ${OUT}`);

// httpbot.mjs — browser-less niburu client (Node fetch + cookie jar).
// Same parsers as explorer.mjs, but no Chromium, so it fits a tiny VM (~40 MB).
//
// Commands:
//   node httpbot.mjs dump "page=research"      # print fetched HTML (debug)
//   node httpbot.mjs planets                   # list planets (id/name/coords)
//   node httpbot.mjs levels [--out file] [--cp id]  # current (or chosen) planet levels JSON
//   node httpbot.mjs simsuite scenarios.json [--out dir]  # batch battle-sim tests
//   node httpbot.mjs card <code>               # parse one unit information card (class fields)
//   node httpbot.mjs cards [--out file]        # fetch every ship/defense card -> data/unit-info.json
//   node httpbot.mjs arsenal                   # list upgrades (bonus, next bracket, available, greid)
//   node httpbot.mjs market                    # list live market lots (id, name, amount, price)
//   node httpbot.mjs activate <greid> [--go]   # activate one upgrade drawing (dry unless --go)
//   node httpbot.mjs sell <type> <amt> <rate> [--go]  # list drawings on the market (dry unless --go)
//   node httpbot.mjs phalanx <g:s:p> [1|3] [--cp <moonCp>]  # moon Phalanx scan
//   node httpbot.mjs msg-scan [--cats 0,3,15] [--max-sites 40]  # scan all message categories -> data/messages.json
//   node httpbot.mjs msg-stats                                 # reprint stats from data/messages.json
//   node httpbot.mjs resolve --goals f.json [--steps N] [--cp id]
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseBuildPage, parseTechtreeGraph, parseQueue, parseCombatReport, parseInfoCard, parseArsenalPage, parseMarketLots, parsePhalanx, stripTags, num } from './parse.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const SECRETS = path.resolve(__dirname, '../../secrets/explorer.env');
const DATA_DIR = path.join(__dirname, 'data');

function loadEnv(file) {
  const out = {};
  if (!fs.existsSync(file)) return out;
  for (const line of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
    const t = line.trim();
    if (!t || t.startsWith('#')) continue;
    const i = t.indexOf('=');
    if (i > 0) out[t.slice(0, i).trim()] = t.slice(i + 1).trim().replace(/^["']|["']$/g, '');
  }
  return out;
}
const env = loadEnv(SECRETS);
for (const [k, v] of Object.entries(process.env)) if ((k.startsWith('EXPLORER_') || k === 'NIBURU_BASE_URL') && v != null) env[k] = v;
const BASE = (env.NIBURU_BASE_URL || 'https://niburuspace.com').replace(/\/$/, '');
const USER = env.NIBURU_USER, PASS = env.NIBURU_PASS;
const UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36';
const MIN_DELAY = Number(env.EXPLORER_MIN_DELAY_MS || 400);
const MAX_DELAY = Number(env.EXPLORER_MAX_DELAY_MS || 900);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const delay = () => sleep(MIN_DELAY + Math.random() * (MAX_DELAY - MIN_DELAY));

const jar = new Map();
const cookieHeader = () => [...jar].map(([k, v]) => `${k}=${v}`).join('; ');
function storeCookies(res) {
  const sc = res.headers.getSetCookie ? res.headers.getSetCookie() : [];
  for (const c of sc) {
    const pair = c.split(';')[0];
    const i = pair.indexOf('=');
    if (i > 0) jar.set(pair.slice(0, i).trim(), pair.slice(i + 1).trim());
  }
}

// Persist the session cookie so restarted processes (and concurrent per-planet
// workers for the same account) reuse one login instead of re-authenticating
// every invocation. Auto-relogin happens only when a request proves we're out.
let sessionReady = false;
const SESSION_FILE = env.EXPLORER_SESSION_FILE || path.join(DATA_DIR, `session-${String(USER).replace(/[^\w.-]/g, '_')}.json`);
function loadSession() { try { const j = JSON.parse(fs.readFileSync(SESSION_FILE, 'utf8')); for (const [k, v] of Object.entries(j)) jar.set(k, v); } catch {} }
function saveSession() { try { fs.mkdirSync(path.dirname(SESSION_FILE), { recursive: true }); fs.writeFileSync(SESSION_FILE, JSON.stringify(Object.fromEntries(jar))); } catch {} }

async function raw(url, opts = {}) {
  const headers = { 'User-Agent': UA, 'Accept-Language': 'de-DE,de;q=0.9,en;q=0.8' };
  if (jar.size) headers.Cookie = cookieHeader();
  Object.assign(headers, opts.headers || {});
  // Hard timeout so a stalled connection can never hang the daemon forever.
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), Number(env.EXPLORER_FETCH_TIMEOUT_MS || 30000));
  try {
    return await fetch(url, { ...opts, headers, redirect: 'manual', signal: ctrl.signal });
  } finally { clearTimeout(t); }
}

async function getUrl(url, depth = 0) {
  const res = await raw(url);
  storeCookies(res);
  if ([301, 302, 303, 307, 308].includes(res.status) && depth < 6) {
    const loc = res.headers.get('location');
    if (loc) return getUrl(loc.startsWith('http') ? loc : new URL(loc, url).href, depth + 1);
  }
  return res;
}
const pageUrl = (q) => `${BASE}/game/game.php?${q}`;
const getPage = (q) => getUrl(pageUrl(q));

async function login(force = false) {
  if (sessionReady && !force) return;
  // Reuse a persisted session when possible (no login POST).
  if (!force && jar.size === 0) loadSession();
  if (!force && jar.size) {
    try {
      const g = await getUrl(`${BASE}/game.php`);
      const html = await g.text();
      if (/game\.php/.test(g.url) || /page=overview|current_metal/.test(html)) { sessionReady = true; return html; }
    } catch {}
  }
  await getUrl(`${BASE}/`);
  await delay();
  const url = `${BASE}/index.php?page=login&mode=send&username=${encodeURIComponent(USER)}&password=${encodeURIComponent(PASS)}&remember=1`;
  const res = await getUrl(url);
  let json = null;
  try { json = await res.json(); } catch {}
  if (json && json.error) throw new Error(`Login rejected: ${json.message}`);
  await delay();
  const g = await getUrl(`${BASE}/game.php`);
  const html = await g.text();
  if (!/game\.php/.test(g.url) && !/page=overview|current_metal/.test(html)) throw new Error('not logged in');
  sessionReady = true;
  saveSession();
  return html;
}

async function postForm(q, form) {
  const url = pageUrl(q);
  const res = await raw(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded', Referer: url },
    body: new URLSearchParams(form).toString(),
  });
  storeCookies(res);
  if ([301, 302, 303, 307, 308].includes(res.status)) {
    const loc = res.headers.get('location');
    if (loc) return (await getUrl(loc.startsWith('http') ? loc : new URL(loc, url).href)).text();
  }
  return res.text();
}

function resourcesFromHtml(html) {
  const grab = (id) => { const m = new RegExp(`id="${id}"[^>]*>([\\d.]+)`).exec(html); return m ? num(m[1]) : 0; };
  return { metal: grab('current_metal'), crystal: grab('current_crystal'), deuterium: grab('current_deuterium') };
}

// Ship/defense counts live in id="val_<code>" (parseBuildPage only reads levels).
function unitsFromHtml(html) {
  const out = {};
  for (const m of html.matchAll(/id="val_(\d+)"[^>]*>([\d.]+)/g)) out[m[1]] = num(m[2]);
  return out;
}

async function scope(query) {
  const res = await getPage(query);
  const html = await res.text();
  // A logged-out page has no build boxes; flag it so the worker re-authenticates.
  if (!/class="build_box/.test(html) && !/Lack of energy|Free energy/.test(html)) { sessionReady = false; throw new Error('not logged in'); }
  const items = parseBuildPage(html);
  const levels = {}, nameToCode = {};
  for (const it of items) { levels[it.code] = it.level; nameToCode[it.name] = it.code; }
  const queued = {};
  for (const q of parseQueue(html)) { const c = nameToCode[q.name]; if (c) queued[c] = (queued[c] || 0) + 1; }
  return { html, byCode: Object.fromEntries(items.map((it) => [it.code, it])), levels, queued, res: resourcesFromHtml(html) };
}

async function cmdDump(arg) {
  await login();
  const res = await getPage(arg);
  const html = await res.text();
  console.log(html);
}

// GET an arbitrary path under /game/ (e.g. scripts/game/message.js) for debugging.
async function cmdGet(p) {
  await login();
  const res = await getUrl(`${BASE}/game/${p}`);
  console.log(await res.text());
}

async function cmdLevels(outArg, cpArg) {
  await login();
  const suffix = cpArg ? `&cp=${cpArg}` : '';
  const B = await scope('page=buildings' + suffix);
  const R = await scope('page=research' + suffix);
  const S = await scope('page=shipyard&mode=fleet' + suffix);
  const D = await scope('page=shipyard&mode=defense' + suffix);
  const planet = (() => {
    const m = /class="active_urlpalnet" url="cp=(\d+)"[\s\S]*?name_palnet">([^<]*)<[\s\S]*?coordinates_palnet">\[([^\]]*)\]/.exec(B.html);
    return m ? { id: Number(m[1]), name: m[2].trim(), coords: m[3] } : null;
  })();
  const data = {
    updatedAt: new Date().toISOString(), account: USER, planet,
    buildings: B.levels, research: R.levels, ships: unitsFromHtml(S.html), defenses: unitsFromHtml(D.html), resources: B.res,
    names: Object.fromEntries([...Object.values(B.byCode), ...Object.values(R.byCode), ...Object.values(S.byCode), ...Object.values(D.byCode)].map((it) => [it.code, it.name])),
  };
  const out = outArg || 'data/levels.json';
  fs.mkdirSync(path.dirname(out), { recursive: true });
  fs.writeFileSync(out, JSON.stringify(data, null, 2));
  console.log(`WROTE ${out}`);
  console.log(JSON.stringify(data));
}

async function cmdPlanets() {
  await login();
  const html = await (await getPage('page=overview')).text();
  const planets = [];
  for (const m of html.matchAll(/class="(active_)?urlpalnet" url="cp=(\d+)"[\s\S]*?name_palnet">([^<]*)<[\s\S]*?coordinates_palnet">\[([^\]]*)\]/g)) {
    planets.push({ id: Number(m[2]), name: m[3].trim(), coords: m[4], active: !!m[1] });
  }
  console.log(JSON.stringify({ account: USER, planets }, null, 2));
  return planets;
}

// Fetch one unit's information card and parse its class fields (weapon type,
// structural armor, shield and engine classes + base stats). Used to build the
// per-unit upgrade-class mapping in internal/game/unit_classes.go.
async function cmdCard(code) {
  if (!code) throw new Error('need a unit code (e.g. 204)');
  await login();
  const html = await (await getPage(`page=information&id=${code}`)).text();
  const card = parseInfoCard(html);
  console.log(JSON.stringify({ code: Number(code), ...card }, null, 2));
  return card;
}

// Fetch every ship (202-228) and defense (401-419) information card and write
// data/unit-info.json. This is the raw input for unit-classes-dataset.mjs.
async function cmdCards(outArg) {
  await login();
  const ids = [];
  for (let i = 202; i <= 228; i++) ids.push(i);
  for (let i = 401; i <= 419; i++) ids.push(i);

  const units = {};
  for (const id of ids) {
    try {
      const html = await (await getPage(`page=information&id=${id}`)).text();
      if (!/Structural armor/i.test(html)) { console.log(`[skip] ${id} (no stat card)`); continue; }
      const u = parseInfoCard(html);
      units[id] = u;
      console.log(`[ok] ${id} ${u.name}: weapon=${u.weaponType} armor=${u.armorClass} shield=${u.shieldClass} engine=${u.engineClass}`);
    } catch (e) { console.log(`[ERR] ${id}: ${e.message}`); }
    await delay();
  }

  const out = outArg || path.join(DATA_DIR, 'unit-info.json');
  fs.mkdirSync(path.dirname(out), { recursive: true });
  fs.writeFileSync(out, JSON.stringify({ updatedAt: new Date().toISOString(), account: USER, units }, null, 2));
  console.log(`WROTE ${out} (${Object.keys(units).length} units)`);
  return units;
}

// Arsenal upgrade list (game.php?page=arsenal): current bonus, next-activation
// bracket, held drawings and the activate `greid` key for each upgrade.
async function cmdArsenal() {
  await login();
  const html = await (await getPage('page=arsenal')).text();
  const upgrades = parseArsenalPage(html);
  console.log(JSON.stringify({ account: USER, count: upgrades.length, upgrades }, null, 2));
  return upgrades;
}

// Live market lots (game.php?page=market): id, upgrade, amount, total Antimatter.
async function cmdMarket() {
  await login();
  const html = await (await getPage('page=market')).text();
  const lots = parseMarketLots(html);
  console.log(JSON.stringify({ account: USER, count: lots.length, lots }, null, 2));
  return lots;
}

// Activate one upgrade drawing (POST page=arsenal mode=send greid=<key>).
// Dry by default (shows the held drawing it would consume); pass --go to post.
// The `greid` key is the live internal name, not the 1..19 market `type` id.
async function cmdActivate(greid, go) {
  if (!greid) throw new Error('need a greid key (e.g. combustion)');
  await login();
  if (!go) {
    const html = await (await getPage('page=arsenal')).text();
    const item = parseArsenalPage(html).find((u) => u.greid === greid);
    console.log(item
      ? `[activate] DRY greid=${greid} (${item.name}, have ${item.available}, next +${item.nextBonus})`
      : `[activate] DRY greid=${greid} (not currently activatable; no owned drawing or unknown key)`);
    return;
  }
  const html = await postForm('page=arsenal', { mode: 'send', greid });
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(path.join(DATA_DIR, 'arsenal-activate.html'), html);
  const after = parseArsenalPage(html).find((u) => u.greid === greid);
  const txt = stripTags(html.replace(/<script[\s\S]*?<\/script>/gi, '')).replace(/\s+/g, ' ');
  console.log(`[activate] greid=${greid} posted${after ? ` -> bonus ${after.bonus}%, have ${after.available}` : ''}`);
  console.log(`[activate] ${txt.slice(0, 300)}`);
}

// List (sell) upgrade drawings on the market (POST page=market mode=sellUpgrades).
// `type` is the 1..19 market id, `rate` the Antimatter price per unit.
// Dry by default; pass --go to post. Usage: sell <type> <amount> <rate> [--go]
async function cmdSell(type, amount, rate, go) {
  if (!type) throw new Error('need a type 1..19');
  const form = { mode: 'sellUpgrades', type: String(type), amount: String(amount || 1), rate: String(rate || 500) };
  await login();
  if (!go) { console.log(`[sell] DRY ${JSON.stringify(form)}`); return; }
  const html = await postForm('page=market', form);
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(path.join(DATA_DIR, 'market-sell.html'), html);
  const txt = stripTags(html.replace(/<script[\s\S]*?<\/script>/gi, '')).replace(/\s+/g, ' ');
  console.log(`[sell] posted ${JSON.stringify(form)}`);
  console.log(`[sell] ${txt.slice(0, 300)}`);
}

// Conveyor probe: measure how the shipyard/defense "Building: N per second"
// throughput changes as conveyors (71 light / 72 average / 73 heavy) gain
// levels. Usage: httpbot.mjs conveyor-probe [cp] [rounds] [codesCSV]
// e.g. `conveyor-probe 1695 2 72` levels only the Average conveyor.
async function cmdConveyorProbe(cpArg, roundsArg, codesArg) {
  const cp = cpArg || '1695';
  const rounds = Math.max(1, Number(roundsArg || 3));
  const CONVEYORS = String(codesArg || '71,72,73').split(',').map((s) => Number(s.trim())).filter(Boolean);
  await login();
  const cpq = `&cp=${cp}`;

  const perSec = (sc) => Object.fromEntries(Object.values(sc.byCode).map((it) => [it.code, it.perSec]));
  const snapshot = async () => {
    const B = await scope('page=buildings' + cpq);
    const S = await scope('page=shipyard&mode=fleet' + cpq);
    const D = await scope('page=shipyard&mode=defense' + cpq);
    return {
      at: new Date().toISOString(),
      conveyors: Object.fromEntries(CONVEYORS.map((c) => [c, B.levels[c] || 0])),
      fleet: perSec(S),
      defense: perSec(D),
      resources: B.res,
    };
  };
  const waitLevels = async (want) => {
    for (let i = 0; i < 60; i++) {
      const B = await scope('page=buildings' + cpq);
      if (CONVEYORS.every((c) => (B.levels[c] || 0) >= want[c])) return true;
      await sleep(3000);
    }
    return false;
  };

  let prev = await snapshot();
  const baseline = prev;
  console.log(`[probe] cp=${cp} baseline conveyors=${JSON.stringify(prev.conveyors)}`);
  console.log(`[probe] baseline fleet perSec=${JSON.stringify(prev.fleet)}`);
  console.log(`[probe] baseline defense perSec=${JSON.stringify(prev.defense)}`);

  const results = [];
  for (let r = 1; r <= rounds; r++) {
    const want = {};
    for (const c of CONVEYORS) want[c] = (prev.conveyors[c] || 0) + 1;
    for (const c of CONVEYORS) {
      await delay();
      await postForm('page=buildings' + cpq, { cmd: 'insert', building: c, lvlup: want[c] });
      console.log(`[probe] queued conveyor ${c} -> L${want[c]}`);
    }
    const ok = await waitLevels(want);
    const cur = await snapshot();
    if (!ok) console.log(`[probe] WARN round ${r}: levels not reached; got ${JSON.stringify(cur.conveyors)}`);
    const delta = (a, b) => {
      const d = {};
      for (const k of Object.keys(b)) if (b[k] !== a[k]) d[k] = `${a[k]} -> ${b[k]}`;
      return d;
    };
    console.log(`[probe] round ${r} conveyors=${JSON.stringify(cur.conveyors)}`);
    console.log(`[probe]   fleet   delta=${JSON.stringify(delta(prev.fleet, cur.fleet))}`);
    console.log(`[probe]   defense delta=${JSON.stringify(delta(prev.defense, cur.defense))}`);
    results.push({ round: r, conveyors: cur.conveyors, fleet: cur.fleet, defense: cur.defense });
    prev = cur;
  }
  fs.mkdirSync(DATA_DIR, { recursive: true });
  const out = path.join(DATA_DIR, `conveyor-probe-${cp}.json`);
  fs.writeFileSync(out, JSON.stringify({ cp, account: USER, baseline, rounds: results }, null, 2));
  console.log(`WROTE ${out}`);
}

// Energy balance shown on the buildings page: negative = lack of energy.
function energyFromHtml(html) {
  const lack = /Lack of energy:\s*(\d+)%/.exec(html);
  if (lack) return -parseInt(lack[1], 10);
  const free = /Free energy:\s*(\d+)%/.exec(html);
  if (free) return parseInt(free[1], 10);
  return null;
}

async function cmdResolve(goalsPath, steps, cpArg) {
  const goals = JSON.parse(fs.readFileSync(goalsPath, 'utf8'));
  const cpq = cpArg ? `&cp=${cpArg}` : '';
  await login();
  const tt = await (await getPage('page=techtree' + cpq)).text();
  const graph = parseTechtreeGraph(tt);
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(path.join(DATA_DIR, 'techtree-graph.json'), JSON.stringify(graph, null, 2));
  console.log(`[graph] ${Object.keys(graph).length} items`);

  // Goals: {buildings:{code:lvl}, research:{code:lvl}}. `order` (array of codes)
  // gives explicit priority — JSON objects sort integer-like keys ascending, so
  // the map order alone would put mines/solar before Nanite.
  const goalTargets = new Map();
  for (const kind of ['buildings', 'research']) {
    for (const [c, l] of Object.entries(goals[kind] || {})) {
      const k = String(c);
      goalTargets.set(k, Math.max(goalTargets.get(k) || 0, l));
    }
  }
  const ordered = [];
  const seen = new Set();
  for (const c of (goals.order || [])) {
    const k = String(c);
    if (goalTargets.has(k) && !seen.has(k)) { ordered.push(k); seen.add(k); }
  }
  for (const k of goalTargets.keys()) if (!seen.has(k)) { ordered.push(k); seen.add(k); }

  // "gradual": economy codes bumped +1 every time the whole plan is satisfied,
  // so a long-running daemon keeps raising production instead of exiting.
  // "bumpBuilders" (Robot/Nanite) only grow when the metal mine starts taking
  // >= threshold seconds, since those facilities get expensive very fast.
  const gradual = (goals.gradual || []).map(String);
  // Optional per-code ceiling for `gradual` (else the global gradualCap applies).
  const caps = goals.caps || {};
  const bumpBuilders = (goals.bumpBuilders || []).map(String);
  // Research codes in `allowSlow` bypass the <=1s Duration gate (short-but-not-
  // instant techs we explicitly want, e.g. weapons/shield/armour for recyclers).
  const allowSlow = new Set((goals.allowSlow || []).map(String));
  // Ship/defense build targets: ships go to the fleet page, defenses to the
  // defense page. Counts are read from `val_<code>` on the respective page.
  // Rebuilt from the plan file every loop (see below): a long-lived worker must
  // follow planner updates (e.g. the HC/BB ratio gate flipping) instead of
  // building the targets it locked in when the process started.
  const unitTargetsFrom = (g) => {
    const out = [];
    for (const [c, t] of Object.entries(g.ships || {})) out.push({ code: String(c), target: Number(t), scope: 'fleet' });
    for (const [c, t] of Object.entries(g.defenses || {})) out.push({ code: String(c), target: Number(t), scope: 'defense' });
    return out;
  };
  let unitTargets = unitTargetsFrom(goals);
  const unitBatch = Number(env.EXPLORER_UNIT_BATCH || 100);
  // Rate-aware batching: size each unit order to ~this many seconds of the
  // shipyard's "Building: N per second" throughput, then re-submit just after it
  // completes (see the unit post below). 0 = off (use unitBatch only).
  const unitSeconds = Number(env.EXPLORER_UNIT_SECONDS || 0);
  const unitVal = (html, c) => { const m = new RegExp('id="val_' + c + '"[^>]*>([\\d.]+)').exec(html); return m ? parseInt(m[1].replace(/[^\d]/g, ''), 10) : 0; };
  const unitAvail = (html, c) => new RegExp('name="fmenge\\[' + c + '\\]"').test(html);
  for (const c of [...gradual, ...bumpBuilders]) if (!goalTargets.has(c)) goalTargets.set(c, 0);
  for (const c of [...gradual, ...bumpBuilders]) if (!seen.has(c)) { ordered.push(c); seen.add(c); }
  const gradualCap = Number(env.EXPLORER_GRADUAL_CAP || 50);
  const gradualWaitMs = Number(env.EXPLORER_GRADUAL_WAIT_MS || 30000);
  const builderBumpSec = Number(env.EXPLORER_BUILDER_BUMP_SEC || 300);

  // 0 = wait forever (Recommended for long-lived daemon runs on this fast server).
  const maxStalls = Number(env.EXPLORER_MAX_STALLS || 0);
  const waitMs = Number(env.EXPLORER_QUEUE_WAIT_MS || 5000);
  const maxBuild = Number(env.EXPLORER_MAX_BUILD_QUEUE || 2);
  const maxResearch = Number(env.EXPLORER_MAX_RESEARCH_QUEUE || 1);
  const satBatch = Number(env.EXPLORER_ENERGY_SATS || 200);
  const satCooldownMs = Number(env.EXPLORER_SAT_COOLDOWN_MS || 300000);
  // Only auto-queue research that is (near) instant; long techs need University.
  const maxResearchSec = Number(env.EXPLORER_MAX_RESEARCH_SEC ?? 1);
  const SAT = 212, MINE_CODES = new Set([1, 2, 3, 12]);
  let stalls = 0, lastSatAt = 0;
  const pendB = new Map(), pendR = new Map(), pendU = new Map();
  const slowReported = new Set();
  const prune = (pend, levels) => { for (const [c, t] of [...pend]) if ((levels[c] ?? 0) >= t) pend.delete(c); };

  for (let step = 0; step < steps; step++) {
    // Follow planner updates: the ratio gate flips which unit type to build, so
    // re-read the plan each pass; keep the previous targets if the file is mid-write.
    try { unitTargets = unitTargetsFrom(JSON.parse(fs.readFileSync(goalsPath, 'utf8'))); } catch { /* keep previous */ }
    const B = await scope('page=buildings' + cpq);
    const R = await scope('page=research' + cpq);
    const S = await scope('page=shipyard&mode=fleet' + cpq);
    const D = await scope('page=shipyard&mode=defense' + cpq);
    prune(pendB, B.levels); prune(pendR, R.levels);
    const eff = (c) => (B.levels[c] ?? R.levels[c] ?? 0) + (B.queued[c] ?? 0) + (R.queued[c] ?? 0);
    // Adopt current levels for auto-grown codes so a restart resumes where the
    // colony actually is instead of re-climbing from the plan's low baseline.
    for (const c of [...gradual, ...bumpBuilders]) {
      if ((goalTargets.get(c) || 0) < eff(c)) goalTargets.set(c, eff(c));
    }

    // Map (not object) so insertion order survives for integer-like keys.
    const need = new Map();
    const visit = (c, l) => {
      const k = String(c);
      if (!need.has(k) || need.get(k) < l) need.set(k, l);
      // If this goal is already satisfied, do not force its prerequisites: an
      // account-wide tech (e.g. Computer Tech) may be done without this planet
      // having the building that normally gates it (e.g. Research Lab).
      if (eff(k) >= l) return;
      for (const r of graph[k] || []) visit(r.id, r.required);
    };
    for (const c of ordered) visit(c, goalTargets.get(c));

    const energy = energyFromHtml(B.html);
    // Research whose card shows a Duration is deferred (needs University/colonies);
    // it must not block the "plan satisfied" check that drives gradual growth.
    const isSlowResearch = (c) => {
      if (allowSlow.has(c)) return false;
      if (B.byCode[Number(c)]) return false;
      const it = R.byCode[Number(c)];
      return it ? it.durationSec > maxResearchSec : false;
    };
    // "unmet" = targets that are actionable now. Prerequisite-blocked targets are
    // excluded so they cannot stall the gradual growth; they stay in `need` and
    // get built once their prerequisites are satisfied (e.g. conveyors waiting
    // on Nanite).
    const unmet = [...need.entries()].filter(([c, l]) =>
      eff(c) < l && !isSlowResearch(c) &&
      (graph[c] || []).every((r) => eff(r.id) >= r.required));
    // Unit build tasks (ships/defenses), independent of the building/research queues.
    const unitHtml = (scope) => (scope === 'fleet' ? S.html : D.html);
    const unitTasks = [];
    for (const u of unitTargets) {
      const html = unitHtml(u.scope);
      if (!unitAvail(html, u.code)) continue;
      const have = unitVal(html, u.code);
      const p = pendU.get(u.code);
      if (p !== undefined) {
        // Done when the order lands; stale when the planet was DRAINED (the pooler
        // moved the ships to main, so `have` drops below the submit baseline) or the
        // deadline passed without the total arriving (POST silently rejected).
        // Dropping it lets the next pass re-order instead of stalling forever.
        if (have >= p.exp || have < p.base || Date.now() > p.until) pendU.delete(u.code);
        else continue;
      }
      if (have >= u.target) continue;
      const it = (u.scope === 'fleet' ? S.byCode : D.byCode)[Number(u.code)];
      const rate = it ? (it.perSec || 0) : 0;
      const remaining = u.target - have;
      let want = Math.min(unitBatch, remaining);
      // If we know the throughput, make the order last ~unitSeconds (but never a
      // tiny order that drains before the next pass).
      if (unitSeconds > 0 && rate > 0) want = Math.min(remaining, Math.max(want, Math.ceil(rate * unitSeconds)));
      unitTasks.push({ ...u, have, want, rate });
    }
    const unitsPending = unitTargets.some((u) => unitAvail(unitHtml(u.scope), u.code) && unitVal(unitHtml(u.scope), u.code) < u.target);
    if (unmet.length === 0 && !unitsPending && !(energy !== null && energy < 0)) {
      if (gradual.length || bumpBuilders.length) {
        const bumped = [];
        const capOf = (c) => Number(caps[c] != null ? caps[c] : gradualCap);
        const bump = (c, tag) => {
          const isB = B.byCode[Number(c)] !== undefined;
          const it = isB ? null : R.byCode[Number(c)];
          if (!isB && it && it.durationSec > maxResearchSec) return; // long research: don't grow it
          const cur = goalTargets.get(c) || 0;
          const cap = capOf(c);
          if (cur < cap) { goalTargets.set(c, cur + 1); bumped.push(`${c}${tag}->${cur + 1}`); }
        };
        const [robotC, naniteC] = bumpBuilders;
        for (const c of gradual) {
          if (c === robotC || c === naniteC) continue; // Robot/Nanite handled below
          bump(c, '');
        }
        // Robot/Nanite are the builders: Robot drives, Nanite trails by 11
        // (floor 1, capped). Growing them here (not only on slow builds) means the
        // plan keeps progressing until their caps are reached instead of stopping
        // once the economy caps are hit.
        const robotIt = robotC ? B.byCode[Number(robotC)] : null;
        if (robotIt) {
          const robotTarget = Math.min((goalTargets.get(robotC) || robotIt.level) + 1, capOf(robotC));
          if ((goalTargets.get(robotC) || 0) < robotTarget) { goalTargets.set(robotC, robotTarget); bumped.push(`${robotC}->${robotTarget}`); }
          if (naniteC) {
            const naniteTarget = Math.max(1, Math.min(robotTarget - 11, capOf(naniteC)));
            if ((goalTargets.get(naniteC) || 0) < naniteTarget) { goalTargets.set(naniteC, naniteTarget); bumped.push(`${naniteC}->${naniteTarget}`); }
          }
        }
        if (bumped.length) { console.log(`[+] gradual bump: ${bumped.join(', ')}`); await sleep(gradualWaitMs); continue; }
      }
      console.log('[*] All goals satisfied.'); break;
    }

    const affordable = (it) => B.res.metal >= it.cost.metal && B.res.crystal >= it.cost.crystal && B.res.deuterium >= it.cost.deuterium;
    const buildable = (it) => it && (it.cost.metal + it.cost.crystal + it.cost.deuterium) > 0 && it.hasBuild;

    // Feed each in-game queue independently; cap in-flight work so the queue
    // keeps moving without being saturated.
    const pick = (isBuilding) => {
      const scopeObj = isBuilding ? B : R;
      const pend = isBuilding ? pendB : pendR;
      if (pend.size >= (isBuilding ? maxBuild : maxResearch)) return null;
      for (const [c, l] of need.entries()) {
        if (eff(c) >= l || pend.has(c)) continue;
        const it = scopeObj.byCode[Number(c)];
        if (!it) continue;                        // not on this queue
        if (!isBuilding && eff(31) < 1) continue; // research needs a lab
        if (!(graph[c] || []).every((r) => eff(r.id) >= r.required)) continue;
        if (isBuilding && energy !== null && energy < 0 && MINE_CODES.has(Number(c))) continue; // don't deepen an energy deficit
        if (!buildable(it)) continue;
        if (!isBuilding && !allowSlow.has(c) && it.durationSec > maxResearchSec) {
          if (!slowReported.has(c)) { console.log(`[~] skip ${it.name}: ${it.durationSec}s research (> ${maxResearchSec}s)`); slowReported.add(c); }
          continue;
        }
        if (!affordable(it)) continue;
        return { code: Number(c), it };
      }
      return null;
    };

    // Keep every structure build <= builderBumpSec. Find the next structure the
    // plan wants to build; if IT would take too long, raise Robot/Nanite and
    // build a builder this cycle instead (Nanite kept at Robot-11, floor 1).
    let bumpFirst = null;
    if (pendB.size < maxBuild) {
      let nextIt = null;
      for (const [c, l] of need.entries()) {
        if (eff(c) >= l || pendB.has(c)) continue;
        const it = B.byCode[Number(c)];
        if (it && !bumpBuilders.includes(String(c))) { nextIt = it; break; }
      }
      if (nextIt && nextIt.durationSec >= builderBumpSec) {
        const capOf = (c) => Number(caps[c] != null ? caps[c] : gradualCap);
        // Strategy: raise Robot one level (the driver), then keep Nanite at
        // Robot-11 (floor 1, capped). e.g. Robot 15/Nanite 4, Robot 16/Nanite 5.
        const [robotC, naniteC] = bumpBuilders;
        const robotIt = B.byCode[Number(robotC)];
        const naniteIt = naniteC != null ? B.byCode[Number(naniteC)] : null;
        if (robotIt) {
          const robotTarget = Math.min(robotIt.level + 1, capOf(robotC));
          if ((goalTargets.get(robotC) || 0) < robotTarget) goalTargets.set(robotC, robotTarget);
          if (naniteIt) {
            const naniteTarget = Math.max(1, Math.min(robotTarget - 11, capOf(naniteC)));
            if ((goalTargets.get(naniteC) || 0) < naniteTarget) goalTargets.set(naniteC, naniteTarget);
          }
        }
        // Build whichever of Nanite/Robot is still behind its target (Nanite first).
        const candidates = [naniteC, robotC]
          .filter(Boolean)
          .map((c) => ({ code: Number(c), it: B.byCode[Number(c)] }))
          .filter((x) => x.it && !pendB.has(String(x.code)) && (goalTargets.get(String(x.code)) || 0) > x.it.level
            && buildable(x.it) && affordable(x.it));
        if (candidates.length) {
          bumpFirst = candidates[0];
          console.log(`[~] next ${nextIt.name} would take ${nextIt.durationSec}s (>= ${builderBumpSec}s); prioritising ${bumpFirst.it.name} L${bumpFirst.it.level} -> L${bumpFirst.it.level + 1}`);
        }
      }
    }

    const bld = bumpFirst || pick(true);
    const tech = pick(false);
    // Energy top-up: Solar Satellites are far cheaper than Solar Plant levels.
    const sats = (energy !== null && energy < 0 && S.byCode[SAT] && (Date.now() - lastSatAt) > satCooldownMs) ? satBatch : 0;

    if (!bld && !tech && !sats && !unitTasks.length) {
      stalls++;
      if (stalls === 1 && !pendB.size && !Object.keys(B.queued).length) {
        console.log(`[debug] unmet=[${unmet.map(([c, l]) => `${c}:${eff(c)}/${l}`).join(' ')}] `
          + `pendB=${pendB.size} pendR=${pendR.size} pendU=${pendU.size} `
          + `queuedB=${JSON.stringify(B.queued)} queuedR=${JSON.stringify(R.queued)} `
          + `energy=${energy} unitsPending=${unitsPending} `
          + `targets=${JSON.stringify([...goalTargets])}`);
      }
      // Self-heal: an in-game queue that is empty while we still hold process-side
      // pending entries means a build POST was silently rejected; drop the stale
      // entries so the next loop can re-evaluate instead of stalling forever.
      if (stalls >= 3 && !Object.keys(B.queued).length && !Object.keys(R.queued).length
          && (pendB.size || pendR.size || pendU.size)) {
        console.log(`[!] clearing stale pending (bld=${pendB.size} res=${pendR.size} unit=${pendU.size})`);
        // NB: do NOT clear pendU — we don't parse the shipyard queue here, so an
        // in-progress unit order looks "empty" and clearing it would re-order the
        // same batch (overproducing). The daemon's next resolve run re-evaluates.
        pendB.clear(); pendR.clear();
      }
      if (maxStalls && stalls >= maxStalls) { console.log(`[~] stalled ${stalls} times; exiting`); break; }
      console.log(`[~] queues busy${energy !== null ? ` (energy ${energy}%)` : ''}; waiting`);
      await delay(); await sleep(waitMs);
      continue;
    }

    if (bld) {
      await delay();
      await postForm('page=buildings' + cpq, { cmd: 'insert', building: bld.code, lvlup: bld.it.level + 1 });
      pendB.set(String(bld.code), bld.it.level + 1);
      await delay();
      console.log(`[+] build ${bld.it.name} L${bld.it.level} -> L${bld.it.level + 1}${energy !== null ? ` [energy ${energy}%]` : ''}`);
    }
    if (tech) {
      await delay();
      await postForm('page=research' + cpq, { cmd: 'insert', tech: tech.code, lvlup: tech.it.level + 1 });
      pendR.set(String(tech.code), tech.it.level + 1);
      await delay();
      console.log(`[+] research ${tech.it.name} L${tech.it.level} -> L${tech.it.level + 1}`);
    }
    if (sats) {
      await delay();
      await postForm('page=shipyard&mode=fleet' + cpq, { [`fmenge[${SAT}]`]: sats });
      lastSatAt = Date.now();
      await delay();
      console.log(`[+] shipyard ${sats}x Solar Satellite [energy ${energy}%]`);
    }
    if (unitTasks.length) {
      const u = unitTasks[0];
      const q = u.scope === 'fleet' ? 'page=shipyard&mode=fleet' : 'page=shipyard&mode=defense';
      await delay();
      await postForm(q + cpq, { [`fmenge[${u.code}]`]: u.want });
      // Store the submit baseline + expected total + a deadline. If the count later
      // drops below `base` (ships pooled away) or `until` passes under `exp`, the
      // pending entry is discarded instead of blocking that unit forever.
      const etaMs = u.rate > 0 ? Math.ceil((u.want / u.rate) * 1000) : 300000;
      pendU.set(u.code, { base: u.have, exp: u.have + u.want, until: Date.now() + etaMs + 120000 });
      console.log(`[+] ${u.scope} ${u.want}x code ${u.code} (have ${u.have}/${u.target}${u.rate ? ` @${u.rate}/s` : ''})`);
      // Time the next pass to just after this order finishes (instead of a fixed
      // short poll), so the shipyard never drains between batches.
      if (u.rate > 0) await sleep(Math.min(Math.ceil((u.want / u.rate) * 1000), 900000) + 2500);
      else await delay();
    }
    stalls = 0;
  }
  console.log('[+] resolve done');
}

// Persistent per-planet worker: keeps ONE session and loops resolve for a single
// planet, so several planets build in parallel (the server allows concurrent
// sessions). Re-logs only when a request proves the session died.
// Usage: httpbot.mjs worker <cp> <goals.json> [--interval sec]
async function cmdWorker(cp, goalsPath, intervalSec) {
  if (!cp || !goalsPath) { console.error('usage: httpbot.mjs worker <cp> <goals.json> [--interval sec]'); process.exit(2); }
  const interval = Math.max(2, Number(intervalSec || env.EXPLORER_WORKER_INTERVAL_S || 10)) * 1000;
  console.log(`[worker] start cp=${cp} goals=${goalsPath} interval=${interval / 1000}s`);
  for (;;) {
    try {
      await cmdResolve(goalsPath, Number(env.EXPLORER_WORKER_STEPS || 1000000), cp);
    } catch (e) {
      console.error(`[worker cp=${cp}] ${e.message}; relogin + retry in 10s`);
      sessionReady = false;
      try { await login(true); } catch (e2) { console.error(`[worker cp=${cp}] relogin failed: ${e2.message}`); }
      await sleep(10000);
      continue;
    }
    await sleep(interval);
  }
}

// Read back the plan's final targets and confirm they are met. Exit 0 when every
// fixed target, research target, ship/defense target and every gradual/builder
// cap is reached; exit 2 otherwise. Daemon wrappers use this to stop a colony
// daemon automatically once the build-out is genuinely done.
async function cmdVerify(goalsPath, cpArg) {
  const goals = JSON.parse(fs.readFileSync(goalsPath, 'utf8'));
  const cpq = cpArg ? `&cp=${cpArg}` : '';
  await login();
  const B = await scope('page=buildings' + cpq);
  const R = await scope('page=research' + cpq);
  const S = await scope('page=shipyard&mode=fleet' + cpq);
  const D = await scope('page=shipyard&mode=defense' + cpq);
  const lvl = (c) => (B.levels[c] ?? R.levels[c] ?? 0);
  const caps = goals.caps || {};
  const missing = [];
  const check = (label, have, want) => { if (Number(have) < Number(want)) missing.push(`${label} ${have}/${want}`); };
  for (const [c, t] of Object.entries(goals.buildings || {})) check(`b${c}`, lvl(c), t);
  for (const [c, t] of Object.entries(goals.research || {})) check(`r${c}`, R.levels[c] ?? 0, t);
  for (const c of [...(goals.gradual || []), ...(goals.bumpBuilders || [])]) {
    if (caps[c] != null) check(`cap${c}`, lvl(String(c)), caps[c]);
  }
  const ships = unitsFromHtml(S.html), defs = unitsFromHtml(D.html);
  for (const [c, t] of Object.entries(goals.ships || {})) check(`ship${c}`, ships[c] ?? 0, t);
  for (const [c, t] of Object.entries(goals.defenses || {})) check(`def${c}`, defs[c] ?? 0, t);
  if (missing.length) {
    console.log(`[verify] NOT MET (${missing.length} missing): ${missing.join(', ')}`);
    process.exit(2);
  }
  console.log('[verify] MET: all plan targets reached');
}

// Cancel/clear the building queue: the active row posts cmd=cancel, queued
// rows post cmd=remove with their listid.
async function cmdCancel() {
  await login();
  for (let i = 0; i < 1000; i++) {
    const html = await (await getPage('page=buildings')).text();
    const rows = (html.match(/class="element_row/g) || []).length;
    if (rows === 0) { console.log('[*] queue empty'); break; }
    let form;
    if (/name="cmd"\s+value="cancel"/.test(html)) form = { cmd: 'cancel' };
    else {
      const m = /name="cmd"\s+value="remove"[\s\S]*?name="listid"\s+value="(\d+)"/.exec(html);
      if (!m) { console.log('[-] no removable queue row found'); break; }
      form = { cmd: 'remove', listid: m[1] };
    }
    await delay();
    await postForm('page=buildings', form);
    await delay();
    console.log(`[-] cancelled ${form.cmd}${form.listid ? ' ' + form.listid : ''} (rows before=${rows})`);
  }
}

// Remove only *queued* rows (cmd=remove), leaving the active row running.
// Useful to drop over-queued research/builds while keeping the current one.
async function cmdTrim(query) {
  const q = query || 'page=research';
  await login();
  for (let i = 0; i < 2000; i++) {
    const html = await (await getPage(q)).text();
    const m = /name="cmd"\s+value="remove"[\s\S]*?name="listid"\s+value="(\d+)"/.exec(html);
    if (!m) { console.log('[*] no queued rows left'); break; }
    await delay();
    await postForm(q, { cmd: 'remove', listid: m[1] });
    await delay();
    console.log(`[-] removed queued listid ${m[1]}`);
  }
}

// Redeem a voucher code on the "Voucher System" page (page=reward2).
async function cmdRedeem(code) {
  if (!code) throw new Error('need a voucher code');
  await login();
  const html = await postForm('page=reward2', { voucher: code, redeem: '' });
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(path.join(DATA_DIR, 'redeem-last.html'), html);
  const text = stripTags(html);
  const hits = text.match(/.{0,70}(success|invalid|already|reward|voucher|redeem|activat|claim|error|wrong|expired).{0,90}/gi);
  console.log(`[redeem ${code}] ${hits ? [...new Set(hits)].slice(0, 5).join(' || ') : text.slice(0, 300)}`);
}

// Academy: level Weaponry (1101) to 5, then dump all remaining points into
// Engine limitation (1105, +3% fleet speed / level). Skills upgrade via GET.
async function cmdAcademy() {
  await login();
  const readState = async () => {
    const html = await (await getPage('page=academy')).text();
    const points = parseInt((/Academy points:\s*(\d+)/.exec(stripTags(html)) || [])[1] || '0', 10);
    const cells = {};
    for (const m of html.matchAll(/<td class="skils_bg[^"]*"><a\s[^>]*skil=(\d+)[^>]*data-tooltip-content="([^"]*)"/g)) {
      cells[+m[1]] = stripTags(m[2]);
    }
    return { points, cells };
  };
  const parse = (tip) => ({
    nextLevel: parseInt((/Level\s+(\d+)\s*:/.exec(tip) || [])[1] || '0', 10),
    cost: parseInt((/Required:\s*(\d+)/.exec(tip) || [])[1] || '0', 10),
  });
  const upgrade = async (skil, label) => {
    const { points, cells } = await readState();
    const tip = cells[skil];
    if (!tip) return { done: true, reason: 'cell locked/missing' };
    const { nextLevel, cost } = parse(tip);
    if (points < cost) return { done: true, reason: `need ${cost} pts (have ${points})` };
    await delay();
    await getUrl(pageUrl('page=academy&mode=up&skil=' + skil));
    await delay();
    console.log(`[+] academy ${label} L${nextLevel - 1} -> L${nextLevel} (pts before ${points})`);
    return { done: false };
  };

  for (let i = 0; i < 20; i++) {
    const { cells, points } = await readState();
    const tip = cells[1101];
    if (!tip) { console.log('[academy] Weaponry cell missing'); break; }
    const { nextLevel } = parse(tip);
    if (nextLevel > 5) { console.log('[academy] Weaponry reached L5'); break; }
    const r = await upgrade(1101, 'Weaponry');
    if (r.done) { console.log(`[academy] Weaponry stop: ${r.reason} (points ${points})`); break; }
  }

  for (let i = 0; i < 500; i++) {
    const { cells } = await readState();
    let skil = null;
    for (const [id, t] of Object.entries(cells)) if (/Engine limitation/i.test(t)) { skil = +id; break; }
    if (!skil) { console.log('[academy] Engine limitation not unlocked (need Weaponry 5)'); break; }
    const r = await upgrade(skil, 'Engine limitation');
    if (r.done) { console.log(`[academy] Engine limitation stop: ${r.reason}`); break; }
  }
  const final = await readState();
  console.log(`[academy] remaining points: ${final.points}`);
}

// Level one academy skill (code) toward a target level, spending points.
// Prints each upgrade + remaining points. Usage: academy-up <skil> <targetLevel>
async function cmdAcademyUp(skil, target) {
  if (!skil) throw new Error('need a skill code');
  const tgt = parseInt(target || '100', 10);
  await login();
  const readState = async () => {
    const html = await (await getPage('page=academy')).text();
    const points = parseInt((/Academy points:\s*(\d+)/.exec(stripTags(html)) || [])[1] || '0', 10);
    const cells = {};
    for (const m of html.matchAll(/skil=(\d+)[^>]*data-tooltip-content="([^"]*)"/g)) cells[+m[1]] = stripTags(m[2]);
    return { points, cells };
  };
  const parse = (tip) => ({
    nextLevel: parseInt((/Level\s+(\d+)\s*:/.exec(tip) || [])[1] || '0', 10),
    cost: parseInt((/Required:\s*(\d+)\s*Academy/i.exec(tip) || [])[1] || '0', 10),
  });
  for (let i = 0; i < 200; i++) {
    const { points, cells } = await readState();
    const tip = cells[skil];
    if (!tip) { console.log(`[academy-up] ${skil} locked or maxed (points ${points})`); return; }
    const { nextLevel, cost } = parse(tip);
    if (nextLevel > tgt) { console.log(`[academy-up] ${skil} at L${nextLevel - 1} >= target L${tgt} (points ${points})`); return; }
    if (points < cost) { console.log(`[academy-up] ${skil} needs ${cost} pts, have ${points}; stop at L${nextLevel - 1}`); return; }
    await delay();
    await getUrl(pageUrl(`page=academy&mode=up&skil=${skil}`));
    console.log(`[+] academy-up ${skil} L${nextLevel - 1} -> L${nextLevel} (cost ${cost}, had ${points})`);
  }
  console.log('[academy-up] loop limit reached');
}

// Dump the whole Academy tree (all 3 branches) as code -> {name, effect, level,
// next, requirement, cost}. Works for locked skills too (no <a skil=> link).
async function cmdAcademyMap() {
  await login();
  const html = await (await getPage('page=academy')).text();
  const points = parseInt((/Academy points:\s*(\d+)/.exec(stripTags(html)) || [])[1] || '0', 10);
  const out = { points, skills: {} };
  const re = /data-tooltip-content="([^"]*)"/g;
  let m;
  while ((m = re.exec(html))) {
    const code = /gebaeude\/(\d{4})\.jpg/.exec(html.slice(m.index, m.index + 400));
    if (!code) continue;
    const tip = stripTags(m[1]).replace(/\s+/g, ' ').trim();
    if (!/Level\s+\d+\s*:/.test(tip)) continue;
    const name = (tip.match(/^(.*?)\s+Level\s+\d+\s*:/) || [])[1] || '';
    const level = parseInt((/Level\s+(\d+)\s*:/.exec(tip) || [])[1] || '0', 10);
    const effect = ((tip.match(/Level\s+\d+\s*:\s*(.*?)(?:\s+Required:|\s*$)/) || [])[1] || '').trim();
    const reqRaw = (tip.match(/\s+Required:\s*(.+?)\s*$/) || [])[1] || '';
    const cost = /^\d+$/.test(reqRaw) ? parseInt(reqRaw, 10) : null;
    const requires = cost === null && reqRaw ? reqRaw : null;
    out.skills[code[1]] = { name, level, effect, cost, requires, tip };
  }
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(path.join(DATA_DIR, 'academy-map.json'), JSON.stringify(out, null, 2));
  console.log(`[academy-map] points=${points} skills=${Object.keys(out.skills).length}`);
  console.log(JSON.stringify(out, null, 2));
}

// Fleet send, mirroring the working browser flow captured in fleet_movement.har:
//   step1 POST source coords + ships (mission=0)
//   GET  fleetStep1&mode=checkTarget (target) -> OK
//   step2 POST target coords + speed + mission=<n> + token  (mission=0 is rejected)
//   step3 POST token + univers_<id> + mission + resources + staytime
// speed is the 1..10 index (10 = 100%).
// Usage: httpbot.mjs fleet <g:s:p> <mission> <code:count,...> [speed] [--dry]
async function cmdFleet(gtarget, mission, shipsCsv, speedArg, dry, cpArg) {
  await login();
  const [g, sys, p] = String(gtarget).split(':').map(Number);
  const speed = String(speedArg || 10);
  const cpq = cpArg ? `&cp=${cpArg}` : '';
  // Recycle (mission 8) targets a debris field (type 2), everything else a planet (1).
  const ttype = String(mission) === '8' ? '2' : '1';
  fs.mkdirSync(DATA_DIR, { recursive: true });

  // Seed step 1 from fleetTable's glav form — it carries the SOURCE coordinates.
  const ft = await (await getPage('page=fleetTable' + cpq)).text();
  const fm = /<form[^>]*name="glav"[^>]*>([\s\S]*?)<\/form>/.exec(ft);
  const step1 = {};
  if (fm) for (const m of fm[0].matchAll(/<input[^>]*name="([^"]+)"[^>]*value="([^"]*)"/g)) step1[m[1]] = m[2];
  step1.type = '1';
  step1.mission = '0';
  for (const k of Object.keys(step1)) if (/^ship\d+$/.test(k)) step1[k] = '0';
  for (const pair of String(shipsCsv || '').split(',')) { const [c, n] = pair.split(':'); if (c) step1['ship' + c] = String(n); }
  let html = await postForm('page=fleetStep1' + cpq, step1);
  fs.writeFileSync(path.join(DATA_DIR, 'fleet-step2.html'), html);
  const token2 = (/name="token" value="([^"]+)"/.exec(html) || [])[1];
  console.log(`[fleet] step1 (source ${step1.galaxy}:${step1.system}:${step1.planet}) ships ${shipsCsv}`);
  if (!token2) { console.log('[fleet] step1 failed (no token)'); return; }

  const ct = await getUrl(pageUrl(`page=fleetStep1&mode=checkTarget&galaxy=${g}&system=${sys}&planet=${p}&planet_type=${ttype}&lang=en&kolo=0`));
  const ctTxt = (await ct.text()).trim();
  console.log(`[fleet] checkTarget ${g}:${sys}:${p} -> ${ctTxt}`);
  if (ctTxt !== 'OK') { console.log('[fleet] target rejected'); return; }

  const step2 = {
    token: token2, fleet_group: '0', mission: String(mission),
    galaxy: String(g), system: String(sys), planet: String(p), type: ttype, speed,
    'shortcut[][name]': '', 'shortcut[][galaxy]': '', 'shortcut[][system]': '', 'shortcut[][planet]': '', 'shortcut[][type]': ttype,
  };
  html = await postForm('page=fleetStep2' + cpq, step2);
  fs.writeFileSync(path.join(DATA_DIR, 'fleet-step3.html'), html);
  const token3 = (/name="token" value="([^"]+)"/.exec(html) || [])[1];
  const univ = /name="(univers_\d+)" value="([^"]+)"/.exec(html);
  if (!token3 || !univ) { console.log('[fleet] step2 failed (no step3 token)'); return; }
  if (dry) { console.log('[fleet] --dry: stopped before final dispatch'); return; }

  const step3 = { token: token3, [univ[1]]: univ[2], mission: String(mission), metal: '1', crystal: '', deuterium: '', staytime: '1' };
  const out = await postForm('page=fleetStep3' + cpq, step3);
  fs.writeFileSync(path.join(DATA_DIR, 'fleet-result.html'), out);
  const sent = /Fleet sent/i.test(out);
  const txt = stripTags(out);
  const info = /Mission\s+(\w+)\s+Distance\s+([\d.]+)\s+Fleet speed\s+([\d.]+)\s+Consumption of deuterium\s+([\d.]+)/i.exec(txt);
  console.log(sent ? `[fleet] OK sent: ${info ? info[0] : 'confirmed'}` : `[fleet] FAILED (response body ${(/<body[^>]*id="([^"]+)"/.exec(out) || [])[1]})`);
}

// Recall the newest (or a given) outgoing fleet.
async function cmdFleetBack(fleetID) {
  await login();
  const ft = await (await getPage('page=fleetTable')).text();
  const id = fleetID || (/name="fleetID"[^>]*value="(\d+)"/.exec(ft) || [])[1] || (/fleetID["']?\s*[:=]\s*["']?(\d+)/.exec(ft) || [])[1];
  if (!id) { console.log('[fleetback] no outgoing fleet found'); return; }
  const out = await postForm('page=fleetTable&action=sendfleetback', { fleetID: String(id) });
  const txt = stripTags(out);
  console.log(`[fleetback] recalled fleetID ${id}${/not|error/i.test(txt) ? ' (check response)' : ''}`);
}

// Auto-expedition — a server feature (see ft form `#expfleet`): POST page=fleetTable
//   cmd=1                       -> random "Deep area of galaxy"
//   cmd=2 + pve=1|2|3           -> chosen target Barbarians|Pirates|Aliens
// Ship counts use the `ship2<code>` prefix (e.g. ship2217). exp_time is 1..10 -> 0.25..2.5 h.
// Usage: httpbot.mjs expedition <code:count,...> [num] [time] [speed] [--pve N] [--cp id]
async function cmdExpedition(shipsCsv, numArg, timeArg, speedArg, pve, cpArg) {
  await login();
  const cpq = cpArg ? `&cp=${cpArg}` : '';
  const form = {
    cmd: pve ? '2' : '1',
    exp_num: String(numArg || 1),
    exp_time: String(timeArg || 1),
    exp_speed: String(speedArg || 10),
  };
  if (pve) form.pve = String(pve);
  for (const pair of String(shipsCsv || '').split(',')) {
    const [c, n] = pair.split(':');
    if (c) form['ship2' + c] = String(n);
  }
  const html = await postForm('page=fleetTable' + cpq, form);
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(path.join(DATA_DIR, 'expedition-send.html'), html);
  const txt = stripTags(html.replace(/<script[\s\S]*?<\/script>/gi, ''));
  const slots = /(\d+)\s*\/\s*(\d+)\s*expedition/i.exec(txt);
  console.log(`[expedition] cmd=${form.cmd} num=${form.exp_num} time=${form.exp_time} speed=${form.exp_speed} ships=${shipsCsv}${pve ? ' pve=' + pve : ''}`);
  console.log(`[expedition] slots now ${slots ? slots[0].trim() : '? (page saved to data/expedition-send.html)'}`);
  // Record what we sent so outcome reports can be attributed to a composition.
  const runPath = path.join(DATA_DIR, 'expedition-runs.json');
  const runs = fs.existsSync(runPath) ? JSON.parse(fs.readFileSync(runPath, 'utf8')) : [];
  runs.push({
    at: new Date().toISOString(), account: USER, cp: cpArg || null, pve: pve || null,
    ships: shipsCsv, num: Number(form.exp_num), time: Number(form.exp_time), speed: Number(form.exp_speed),
    slotsAfter: slots ? `${slots[1]}/${slots[2]}` : null,
  });
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(runPath, JSON.stringify(runs, null, 2));
}

// Resource Trader (page=trader). Value ratio metal:crystal:deuterium = 1:2:4;
// buying <resource> with others yields  sum(given * value_given / value_bought)
// (e.g. buying crystal: 2 metal -> 1 crystal, 1 deut -> 2 crystal). Every call
// costs 250 Dark Matter, so consolidate into ONE big order.
// Usage: httpbot.mjs trade <buyCode 901|902|903> <giveCode:amt,...> [--cp id]
async function cmdTrade(buy, giveArg, cpArg) {
  await login();
  const cpq = cpArg ? `&cp=${cpArg}` : '';
  const form = { mode: 'send', resource: String(buy) };
  for (const pair of String(giveArg || '').split(',')) {
    const [c, n] = pair.split(':');
    if (c) form['trade[' + c + ']'] = String(n);
  }
  const html = await postForm('page=trader' + cpq, form);
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(path.join(DATA_DIR, 'trader-result.html'), html);
  const txt = stripTags(html.replace(/<script[\s\S]*?<\/script>/gi, '')).replace(/\s+/g, ' ');
  const hit = /not enough[^.]{0,60}|not possible[^.]{0,60}|too little[^.]{0,60}|Dark Matter[^.]{0,60}|no longer[^.]{0,60}/i.exec(txt);
  console.log(`[trade] buy=${buy} give=${giveArg}${cpArg ? ` cp=${cpArg}` : ''}${hit ? ' :: ' + hit[0] : ''}`);
}

// List outgoing fleets from fleetTable (ID / mission / destination / objective / eta).
async function cmdExpState() {
  await login();
  const html = await (await getPage('page=fleetTable')).text();
  const txt = stripTags(html.replace(/<script[\s\S]*?<\/script>/gi, ''));
  const cap = /(\d+)\s*\/\s*(\d+)\s*expedition/i.exec(txt);
  const rows = [];
  const start = html.indexOf('Arrival(Destination)');
  const end = start >= 0 ? html.indexOf('Automatically send an expedition', start) : -1;
  const region = (start >= 0 ? html.slice(start, end > 0 ? end : undefined) : '').replace(/data-tooltip-content="[\s\S]*?"/g, '');
  for (const tr of region.split(/<tr[^>]*>/).slice(1)) {
    const cells = [...tr.matchAll(/<td[^>]*>([\s\S]*?)<\/td>/g)].map((c) => stripTags(c[1]).replace(/\s+/g, ' ').trim());
    const id = (tr.match(/name="fleetID" value="(\d+)"/) || [])[1] || null;
    if (cells.length >= 7 && /^\d+$/.test(cells[0]) && /expedition|transport|deploy|attack|espionage|recycle|hold/i.test(cells[1])) {
      rows.push({ fleetID: id, mission: cells[1], number: cells[2], start: cells[3], arrival: cells[4], destination: cells[5], back: cells[6] || '', eta: cells[7] || '' });
    }
  }
  console.log(JSON.stringify({ expeditionSlots: cap ? cap[2] : null, used: cap ? cap[1] : null, fleets: rows }, null, 2));
  return rows;
}

// Parse message rows (id, date, sender, subject, body text) from a messages AJAX page.
function parseMessageRows(html) {
  const rows = [];
  for (const m of html.matchAll(/<tr id="message_(\d+)"[\s\S]*?<\/tr>\s*<tr class="messages_body[^"]*">([\s\S]*?)<\/tr>/g)) {
    const id = m[1];
    const head = [...m[0].matchAll(/<td[^>]*class="head_row_msg"[^>]*>([\s\S]*?)<\/td>/g)].map((c) => stripTags(c[1]).replace(/\s+/g, ' ').trim());
    const date = head[1] || null;
    const sender = (head[2] || '').replace(/^From\s+/i, '').trim() || null;
    const subject = head[3] || null;
    const body = stripTags(m[2]).replace(/\s+/g, ' ').trim();
    const report = (m[2].match(/CombatReport\.php\?raport=([a-f0-9]+)/i) || [])[1] || null;
    const row = { id, date, sender, subject, body };
    if (report) {
      row.report = report;
      row.attackerLosses = num(/(?:Losses attacker|Lost attacker):\s*([\d.]+)/i.exec(body)?.[1]);
      row.defenderLosses = num(/(?:Lost defender|Losses defender):\s*([\d.]+)/i.exec(body)?.[1]);
    }
    // Fight-report summary (messcat=3): profit, rubblefield and combat XP are
    // all printed in the message body and were previously dropped.
    const profit = /Profit Metal:\s*([\d.]+)\s*Crystal:\s*([\d.]+)\s*Deuterium:\s*([\d.]+)/i.exec(body);
    if (profit) row.profit = { metal: num(profit[1]), crystal: num(profit[2]), deuterium: num(profit[3]) };
    const rub = /Rubblefield Metal:\s*([\d.]+)\s*Crystal:\s*([\d.]+)/i.exec(body);
    if (rub) row.rubblefield = { metal: num(rub[1]), crystal: num(rub[2]) };
    const xp = /Received\s+([\d.]+)\s+combat experience/i.exec(body);
    if (xp) row.combatXp = num(xp[1]);
    // Achievement (messcat=4): "Reached: <name> <n> level/lvl. Received: <a> Antimatter [and] <b> Achievement Points".
    const ach = /Reached:\s*(.+?)\s+(\d+)\s+(?:level|lvl)[.!]?\s*Received:\s*([\d.]+)\s+Antimatter\s*(?:and\s*)?([\d.]+)\s+Achiev/i.exec(body);
    if (ach) row.achievement = { name: ach[1].trim(), level: num(ach[2]), antimatter: num(ach[3]), points: num(ach[4]) };
    // Spy (messcat=0): fleet sighting with owner/coords, or a spy report.
    const sight = /hostile fleet of the planet (.+?)\s*\[([\d:]+)\]\s*was sighted near your planet (.+?)\s*\[([\d:]+)\]/i.exec(body);
    if (sight) row.sighting = { owner: sight[1].trim(), from: sight[2], target: sight[3].trim(), at: sight[4] };
    rows.push(row);
  }
  return rows;
}

// Ordered expedition-outcome taxonomy keyed off the live server language file
// (data/_lang_FLEETphp, sys_expe_*). First match wins, so the specific custom
// strings precede the generic buckets. Vanilla MissionCaseExpedition rolls
// mt_rand(1,9): 1=resources 2=darkmatter 3=ships 4=combat 5=black hole
// 6=time shift 7-9=nothing; this server adds custom flavours (bacterium, virus,
// stardust, "ancient battlefield" arsenal drops, a non-fatal black hole).
const EXPEDITION_FLAVORS = [
  ['return', 'return', [/returned from the expedition/i]],
  ['blackhole', 'lost-fleet', [/has not returned from the hyperspacejump/i, /opening black hole/i, /nuclear breach/i, /Zzzrrt/i, /encountered a black hole/i]],
  ['delay', 'collision', [/collided with a strange ship/i]],
  ['delay', 'particle-storm', [/particle storms/i]],
  ['delay', 'red-giant', [/red giant distorted/i]],
  ['delay', 'navigator', [/miscalculation of the navigator|landed the fleet at a completely wrong place/i]],
  ['delay', 'missed-target', [/missed it`s target|missed its target/i]],
  ['delay', 'navigation-module', [/navigation module still has a few bugs/i]],
  ['fast', 'relay', [/unforeseen relay in the energy coils/i]],
  ['fast', 'wormhole', [/unstable wormhole as a shortcut/i]],
  ['fast', 'solar-wind', [/got into a solar wind at the return flight/i]],
  ['combat', 'pirates', [/Moa Tikarr/i, /space ?pirates?/i, /star ?pirates?/i, /secret pirate base/i, /barbarian/i, /trap of some cunning pirates/i]],
  ['combat', 'aliens', [/unknown ships/i, /unknown specie/i, /activate their weapons/i, /crystalline ships of unknown origin/i, /alien invasion fleet/i, /aggressive alien race/i, /disconnected abruptly/i]],
  ['darkmatter', 'darkmatter', [/dark ?matter/i]],
  ['ships', 'ancient-battlefield', [/ancient battlefield/i]],
  ['ships', 'predecessor', [/predecessor expedition/i]],
  ['ships', 'pirate-base', [/deserted pirate base/i]],
  ['ships', 'war-wrecks', [/almost completly destroyed by wars/i]],
  ['ships', 'starbase', [/old starbase|hangar of the fortress/i]],
  ['ships', 'armada', [/remains of an armada/i]],
  ['ships', 'shipyard', [/automatic shipyard/i]],
  ['ships', 'cemetery', [/ship cemetery/i]],
  ['ships', 'perfect', [/spaceships which were in perfect condition/i]],
  ['resources', 'bacterium', [/bacterium that eats metal/i]],
  ['resources', 'virus', [/virus that will destroy crystalline/i]],
  ['stardust', 'stardust', [/rare Stardust/i]],
  ['resources', 'resources', [/raw material|asteroids? cluster|resource fields|highly-poisonous|freighter convoy|raw material deposits|civilian ships|alien shipwreck|asteroid belt|rich in raw materials|useful resources/i]],
  ['nothing', 'life-form', [/life-form of pure energy/i]],
  ['nothing', 'yellow-fever', [/yellow fever/i]],
  ['nothing', 'supernova', [/lovely pictures of a supernova/i]],
  ['nothing', 'computer-virus', [/computervirus/i]],
  ['nothing', 'red-anomaly', [/red anomalies of class 5/i]],
  ['nothing', 'emptiness', [/vast emptiness of space/i]],
  ['nothing', 'reactor', [/reactor malfuntion/i]],
  ['nothing', 'animals', [/curious, small little animals/i]],
  ['nothing', 'still-nothing', [/has not brought any real new knowledge|not very successful|empty-handed/i]],
  // Non-fatal black-hole flavour: the fleet survives and resources increase.
  ['resources', 'blackhole-loot', [/drawn into the black hole[\s\S]*resources became much more/i]],
];

// Classify an expedition message body -> coarse outcome (Go-aligned). See
// EXPEDITION_FLAVORS; returns 'unknown' only when nothing matches.
function classifyExpedition(body) {
  const b = body || '';
  for (const [outcome, , pats] of EXPEDITION_FLAVORS) if (pats.some((re) => re.test(b))) return outcome;
  return 'unknown';
}

// The specific flavour label (e.g. 'particle-storm', 'bacterium') for an
// expedition body, or null.
function expeditionFlavor(body) {
  const b = body || '';
  for (const [outcome, flavor, pats] of EXPEDITION_FLAVORS) if (pats.some((re) => re.test(b))) return { outcome, flavor };
  return { outcome: 'unknown', flavor: null };
}

// Coarse class for the full message scan, by category. Category names come from
// the live messages sidebar (Message.getMessages(id)).
function classifyMessage(m) {
  const cat = m.catName;
  if (cat === 'expedition') return classifyExpedition(m.body);
  if (cat === 'combat') return 'fight';
  if (cat === 'spy') return m.sighting ? 'spy-sighting' : 'spy-report';
  if (cat === 'system') return m.achievement ? 'achievement' : 'system';
  if (cat === 'transport') return 'transport';
  if (cat === 'player') return 'player';
  if (cat === 'alliance') return 'alliance';
  if (cat === 'construction') return 'construction';
  if (cat === 'game') return 'game';
  return 'other';
}

// Pull the delivered loot out of a return-ack body. Handles both the standard
// "It deliveres Metal X, Crystal Y, Deuterium Z and Dark Matter W." form and the
// with-DM variant "They found (W)Dark matter ... the Dark matter was saved.".
function parseExpeditionLoot(body) {
  const g = (name) => {
    const m = new RegExp(`${name}\\s+([\\d.]+)`, 'i').exec(body);
    return m ? Number(m[1].replace(/\./g, '')) : 0;
  };
  const paren = /They found\s*\(([\d.]+)\)\s*Dark ?matter/i.exec(body);
  return {
    metal: g('Metal'), crystal: g('Crystal'), deuterium: g('Deuterium'),
    darkmatter: paren ? Number(paren[1].replace(/\./g, '')) : g('Dark Matter'),
  };
}

// Fetch expedition messages (messcat=15) and append newly seen ones to data/expeditions.json.
async function cmdExpLog() {
  await login();
  const outPath = path.join(DATA_DIR, 'expeditions.json');
  let log = fs.existsSync(outPath) ? JSON.parse(fs.readFileSync(outPath, 'utf8')) : { updatedAt: null, messages: [] };
  const seen = new Set(log.messages.map((m) => m.id));
  let added = 0;
  for (let site = 1; site <= 20; site++) {
    const html = await (await getPage(`page=messages&mode=view&messcat=15&site=${site}&ajax=1`)).text();
    const rows = parseMessageRows(html);
    if (!rows.length) break;
    for (const r of rows) {
      if (!seen.has(r.id)) { seen.add(r.id); log.messages.push(r); added++; continue; }
      // Backfill fields added after a message was first logged (e.g. report hash).
      const ex = log.messages.find((m) => m.id === r.id);
      if (ex) for (const k of ['report', 'attackerLosses', 'defenderLosses', 'date', 'sender', 'subject']) {
        if (r[k] != null && ex[k] == null) ex[k] = r[k];
      }
    }
    await delay();
  }
  for (const m of log.messages) {
    const cl = expeditionFlavor(m.body || '');
    m.outcome = cl.outcome;
    m.flavor = cl.flavor;
    m.kind = m.outcome === 'return' ? 'return' : 'outcome';
    if (m.kind === 'return') m.loot = parseExpeditionLoot(m.body || '');
  }
  log.messages.sort((a, b) => Number(a.id) - Number(b.id));
  log.updatedAt = new Date().toISOString();
  const outcomes = log.messages.filter((m) => m.kind === 'outcome');
  const counts = {};
  for (const m of outcomes) counts[m.outcome] = (counts[m.outcome] || 0) + 1;
  const n = outcomes.length;
  const bh = counts.blackhole || 0;
  const summary = {
    outcomes: counts,
    outcomeSamples: n,
    returns: log.messages.length - n,
    blackholeRate: n ? +(bh / n).toFixed(4) : null,
  };
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(outPath, JSON.stringify(log, null, 2));
  console.log(`[exp-log] +${added} new, total ${log.messages.length} (${n} outcomes / ${summary.returns} returns) -> ${outPath}`);
  console.log(`[exp-log] outcomes=${JSON.stringify(counts)} blackhole=${bh}/${n} (${summary.blackholeRate})`);
  return log;
}

// Fetch and parse the combat reports attached to expedition messages. Each
// combat outcome links CombatReport.php?raport=<hash>; the report reveals the
// points-scaled enemy template (fractional counts). Cached by hash in
// data/expedition-reports.json.
async function cmdExpReports() {
  await login();
  const outPath = path.join(DATA_DIR, 'expedition-reports.json');
  const store = fs.existsSync(outPath) ? JSON.parse(fs.readFileSync(outPath, 'utf8')) : {};
  // Fight reports are not in the expedition category: harvest "Combat messages"
  // (messcat=3), whose rows carry the CombatReport.php?raport=<hash> link.
  const rows = [];
  const seen = new Set();
  for (let site = 1; site <= 20; site++) {
    const html = await (await getPage(`page=messages&mode=view&messcat=3&site=${site}&ajax=1`)).text();
    const page = parseMessageRows(html);
    if (!page.length) break;
    for (const r of page) if (!seen.has(r.id)) { seen.add(r.id); rows.push(r); }
    await delay();
  }
  const targets = rows.filter((m) => m.report);
  let fetched = 0;
  for (const m of targets) {
    // Re-fetch cached reports until the header (attackerInfo/defenderInfo) is
    // populated, so older reports get the mirrored enemy W/S/A bonus backfilled.
    if (store[m.report] && store[m.report].defenderInfo) continue;
    const html = await (await getUrl(`${BASE}/game/CombatReport.php?raport=${m.report}`)).text();
    const parsed = parseCombatReport(html);
    // Per-unit view: `count` = round-1 starting count (what was sent / the enemy
    // template), `lost` = summed across all rounds (total casualties of the fight).
    const agg = (side) => {
      const map = new Map();
      parsed.rounds.forEach((r, i) => {
        for (const u of r[side] || []) {
          const e = map.get(u.code) || { code: u.code, name: u.name, count: 0, lost: 0 };
          if (i === 0) e.count = u.count;
          e.lost += u.lost || 0;
          map.set(u.code, e);
        }
      });
      return [...map.values()];
    };
    store[m.report] = {
      hash: m.report, msgId: m.id, at: m.date || null, result: parsed.result,
      attacker: agg('attacker'), defender: agg('defender'),
      attackerInfo: parsed.attackerInfo, defenderInfo: parsed.defenderInfo,
      roundCount: parsed.roundCount, lossesRaw: parsed.lossesRaw, debris: parsed.debris,
    };
    fetched++;
    await delay();
  }
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(outPath, JSON.stringify(store, null, 2));
  const all = Object.values(store);
  console.log(`[exp-report] ${fetched} new, ${all.length} total -> ${outPath}`);
  for (const r of all) {
    const d = r.defender.map((u) => `${u.code}:${u.count}`).join(',') || '-';
    const a = r.attacker.map((u) => `${u.code}:${u.count}`).join(',') || '-';
    console.log(`  ${r.hash.slice(0, 8)} ${String(r.result || '?').padEnd(9)} atk=[${a}] def=[${d}] lostA=${r.attacker.reduce((s, u) => s + u.lost, 0)} lostD=${r.defender.reduce((s, u) => s + u.lost, 0)}`);
  }
  return store;
}

// Message categories from the live sidebar (Message.getMessages(id)).
const MSG_CATEGORIES = {
  0: 'spy', 1: 'player', 2: 'alliance', 3: 'combat', 4: 'system', 5: 'transport',
  15: 'expedition', 50: 'game', 99: 'construction', 100: 'all', 199: 'archive', 999: 'outbox',
};

// Full message scan: fetch every category (paged by `site`), parse + classify
// each row, and merge into data/messages.json. Idempotent and additive — rows
// that disappear from the live inbox (deleted/archived) stay in the local log,
// so the statistics keep accumulating. This is the comprehensive store; the
// expedition-specific fast log stays in data/expeditions.json (cmdExpLog).
// The server repeats the final page instead of returning an empty one, so paging
// stops when two consecutive pages carry the same ids (works even on a full
// re-scan where every row is already known).
//   node httpbot.mjs msg-scan [--cats 0,3,15] [--max-sites 500] [--out data/messages.json]
async function cmdMsgScan(catsArg, maxSitesArg, outArg) {
  await login();
  const outPath = path.resolve(outArg || path.join(DATA_DIR, 'messages.json'));
  const cats = catsArg
    ? catsArg.split(',').map((s) => Number(s.trim())).filter((c) => c in MSG_CATEGORIES)
    : Object.keys(MSG_CATEGORIES).map(Number).filter((c) => c !== 100);
  const maxSites = Number(maxSitesArg || 500);
  let log = fs.existsSync(outPath) ? JSON.parse(fs.readFileSync(outPath, 'utf8')) : { updatedAt: null, messages: [] };
  const byId = new Map(log.messages.map((m) => [String(m.id), m]));
  const added = {};
  const tagClass = (m) => {
    m.class = classifyMessage(m);
    if (m.catName === 'expedition') { const cl = expeditionFlavor(m.body); m.outcome = cl.outcome; m.flavor = cl.flavor; }
  };
  for (const cat of cats) {
    const catName = MSG_CATEGORIES[cat];
    let prevIds = '';
    let repeats = 0;
    let count = 0;
    for (let site = 1; site <= maxSites; site++) {
      const html = await (await getPage(`page=messages&mode=view&messcat=${cat}&site=${site}&ajax=1`)).text();
      const rows = parseMessageRows(html);
      const pageIds = rows.map((r) => r.id).join(',');
      if (!rows.length || pageIds === prevIds) { repeats++; if (repeats >= 2 || !rows.length) break; await delay(); continue; }
      repeats = 0; prevIds = pageIds;
      for (const r of rows) {
        r.cat = cat; r.catName = catName;
        const ex = byId.get(String(r.id));
        if (!ex) { tagClass(r); byId.set(String(r.id), r); added[catName] = (added[catName] || 0) + 1; }
        else { for (const k of Object.keys(r)) if (ex[k] == null) ex[k] = r[k]; ex.cat = cat; ex.catName = catName; tagClass(ex); }
        count++;
      }
      await delay();
    }
    console.log(`[msg-scan] cat=${cat} ${catName}: ${count} rows`);
  }
  log.messages = [...byId.values()].sort((a, b) => Number(a.id) - Number(b.id));
  log.updatedAt = new Date().toISOString();
  log.categories = MSG_CATEGORIES;
  const totalAdded = Object.values(added).reduce((s, n) => s + n, 0);
  console.log(`[msg-scan] +${totalAdded} new, ${log.messages.length} total -> ${outPath}`);
  console.log(`[msg-scan] new by category: ${JSON.stringify(added)}`);
  log.stats = summarizeMessages(log);
  fs.mkdirSync(path.dirname(outPath), { recursive: true });
  fs.writeFileSync(outPath, JSON.stringify(log, null, 2));
  return log;
}

// Shared statistics over the merged message log: counts per category/class, the
// expedition outcome mix, fight-report totals, achievements and spy sightings.
function summarizeMessages(log) {
  const byCat = {}, byClass = {}, expOutcome = {}, expFlavor = {};
  const fight = { count: 0, attackerLosses: 0, defenderLosses: 0, profit: { metal: 0, crystal: 0, deuterium: 0 }, rubble: { metal: 0, crystal: 0 }, xp: 0, reports: 0 };
  const ach = {}, sightings = [];
  for (const m of log.messages || []) {
    byCat[m.catName] = (byCat[m.catName] || 0) + 1;
    const cl = m.class || m.outcome || 'unknown';
    byClass[cl] = (byClass[cl] || 0) + 1;
    if (m.catName === 'expedition') {
      const oc = m.outcome || m.class || 'unknown';
      expOutcome[oc] = (expOutcome[oc] || 0) + 1;
      if (oc !== 'return') expFlavor[m.flavor || '?'] = (expFlavor[m.flavor || '?'] || 0) + 1;
    }
    if (m.catName === 'combat') {
      fight.count++;
      fight.attackerLosses += m.attackerLosses || 0;
      fight.defenderLosses += m.defenderLosses || 0;
      fight.xp += m.combatXp || 0;
      if (m.report) fight.reports++;
      if (m.profit) { fight.profit.metal += m.profit.metal; fight.profit.crystal += m.profit.crystal; fight.profit.deuterium += m.profit.deuterium; }
      if (m.rubblefield) { fight.rubble.metal += m.rubblefield.metal; fight.rubble.crystal += m.rubblefield.crystal; }
    }
    if (m.achievement) { const k = m.achievement.name; ach[k] = Math.max(ach[k] || 0, m.achievement.level); }
    if (m.sighting) sightings.push(m.sighting);
  }
  const outcomes = Object.values(expOutcome).reduce((s, n) => s + n, 0) - (expOutcome.return || 0);
  console.log(`[msg-stats] total=${(log.messages || []).length} byCategory=${JSON.stringify(byCat)}`);
  console.log(`[msg-stats] byClass=${JSON.stringify(byClass)}`);
  if (outcomes) {
    const mix = {};
    for (const [k, n] of Object.entries(expOutcome)) if (k !== 'return') mix[k] = `${n} (${(100 * n / outcomes).toFixed(1)}%)`;
    console.log(`[msg-stats] expedition outcomes (n=${outcomes}): ${JSON.stringify(mix)}`);
    console.log(`[msg-stats] expedition flavors: ${JSON.stringify(expFlavor)}`);
  }
  if (fight.count) console.log(`[msg-stats] fights=${fight.count} reports=${fight.reports} lostA=${fight.attackerLosses} lostD=${fight.defenderLosses} xp=${fight.xp} profit=${JSON.stringify(fight.profit)} rubble=${JSON.stringify(fight.rubble)}`);
  const unclassified = byClass.unknown || 0;
  const unknownRows = (log.messages || []).filter((m) => (m.class || m.outcome) === 'unknown').slice(0, 10);
  console.log(`[msg-stats] unclassified=${unclassified}`);
  for (const m of unknownRows) console.log(`   ? [${m.catName}] ${m.id} ${(m.subject || '').slice(0, 30)} :: ${(m.body || '').slice(0, 110)}`);
  return { byCat, byClass, expOutcome, expFlavor, fight, ach, sightings };
}

// Recompute statistics from an existing data/messages.json without fetching.
//   node httpbot.mjs msg-stats [--out data/messages.json]
async function cmdMsgStats(outArg) {
  const p = path.resolve(outArg || path.join(DATA_DIR, 'messages.json'));
  if (!fs.existsSync(p)) throw new Error(`no message log at ${p}; run msg-scan first`);
  const log = JSON.parse(fs.readFileSync(p, 'utf8'));
  summarizeMessages(log);
  return log;
}

// Phalanx scan (page=phalanx). Only works when the CURRENT planet is a moon
// with a Phalanx Sensor; the target must be in the SAME galaxy within
// level^2-1 systems. `--cp <moonCp>` pins the sensor moon (the current planet is
// account-global and the farm workers keep resetting it).
//   node httpbot.mjs phalanx <g:s:p> [type=1|3] [--cp <moonCp>]
async function cmdPhalanx(target, typeArg, cpArg) {
  await login();
  const m = /^(\d+):(\d+):(\d+)$/.exec(String(target || ''));
  if (!m) throw new Error('usage: phalanx <g:s:p> [1|3] [--cp <moonCp>]');
  const type = typeArg && /^[0-9]+$/.test(typeArg) ? typeArg : '1';
  const q = `page=phalanx&galaxy=${m[1]}&system=${m[2]}&planet=${m[3]}&planettype=${type}` + (cpArg ? `&cp=${cpArg}` : '');
  const scan = parsePhalanx(await (await getPage(q)).text());
  if (scan.error) { console.log(`[phalanx] ${scan.target ? '' : ''}${scan.error} (sensor cp=${cpArg || 'current'})`); return scan; }
  console.log(`[phalanx] ${scan.target.coords} (${scan.target.name || '?'}) — ${scan.fleets.length} fleet(s)`);
  for (const f of scan.fleets) {
    const comp = Object.entries(f.ships).map(([k, v]) => `${k}:${v}`).join(',');
    console.log(`  ${f.text.slice(0, 120)}`);
    if (comp) console.log(`    ${comp}`);
  }
  return scan;
}

// Battle simulator (page=battleSimulator). Input JSON file:
//   { "attacker": { "109":15, "110":15, "111":15, "202":200, "204":500, ... },
//     "defender": { "111":15, "401":500, ... } }
// Codes 1xx = techs/skills (attacker row uses them; defender row too), 2xx/4xx = ships/defenses.
// Low-level: submit one simulator spec, fetch its CombatReport. Logs in if needed.
async function runSim(spec) {
  const fields = { slots: String(spec.slots || 2) };
  for (const [side, slot] of [['attacker', 0], ['defender', 1]]) {
    for (const [code, val] of Object.entries(spec[side] || {})) {
      fields[`battleinput[0][${slot}][${code}]`] = String(val);
    }
  }
  // Extra ACS slots (allies) go to slot 2, 3, ...
  if (Array.isArray(spec.acs)) {
    spec.acs.forEach((ally, i) => {
      for (const [code, val] of Object.entries(ally || {})) fields[`battleinput[0][${2 + i}][${code}]`] = String(val);
    });
  }
  const raw = await postForm('page=battleSimulator&mode=send', fields);
  // The simulator POST should return a bare report hash. A PHP notice/error
  // page (or any HTML) means the server-side simulation crashed.
  if (!raw || raw.includes('<') || /Error Administration|NOTICE|reconnect to the game/i.test(raw)) {
    return { hash: null, html: raw || '', phpError: true };
  }
  const hash = raw.replace(/["\s]/g, '');
  if (!hash || hash.length > 64) return { hash: null, html: raw, phpError: true };
  const report = await getUrl(`${BASE}/game/CombatReport.php?raport=${hash}`);
  const html = await report.text();
  const phpError = /Error Administration|NOTICE|Try to reconnect to the game/i.test(html) || html.includes('NOTICE');
  return { hash, html, phpError };
}

async function cmdSim(file) {
  if (!file) throw new Error('need a json file');
  const spec = JSON.parse(fs.readFileSync(file, 'utf8'));
  await login();
  const { hash, html } = await runSim(spec);
  fs.mkdirSync(DATA_DIR, { recursive: true });
  if (!html) { console.log('[sim] empty response (invalid composition)'); return; }
  fs.writeFileSync(path.join(DATA_DIR, 'sim-result.html'), html);
  console.log(`[sim] hash=${hash}; report saved to data/sim-result.html`);
  console.log(stripTags(html.replace(/<script[\s\S]*?<\/script>/gi, '')).slice(0, 1500));
}

// Batch: run an array of { id, notes?, attacker:{}, defender:{} } scenarios.
// Saves each scenario's input + raw report + parsed JSON under data/combat/ and
// appends a row to data/combat/index.json. Usage: simsuite <file> [--out dir]
async function cmdSimSuite(file, outArg) {
  if (!file) throw new Error('need a scenarios json file');
  const raw = JSON.parse(fs.readFileSync(file, 'utf8'));
  const scenarios = Array.isArray(raw) ? raw : (raw.scenarios || []);
  const outDir = outArg || path.join(DATA_DIR, 'combat');
  fs.mkdirSync(outDir, { recursive: true });
  await login();
  const indexPath = path.join(outDir, 'index.json');
  const index = fs.existsSync(indexPath) ? JSON.parse(fs.readFileSync(indexPath, 'utf8')) : [];
  for (const sc of scenarios) {
    const id = sc.id || `scenario-${Date.now()}`;
    if (fs.existsSync(path.join(outDir, `${id}.report.json`))) { console.log(`[simsuite] ${id} exists; skip`); continue; }
    await delay();
    const { hash, html, phpError } = await runSim(sc);
    if (!html) { console.log(`[simsuite] ${id} -> EMPTY (invalid composition)`); continue; }
    if (phpError) {
      fs.writeFileSync(path.join(outDir, `${id}.input.json`), JSON.stringify(sc, null, 2));
      fs.writeFileSync(path.join(outDir, `${id}.error.html`), html);
      fs.writeFileSync(path.join(outDir, `${id}.report.json`), JSON.stringify({ hash, phpError: true, scenario: sc }, null, 2));
      index.push({ id, hash, at: new Date().toISOString(), notes: sc.notes || null, attacker: sc.attacker, defender: sc.defender, slots: sc.slots || 2, result: 'PHP_ERROR' });
      fs.writeFileSync(indexPath, JSON.stringify(index, null, 2));
      console.log(`[simsuite] ${id} -> PHP_ERROR (server notice)`);
      await delay();
      continue;
    }
    const parsed = parseCombatReport(html);
    fs.writeFileSync(path.join(outDir, `${id}.input.json`), JSON.stringify(sc, null, 2));
    fs.writeFileSync(path.join(outDir, `${id}.report.html`), html);
    fs.writeFileSync(path.join(outDir, `${id}.report.json`), JSON.stringify({ hash, ...parsed }, null, 2));
    index.push({
      id, hash, at: new Date().toISOString(), notes: sc.notes || null,
      attacker: sc.attacker, defender: sc.defender, slots: sc.slots || 2,
      result: parsed.result, roundCount: parsed.roundCount,
      debris: parsed.debris, moonChance: parsed.moonChance, lossesRaw: parsed.lossesRaw,
    });
    fs.writeFileSync(indexPath, JSON.stringify(index, null, 2));
    console.log(`[simsuite] ${id} -> ${parsed.result} (${parsed.roundCount} rounds) debris M${parsed.debris.metal}/C${parsed.debris.crystal}`);
    await delay();
  }
  console.log(`[simsuite] done. ${scenarios.length} scenarios -> ${outDir}`);
}

const cmd = process.argv[2] || 'levels';
const flag = (name) => { const i = process.argv.indexOf(name); return i > 0 ? process.argv[i + 1] : null; };
const steps = flag('--steps') ? parseInt(flag('--steps'), 10) : 2000;

if (!USER || !PASS) { console.error(`Missing credentials in ${SECRETS}`); process.exit(1); }
try {
  if (cmd === 'dump') await cmdDump(process.argv[3] || 'page=overview');
  else if (cmd === 'get') await cmdGet(process.argv[3]);
  else if (cmd === 'levels') await cmdLevels(flag('--out'), flag('--cp'));
  else if (cmd === 'planets') await cmdPlanets();
  else if (cmd === 'cancel') await cmdCancel();
  else if (cmd === 'trim') await cmdTrim(process.argv[3] || 'page=research');
  else if (cmd === 'redeem') await cmdRedeem(process.argv[3]);
  else if (cmd === 'academy') await cmdAcademy();
  else if (cmd === 'academy-map') await cmdAcademyMap();
  else if (cmd === 'academy-up') await cmdAcademyUp(process.argv[3], process.argv[4]);
  else if (cmd === 'fleet') await cmdFleet(process.argv[3], process.argv[4], process.argv[5], process.argv[6], process.argv.includes('--dry'), flag('--cp'));
  else if (cmd === 'fleetback') await cmdFleetBack(process.argv[3]);
  else if (cmd === 'expedition') await cmdExpedition(process.argv[3], process.argv[4], process.argv[5], process.argv[6], flag('--pve'), flag('--cp'));
  else if (cmd === 'exp-state') await cmdExpState();
  else if (cmd === 'trade') await cmdTrade(process.argv[3], process.argv[4], flag('--cp'));
  else if (cmd === 'worker') await cmdWorker(process.argv[3], process.argv[4], flag('--interval'));
  else if (cmd === 'exp-log') await cmdExpLog();
  else if (cmd === 'exp-report') await cmdExpReports();
  else if (cmd === 'msg-scan') await cmdMsgScan(flag('--cats'), flag('--max-sites'), flag('--out'));
  else if (cmd === 'msg-stats') await cmdMsgStats(flag('--out'));
  else if (cmd === 'phalanx') await cmdPhalanx(process.argv[3], process.argv[4], flag('--cp'));
  else if (cmd === 'card') await cmdCard(process.argv[3]);
  else if (cmd === 'cards') await cmdCards(flag('--out'));
  else if (cmd === 'arsenal') await cmdArsenal();
  else if (cmd === 'market') await cmdMarket();
  else if (cmd === 'activate') await cmdActivate(process.argv[3], process.argv.includes('--go'));
  else if (cmd === 'sell') await cmdSell(process.argv[3], process.argv[4], process.argv[5], process.argv.includes('--go'));
  else if (cmd === 'conveyor-probe') await cmdConveyorProbe(process.argv[3], process.argv[4], process.argv[5]);
  else if (cmd === 'sim') await cmdSim(process.argv[3]);
  else if (cmd === 'simsuite') await cmdSimSuite(process.argv[3], flag('--out'));
  else if (cmd === 'resolve') {
    const g = flag('--goals'); if (!g) throw new Error('need --goals');
    // Self-healing: a timeout/stalled fetch restarts the resolver instead of dying.
    for (;;) {
      try { await cmdResolve(g, steps, flag('--cp')); break; }
      catch (e) { console.error(`[!] resolve crashed (${e.message}); restart in 10s`); await sleep(10000); }
    }
  }
  else if (cmd === 'verify') {
    const g = flag('--goals'); if (!g) throw new Error('need --goals');
    await cmdVerify(g, flag('--cp'));
  }
  else { console.error('Use: dump|get|levels|planets|cancel|trim|redeem|academy|academy-map|academy-up|fleet|fleetback|expedition|exp-state|exp-log|exp-report|msg-scan|msg-stats|phalanx|card|cards|arsenal|market|activate|sell|conveyor-probe|trade|worker|sim|simsuite|resolve|verify'); process.exit(1); }
} catch (e) { console.error('[FATAL]', e.message); process.exit(1); }

// httpbot.mjs — browser-less niburu client (Node fetch + cookie jar).
// Same parsers as explorer.mjs, but no Chromium, so it fits a tiny VM (~40 MB).
//
// Commands:
//   node httpbot.mjs dump "page=research"      # print fetched HTML (debug)
//   node httpbot.mjs planets                   # list planets (id/name/coords)
//   node httpbot.mjs levels [--out file] [--cp id]  # current (or chosen) planet levels JSON
//   node httpbot.mjs simsuite scenarios.json [--out dir]  # batch battle-sim tests
//   node httpbot.mjs resolve --goals f.json [--steps N] [--cp id]
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseBuildPage, parseTechtreeGraph, parseQueue, parseCombatReport, stripTags, num } from './parse.mjs';

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

async function login() {
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
  const unitTargets = [];
  for (const [c, n] of Object.entries(goals.ships || {})) unitTargets.push({ code: String(c), target: Number(n), scope: 'fleet' });
  for (const [c, n] of Object.entries(goals.defenses || {})) unitTargets.push({ code: String(c), target: Number(n), scope: 'defense' });
  const unitBatch = Number(env.EXPLORER_UNIT_BATCH || 100);
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
    const unmet = [...need.entries()].filter(([c, l]) => eff(c) < l && !isSlowResearch(c));
    // Unit build tasks (ships/defenses), independent of the building/research queues.
    const unitHtml = (scope) => (scope === 'fleet' ? S.html : D.html);
    const unitTasks = [];
    for (const u of unitTargets) {
      const html = unitHtml(u.scope);
      if (!unitAvail(html, u.code)) continue;
      const have = unitVal(html, u.code);
      const exp = pendU.get(u.code);
      if (exp !== undefined) { if (have >= exp) pendU.delete(u.code); else continue; }
      if (have >= u.target) continue;
      unitTasks.push({ ...u, have, want: Math.min(unitBatch, u.target - have) });
    }
    const unitsPending = unitTargets.some((u) => unitAvail(unitHtml(u.scope), u.code) && unitVal(unitHtml(u.scope), u.code) < u.target);
    if (unmet.length === 0 && !unitsPending && !(energy !== null && energy < 0)) {
      if (gradual.length || bumpBuilders.length) {
        const bumped = [];
        const bump = (c, tag) => {
          const isB = B.byCode[Number(c)] !== undefined;
          const it = isB ? null : R.byCode[Number(c)];
          if (!isB && it && it.durationSec > maxResearchSec) return; // long research: don't grow it
          const cur = goalTargets.get(c) || 0;
          const cap = Number(caps[c] != null ? caps[c] : gradualCap);
          if (cur < cap) { goalTargets.set(c, cur + 1); bumped.push(`${c}${tag}->${cur + 1}`); }
        };
        for (const c of gradual) bump(c, '');
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
    // build a builder this cycle instead (lowest level first, so they alternate).
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
        // Make every not-yet-capped builder eligible for one more level.
        for (const c of bumpBuilders) {
          const it = B.byCode[Number(c)];
          if (!it || pendB.has(c) || it.level >= capOf(c)) continue;
          if ((goalTargets.get(c) || 0) < it.level + 1) goalTargets.set(c, it.level + 1);
        }
        const candidates = bumpBuilders
          .map((c) => ({ code: Number(c), it: B.byCode[Number(c)] }))
          .filter((x) => x.it && !pendB.has(String(x.code)) && x.it.level < capOf(String(x.code))
            && (goalTargets.get(String(x.code)) || 0) > x.it.level && buildable(x.it) && affordable(x.it))
          .sort((a, b) => a.it.level - b.it.level);
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
        pendB.clear(); pendR.clear(); pendU.clear();
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
      pendU.set(u.code, u.have + u.want);
      await delay();
      console.log(`[+] ${u.scope} ${u.want}x code ${u.code} (have ${u.have}/${u.target})`);
    }
    stalls = 0;
  }
  console.log('[+] resolve done');
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
//   step2 POST target coords + speed + mission=0 + token
//   step3 POST token + univers_<id> + mission + resources + staytime
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
    token: token2, fleet_group: '0', mission: '0',
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
  else { console.error('Use: dump|get|levels|planets|cancel|trim|redeem|academy|academy-map|academy-up|fleet|fleetback|sim|simsuite|resolve'); process.exit(1); }
} catch (e) { console.error('[FATAL]', e.message); process.exit(1); }

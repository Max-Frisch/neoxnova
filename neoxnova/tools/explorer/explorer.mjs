// explorer.mjs — Game environment explorer for niburuspace.com (XNova "GOW").
// Authenticates with credentials from ../../secrets/explorer.env, then either
// scans game data (read-only) or drives builds to discover the tech tree.
//
// Usage:
//   node explorer.mjs scan   [--headed]
//   node explorer.mjs build  [--steps 40] [--headed]
import { chromium } from 'playwright-core';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseBuildPage, parseInfo, parseTechtree, parseQueue, stripTags, num } from './parse.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const SECRETS = path.resolve(__dirname, '../../secrets/explorer.env');
const DATA_DIR = path.join(__dirname, 'data');
const STATE_FILE = path.join(__dirname, 'session.state.json');

const STRUCTURES = [1, 2, 3, 4, 6, 12, 14, 15, 21, 22, 23, 24, 31, 33, 34, 44, 71, 72, 73];
const TECHS = [106, 108, 109, 110, 111, 113, 114, 115, 117, 118, 120, 121, 122, 123, 124, 125, 131, 132, 133, 199];
const SHIPS = [202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219, 220, 221, 222, 223, 225, 226, 227, 228];
const DEFENSES = [401, 402, 403, 404, 405, 406, 407, 408, 409, 410, 411, 412, 413, 414, 415, 416, 417, 418, 419, 502, 503];

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
const BASE_URL = (env.NIBURU_BASE_URL || 'https://niburuspace.com').replace(/\/$/, '');
const GAME_URL = `${BASE_URL}/game/game.php`;
const USER = env.NIBURU_USER;
const PASS = env.NIBURU_PASS;
const HEADLESS = !process.argv.includes('--headed');
const MIN_DELAY = Number(env.EXPLORER_MIN_DELAY_MS || 350);
const MAX_DELAY = Number(env.EXPLORER_MAX_DELAY_MS || 900);

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const humanDelay = () => sleep(MIN_DELAY + Math.random() * (MAX_DELAY - MIN_DELAY));

const STEALTH = `
Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
window.chrome = { runtime: {} };
Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
Object.defineProperty(navigator, 'languages', { get: () => ['de-DE', 'de', 'en-US', 'en'] });
`;

async function pageName(page) {
  const url = page.url();
  return url.includes('game.php') && !url.includes('index.php');
}

async function loggedIn(page) {
  if (!(await pageName(page))) return false;
  return (await page.$('#current_metal, #res_block_metall, #topbar, .resources')) != null;
}

async function login(page) {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 45000 });
  await humanDelay();
  const result = await page.evaluate(async ([u, p]) => {
    try {
      const r = await fetch(`index.php?page=login&mode=send&username=${encodeURIComponent(u)}&password=${encodeURIComponent(p)}&remember=1`, { credentials: 'same-origin' });
      const text = await r.text();
      let json = null;
      try { json = JSON.parse(text); } catch { /* not json */ }
      return { status: r.status, json, text: text.slice(0, 200) };
    } catch (e) { return { error: true, message: String(e) }; }
  }, [USER, PASS]);
  console.log('[debug] login response:', JSON.stringify({ status: result.status, json: result.json, text: result.error ? result.message : result.text }));
  if (result.error) throw new Error(`Login request failed: ${result.message}`);
  if (result.json && result.json.error) throw new Error(`Login rejected: ${result.json.message || JSON.stringify(result.json)}`);

  const cookies = await page.context().cookies();
  console.log('[debug] cookies after login:', cookies.map((c) => c.name).join(', '));

  await humanDelay();
  await page.goto(`${BASE_URL}/game.php`, { waitUntil: 'domcontentloaded', timeout: 45000 });
  await humanDelay();
  if (page.url() !== GAME_URL && !page.url().includes('game.php')) {
    await page.goto(GAME_URL, { waitUntil: 'domcontentloaded', timeout: 45000 });
    await humanDelay();
  }
  if (!(await loggedIn(page))) {
    fs.mkdirSync(DATA_DIR, { recursive: true });
    fs.writeFileSync(path.join(DATA_DIR, 'debug-login.html'), await page.content());
    throw new Error(`Login done but game session not detected (url=${page.url()})`);
  }
}

async function dismissModals(page) {
  // Auto-opened fancybox popups (e.g. manualinfo) intercept pointer events.
  await page.evaluate(() => {
    for (const sel of ['.fancybox-close', 'a.fancybox-item.fancybox-close', '.fancybox-overlay', '#fancybox-overlay']) {
      const el = document.querySelector(sel);
      if (el) { try { el.click(); } catch {} if (el.style) el.style.display = 'none'; }
    }
    const wrap = document.querySelector('#fancybox-wrap');
    if (wrap) wrap.style.display = 'none';
  }).catch(() => {});
}

async function goto(page, url) {
  for (let i = 0; i < 3; i++) {
    try {
      await humanDelay();
      await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 45000 });
      await dismissModals(page);
      return;
    } catch (e) { console.error(`[!] goto ${url} attempt ${i + 1}: ${e.message}`); await sleep(3000); }
  }
  throw new Error(`Failed to navigate: ${url}`);
}

async function withBrowser(fn) {
  if (!USER || !PASS) throw new Error(`Missing credentials in ${SECRETS}`);
  const ctxOpts = {
    viewport: { width: 1920, height: 1080 },
    userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36',
    locale: 'de-DE',
  };
  if (fs.existsSync(STATE_FILE)) ctxOpts.storageState = STATE_FILE;
  // Browser: Edge on Windows dev, system Chromium on Linux VMs. Override with
  // EXPLORER_CHANNEL (e.g. chromium) or EXPLORER_EXECUTABLE_PATH.
  const launchOpts = {
    headless: HEADLESS,
    args: ['--disable-blink-features=AutomationControlled', '--disable-dev-shm-usage', '--no-sandbox'],
  };
  if (process.env.EXPLORER_EXECUTABLE_PATH) launchOpts.executablePath = process.env.EXPLORER_EXECUTABLE_PATH;
  else launchOpts.channel = process.env.EXPLORER_CHANNEL || 'msedge';
  const browser = await chromium.launch(launchOpts);
  const context = await browser.newContext(ctxOpts);
  await context.addInitScript(STEALTH);
  const page = await context.newPage();
  page.on('dialog', (d) => d.accept().catch(() => {}));
  try {
    await goto(page, GAME_URL);
    if (!(await loggedIn(page))) await login(page);
    if (!(await loggedIn(page))) throw new Error('Could not establish a game session');
    console.log(`[+] Logged in as ${USER}`);
    await context.storageState({ path: STATE_FILE }).catch(() => {});
    return await fn(page, context);
  } finally {
    await browser.close();
  }
}

async function scan(page) {
  const result = { variant: 'XNova GOW - niburuspace.com', scannedAt: new Date().toISOString(), baseUrl: BASE_URL, info: {}, pages: {} };
  const kinds = [['structure', STRUCTURES], ['tech', TECHS], ['ship', SHIPS], ['defense', DEFENSES]];
  for (const [kind, ids] of kinds) {
    result[kind + 's'] = {};
    for (const id of ids) {
      await goto(page, `${GAME_URL}?page=information&id=${id}`);
      const html = await page.content();
      const info = parseInfo(html);
      if (info.name) result[kind + 's'][id] = info;
      console.log(`  [${kind} ${id}] ${info.name} ${JSON.stringify(info.stats)}`);
    }
  }
  const named = { buildings: 'page=buildings', research: 'page=research', shipyardFleet: 'page=shipyard&mode=fleet', shipyardDefense: 'page=shipyard&mode=defense', resources: 'page=resources', premium: 'page=premium', techtree: 'page=techtree' };
  for (const [key, q] of Object.entries(named)) {
    await goto(page, `${GAME_URL}?${q}`);
    const html = await page.content();
    if (key === 'techtree') result.requirements = parseTechtree(html);
    else if (key === 'premium') result.premium = stripTags(html).slice(0, 3000);
    else result.pages[key] = parseBuildPage(html);
    console.log(`  [page] ${key} ok`);
  }
  fs.mkdirSync(DATA_DIR, { recursive: true });
  const out = path.join(DATA_DIR, `scan-${Date.now()}.json`);
  fs.writeFileSync(out, JSON.stringify(result, null, 2));
  console.log(`[+] Wrote ${out}`);
  return result;
}

function resourcesFromPage(html) {
  const grab = (id) => { const m = new RegExp(`id="${id}"[^>]*>([\\d.]+)`).exec(html); return m ? num(m[1]) : 0; };
  return { metal: grab('current_metal'), crystal: grab('current_crystal'), deuterium: grab('current_deuterium') };
}

// Economy-first target plan: raise production + storage, then prerequisites.
// Strategy: metal ~2-3 above crystal, deuterium ~3-4 below crystal, solar just
// high enough for energy. robot factory stays put once shipyard is unlocked.
let BUILD_PLAN = [
  { code: 31, target: 5 },  // Research Lab (unlocks research + officers)
  { code: 21, target: 8 },  // Shipyard (needed for Nanite + ships)
  { code: 14, target: 16 }, // Robot Factory
  { code: 15, target: 5 },  // Nanite Factory (match robot roughly)
  { code: 4, target: 33 },  // Solar Plant (keep energy covered)
  { code: 1, target: 35 },  // Metal Mine
  { code: 2, target: 33 },  // Crystal Mine (metal - 2)
  { code: 3, target: 29 },  // Deuterium Refinery (crystal - 4)
  { code: 22, target: 12 }, // Metal Storage
  { code: 23, target: 12 }, // Crystal Storage
  { code: 24, target: 12 }, // Deuterium Storage
  { code: 33, target: 1 },  // Terraformer
];

// Optional plan override: EXPLORER_PLAN=plans/account2.json
const planFile = process.env.EXPLORER_PLAN;
if (planFile && fs.existsSync(planFile)) {
  BUILD_PLAN = JSON.parse(fs.readFileSync(planFile, 'utf8'));
  console.log(`[plan] loaded ${planFile} (${BUILD_PLAN.length} targets)`);
}

async function queueRowCount(page) {
  return page.evaluate(() => document.querySelectorAll('#buildlist .element_row').length).catch(() => -1);
}

async function cancelQueue(page) {
  for (let i = 0; i < 60; i++) {
    await goto(page, `${GAME_URL}?page=buildings`);
    const before = await queueRowCount(page);
    const submitted = await page.evaluate(() => {
      const forms = [...document.querySelectorAll('#buildlist form')];
      const f = forms.find((x) => x.querySelector('input[name="cmd"][value="cancel"]'));
      if (!f) return false;
      f.submit();
      return true;
    });
    if (!submitted) { console.log(`[*] Queue empty (rows=${before}).`); break; }
    await sleep(600 + Math.random() * 500);
    console.log(`[-] Cancelled one item (rows before=${before})`);
  }
  await goto(page, `${GAME_URL}?page=buildings`);
  console.log(`[*] Remaining queue rows: ${await queueRowCount(page)}`);
}

async function buildSatellites(page, count) {
  await goto(page, `${GAME_URL}?page=shipyard&mode=fleet`);
  const input = await page.$('input[name="fmenge[212]"]');
  if (!input) { console.log('[-] Solar Satellite not buildable yet (need Shipyard).'); return; }
  await input.fill(String(count));
  await sleep(400 + Math.random() * 400);
  const form = await input.evaluateHandle((el) => el.closest('form'));
  const btn = await form.asElement().$('input[type="submit"], button[type="submit"]');
  if (btn) await btn.click();
  await sleep(800 + Math.random() * 500);
  await goto(page, `${GAME_URL}?page=shipyard&mode=fleet`);
  const html = await page.content();
  const avail = /id="val_212"[^>]*>([\d.]+)/.exec(html);
  console.log(`[+] Queued ${count} Solar Satellites${avail ? ` (now have ${avail[1]})` : ''}.`);
}

async function queueDump(page) {
  await goto(page, `${GAME_URL}?page=buildings`);
  const html = await page.content();
  for (const kw of ['00h ', 'cancel', 'Cancel', 'onlist', 'cmd=']) {
    const i = html.indexOf(kw);
    console.log(`\n### '${kw}' @ ${i}`);
    if (i >= 0) console.log(html.slice(Math.max(0, i - 400), i + 700));
  }
}

const MAP_PAGES = {
  overview: 'page=overview',
  resources: 'page=resources',
  academy: 'page=academy',
  premium: 'page=premium',
  senat: 'page=senat',
  officier: 'page=officier',
  gubernators: 'page=gubernators',
  arsenal: 'page=arsenal',
  bonus: 'page=bonus',
  market: 'page=market',
};

async function mapPages(page) {
  const dir = path.join(DATA_DIR, `map-${Date.now()}`);
  fs.mkdirSync(dir, { recursive: true });
  for (const [name, q] of Object.entries(MAP_PAGES)) {
    await goto(page, `${GAME_URL}?${q}`);
    const html = await page.content();
    fs.writeFileSync(path.join(dir, `${name}.html`), html);
    const text = stripTags(html.replace(/<script[\s\S]*?<\/script>/gi, ''));
    fs.writeFileSync(path.join(dir, `${name}.txt`), text);
    const title = (/<title>([^<]*)<\/title>/.exec(html) || [, ''])[1].trim();
    const dm = /Dark Matter[:\s]*([\d.]+)/i.exec(text);
    console.log(`  ${name.padEnd(12)} len=${String(html.length).padStart(6)} DM=${dm ? dm[1] : '?'}  title="${title}"`);
  }
  console.log(`[+] Saved mapped pages to ${dir}`);
  return dir;
}

async function getProduction(page) {
  await goto(page, `${GAME_URL}?page=resources`);
  const html = await page.content();
  // resourceTicker production values are the server-computed effective rates.
  const prod = [...html.matchAll(/production:\s*([\d.]+)/g)].map((m) => Math.round(parseFloat(m[1])));
  const text = stripTags(html);
  const lack = /Lack of energy:\s*(\d+)%/.exec(text);
  const free = /Free energy:\s*(\d+)%/.exec(text);
  return {
    metal: prod[0] || 0, crystal: prod[1] || 0, deuterium: prod[2] || 0,
    lackEnergy: lack ? +lack[1] : 0, freeEnergy: free ? +free[1] : 0,
  };
}

async function officerLevels(page) {
  await goto(page, `${GAME_URL}?page=officier`);
  const text = stripTags(await page.content());
  const out = {};
  for (const m of text.matchAll(/(Geologist|Admiral|Engineer|Technocrat|Constructor|Scientologist|Minister of Defence)\s*\(Level\s*(\d+)\/(\d+)\)/g)) {
    out[m[1]] = { level: +m[2], max: +m[3] };
  }
  const dm = /Dark Matter[:\s]*([\d.]+)/i.exec(text);
  out._dm = dm ? num(dm[1]) : null;
  return out;
}

async function recruitOfficer(page, id) {
  await goto(page, `${GAME_URL}?page=officier`);
  const input = await page.$(`form[action="game.php?page=officier"] input[name="id"][value="${id}"]`);
  if (!input) return false;
  const form = await input.evaluateHandle((el) => el.closest('form'));
  const btn = await form.asElement().$('button[type="submit"]');
  if (!btn) return false;
  await btn.click();
  await sleep(900 + Math.random() * 600);
  return true;
}

const OFFICERS = [[601, 'Geologist'], [602, 'Admiral'], [603, 'Engineer'], [604, 'Technocrat'], [605, 'Constructor'], [606, 'Scientologist'], [607, 'Minister of Defence']];

async function officerExperiment(page) {
  const results = [];
  let prod = await getProduction(page);
  let levels = await officerLevels(page);
  console.log(`[start] production=${JSON.stringify(prod)}`);
  console.log(`[start] DM=${levels._dm} officers=${JSON.stringify(Object.fromEntries(Object.entries(levels).filter(([k]) => k !== '_dm')))}`);
  for (const [id, name] of OFFICERS) {
    const beforeLvl = levels[name]?.level ?? 0;
    const before = prod;
    const beforeLevels = levels;
    const ok = await recruitOfficer(page, id);
    if (!ok) { console.log(`[-] ${name}: could not recruit`); continue; }
    levels = await officerLevels(page);
    prod = await getProduction(page);
    const afterLvl = levels[name]?.level ?? 0;
    const rec = {
      id, name, from: beforeLvl, to: afterLvl, dmBefore: beforeLevels._dm, dmAfter: levels._dm,
      prodBefore: before, prodAfter: prod,
      deltaMetalPct: before.metal ? +(((prod.metal - before.metal) / before.metal) * 100).toFixed(3) : null,
      deltaCrystalPct: before.crystal ? +(((prod.crystal - before.crystal) / before.crystal) * 100).toFixed(3) : null,
      deltaDeutPct: before.deuterium ? +(((prod.deuterium - before.deuterium) / before.deuterium) * 100).toFixed(3) : null,
      deltaFreeEnergy: prod.freeEnergy - before.freeEnergy,
    };
    results.push(rec);
    console.log(`[+] ${name} L${beforeLvl}->L${afterLvl} DM ${beforeLevels._dm}->${levels._dm}  Δprod M${rec.deltaMetalPct}% C${rec.deltaCrystalPct}% D${rec.deltaDeutPct}%  ΔfreeEnergy ${rec.deltaFreeEnergy}  (prod M${prod.metal} C${prod.crystal} D${prod.deuterium})`);
  }
  fs.mkdirSync(DATA_DIR, { recursive: true });
  const out = path.join(DATA_DIR, `officers-${Date.now()}.json`);
  fs.writeFileSync(out, JSON.stringify({ results }, null, 2));
  console.log(`[+] Wrote ${out}`);
}

async function levels(page, outArg) {
  const data = { updatedAt: new Date().toISOString(), account: USER, buildings: {}, research: {}, ships: {}, defenses: {}, resources: {} };
  const pages = [['buildings', 'buildings'], ['research', 'research'], ['ships', 'shipyard&mode=fleet'], ['defenses', 'shipyard&mode=defense']];
  for (const [key, q] of pages) {
    await goto(page, `${GAME_URL}?page=${q}`);
    for (const it of parseBuildPage(await page.content())) data[key][it.code] = { name: it.name, level: it.level };
  }
  await goto(page, `${GAME_URL}?page=overview`);
  data.resources = resourcesFromPage(await page.content());
  const outPath = outArg || 'data/levels.json';
  fs.mkdirSync(path.dirname(outPath), { recursive: true });
  fs.writeFileSync(outPath, JSON.stringify(data, null, 2));
  console.log(`WROTE ${outPath}`);
  console.log(JSON.stringify(data));
}

async function status(page) {
  for (const pageNameStr of ['buildings', 'research', 'shipyard&mode=fleet']) {
    await goto(page, `${GAME_URL}?page=${pageNameStr}`);
    const html = await page.content();
    const items = parseBuildPage(html);
    const shown = items.filter((it) => it.level > 0 || it.hasBuild);
    console.log(`\n--- ${pageNameStr} ---`);
    for (const it of shown) console.log(`${String(it.code).padStart(3)} ${it.name.padEnd(28)} L${String(it.level).padStart(3)} M${it.cost.metal} C${it.cost.crystal} D${it.cost.deuterium} T${it.durationSec}`);
    const queue = [...html.matchAll(/class="[^"]*\bonlist\b[^"]*"[^>]*>([\s\S]{0,120}?)<\//g)].map((m) => stripTags(m[1])).filter((s) => s && s.length > 2).slice(0, 12);
    const timers = [...html.matchAll(/[0-9]{2}h [0-9]{2}m [0-9]{2}s|[0-9]{2}m [0-9]{2}s/g)].map((m) => m[0]);
    console.log(`  queue-labels: ${JSON.stringify(queue)}`);
    console.log(`  visible-timers: ${JSON.stringify(timers.slice(0, 12))} (count=${timers.length})`);
  }
  // Resources from overview
  await goto(page, `${GAME_URL}?page=overview`);
  const res = resourcesFromPage(await page.content());
  console.log(`\nResources: metal=${res.metal} crystal=${res.crystal} deuterium=${res.deuterium}`);
}

async function build(page, steps) {
  const observed = [];
  const skipped = new Set();
  let built = 0, stalls = 0;
  for (let i = 0; i < steps && stalls < 70; i++) {
    await goto(page, `${GAME_URL}?page=buildings`);
    const html = await page.content();
    if (!(await loggedIn(page))) { await login(page); continue; }
    const res = resourcesFromPage(html);
    const items = parseBuildPage(html);
    const byCode = Object.fromEntries(items.map((it) => [it.code, it]));
    const nameToCode = Object.fromEntries(items.map((it) => [it.name, it.code]));
    const queued = {};
    for (const q of parseQueue(html)) { const c = nameToCode[q.name]; if (c) queued[c] = (queued[c] || 0) + 1; }
    const effectiveLevel = (code) => (byCode[code]?.level ?? 0) + (queued[code] ?? 0);
    const next = BUILD_PLAN.find((p) => !skipped.has(p.code) && effectiveLevel(p.code) < p.target);
    if (!next) { console.log('[*] Build plan complete (or all remaining targets skipped).'); break; }

    const it = byCode[next.code];
    const hasCost = it && (it.cost.metal + it.cost.crystal + it.cost.deuterium) > 0;
    if (!hasCost) { console.log(`[-] ${next.code} (${it ? it.name : '?'}) locked; skipping for this run`); skipped.add(next.code); continue; }
    if (!it.hasBuild) { console.log(`[~] build queue busy (no form for ${it.name}); waiting`); stalls++; await sleep(20000); continue; }
    if (res.metal < it.cost.metal || res.crystal < it.cost.crystal || res.deuterium < it.cost.deuterium) {
      console.log(`[~] waiting for resources for ${it.name} (need M${it.cost.metal} C${it.cost.crystal} D${it.cost.deuterium}, have M${res.metal} C${res.crystal} D${res.deuterium})`);
      stalls++; await sleep(10000); continue;
    }

    const targetLevel = it.level + 1;
    const form = await page.$(`#build_${next.code} form.build_form`);
    if (!form) { console.log(`[~] queue busy (no form) for ${it.name}`); stalls++; await sleep(20000); continue; }
    const lvlInput = await form.$('input[name="lvlup"]');
    if (lvlInput) await lvlInput.fill(String(targetLevel));
    await sleep(250 + Math.random() * 300);
    const btn = await form.$('button[type="submit"]');
    if (btn) await btn.click();
    await sleep(400 + Math.random() * 500);

    await goto(page, `${GAME_URL}?page=buildings`);
    const afterIt = parseBuildPage(await page.content()).find((x) => x.code === next.code);
    const changed = afterIt && afterIt.level > it.level;
    observed.push({ code: next.code, name: it.name, fromLevel: it.level, toLevel: targetLevel, changed, at: new Date().toISOString() });
    if (changed) { built++; stalls = 0; console.log(`[+] ${it.name} L${it.level} -> L${afterIt.level}`); }
    else { stalls++; console.log(`[~] ${it.name} queued (still L${it.level}) stalls=${stalls}`); }
  }
  fs.mkdirSync(DATA_DIR, { recursive: true });
  const out = path.join(DATA_DIR, `build-${Date.now()}.json`);
  fs.writeFileSync(out, JSON.stringify({ built, observed }, null, 2));
  console.log(`[+] Applied ${built} build(s). Wrote ${out}`);
}

const cmd = process.argv[2] || 'scan';
const stepsArg = process.argv.indexOf('--steps');
const steps = stepsArg > 0 ? parseInt(process.argv[stepsArg + 1], 10) : 30;
const outArg = (() => { const i = process.argv.indexOf('--out'); return i > 0 ? process.argv[i + 1] : null; })();

await withBrowser(async (page) => {
  if (cmd === 'scan') await scan(page);
  else if (cmd === 'build') await build(page, steps);
  else if (cmd === 'status') await status(page);
  else if (cmd === 'queue') await queueDump(page);
  else if (cmd === 'cancel') await cancelQueue(page);
  else if (cmd === 'sats') await buildSatellites(page, steps);
  else if (cmd === 'map') await mapPages(page);
  else if (cmd === 'officers') await officerExperiment(page);
  else if (cmd === 'levels') await levels(page, outArg);
  else if (cmd === 'login') console.log('[+] login ok');
  else { console.error('Unknown command. Use: scan | build | status | queue | cancel | sats | login'); process.exit(1); }
}).catch((e) => { console.error('[FATAL]', e.message); process.exit(1); });

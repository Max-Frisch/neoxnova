// httpbot.mjs — browser-less niburu client (Node fetch + cookie jar).
// Same parsers as explorer.mjs, but no Chromium, so it fits a tiny VM (~40 MB).
//
// Commands:
//   node httpbot.mjs dump "page=research"      # print fetched HTML (debug)
//   node httpbot.mjs levels [--out file]       # current levels JSON
//   node httpbot.mjs resolve --goals f.json [--steps N]
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseBuildPage, parseTechtreeGraph, parseQueue, stripTags, num } from './parse.mjs';

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
  return fetch(url, { ...opts, headers, redirect: 'manual' });
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

async function cmdLevels(outArg) {
  await login();
  const B = await scope('page=buildings');
  const R = await scope('page=research');
  const S = await scope('page=shipyard&mode=fleet');
  const D = await scope('page=shipyard&mode=defense');
  const data = {
    updatedAt: new Date().toISOString(), account: USER,
    buildings: B.levels, research: R.levels, ships: S.levels, defenses: D.levels, resources: B.res,
    names: Object.fromEntries([...Object.values(B.byCode), ...Object.values(R.byCode), ...Object.values(S.byCode), ...Object.values(D.byCode)].map((it) => [it.code, it.name])),
  };
  const out = outArg || 'data/levels.json';
  fs.mkdirSync(path.dirname(out), { recursive: true });
  fs.writeFileSync(out, JSON.stringify(data, null, 2));
  console.log(`WROTE ${out}`);
  console.log(JSON.stringify(data));
}

async function cmdResolve(goalsPath, steps) {
  const goals = JSON.parse(fs.readFileSync(goalsPath, 'utf8'));
  await login();
  let graph = {};
  const tt = await (await getPage('page=techtree')).text();
  graph = parseTechtreeGraph(tt);
  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(path.join(DATA_DIR, 'techtree-graph.json'), JSON.stringify(graph, null, 2));
  console.log(`[graph] ${Object.keys(graph).length} items`);

  const goalMap = {};
  for (const kind of ['buildings', 'research']) for (const [c, l] of Object.entries(goals[kind] || {})) goalMap[c] = Math.max(goalMap[c] || 0, l);

  let stalls = 0;
  for (let step = 0; step < steps && stalls < 120; step++) {
    const B = await scope('page=buildings');
    const R = await scope('page=research');
    const eff = (c) => (B.levels[c] ?? R.levels[c] ?? 0) + (B.queued[c] ?? 0) + (R.queued[c] ?? 0);
    const need = {};
    const visit = (c, l) => { if (!need[c] || need[c] < l) need[c] = l; for (const r of graph[c] || []) visit(r.id, r.required); };
    for (const [c, l] of Object.entries(goalMap)) visit(c, l);

    const unmet = Object.entries(need).filter(([c, l]) => eff(c) < l);
    if (unmet.length === 0) { console.log('[*] All goals satisfied.'); break; }
    const actionable = unmet.find(([c]) => {
      if (!B.byCode[c] && !R.byCode[c]) return false; // unknown/unit
      if (!B.byCode[c] && eff(31) < 1) return false;   // research needs a lab
      return (graph[c] || []).every((r) => eff(r.id) >= r.required);
    });
    if (!actionable) { console.log('[~] no actionable; waiting'); stalls++; await delay(); await sleep(15000); continue; }
    const code = Number(actionable[0]);
    const isBuilding = B.byCode[code] !== undefined;
    const it = (isBuilding ? B : R).byCode[code];
    if (!it) { stalls++; continue; }
    if ((it.cost.metal + it.cost.crystal + it.cost.deuterium) === 0) { console.log(`[-] ${it.name} locked; skipping`); stalls++; continue; }
    if (!it.hasBuild) { console.log(`[~] ${it.name} queue busy; waiting`); stalls++; await delay(); await sleep(15000); continue; }
    const res = B.res;
    if (res.metal < it.cost.metal || res.crystal < it.cost.crystal || res.deuterium < it.cost.deuterium) { console.log(`[~] resources for ${it.name}`); stalls++; await delay(); await sleep(15000); continue; }
    await delay();
    if (isBuilding) await postForm('page=buildings', { cmd: 'insert', building: code, lvlup: it.level + 1 });
    else await postForm('page=research', { cmd: 'insert', tech: code, lvlup: it.level + 1 });
    await delay();
    stalls = 0;
    console.log(`[+] ${isBuilding ? 'build' : 'research'} ${it.name} L${it.level} -> L${it.level + 1}`);
  }
  console.log('[+] resolve done');
}

const cmd = process.argv[2] || 'levels';
const flag = (name) => { const i = process.argv.indexOf(name); return i > 0 ? process.argv[i + 1] : null; };
const steps = flag('--steps') ? parseInt(flag('--steps'), 10) : 2000;

if (!USER || !PASS) { console.error(`Missing credentials in ${SECRETS}`); process.exit(1); }
try {
  if (cmd === 'dump') await cmdDump(process.argv[3] || 'page=overview');
  else if (cmd === 'levels') await cmdLevels(flag('--out'));
  else if (cmd === 'resolve') { const g = flag('--goals'); if (!g) throw new Error('need --goals'); await cmdResolve(g, steps); }
  else { console.error('Use: dump|levels|resolve'); process.exit(1); }
} catch (e) { console.error('[FATAL]', e.message); process.exit(1); }

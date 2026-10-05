// rapidfire-scan.mjs — fetch every ship/defense info page and extract the two
// rapid-fire tables separately (the generic parseInfo merged "He makes" and
// "He gets", which corrupted the earlier scan).
//
// Output: data/rapidfire-matrix.json
//   { units: { "<code>": { name, attack, shield, armor, speed, fuel, cargo,
//                          weaponType, makes:{<targetName>:shots}, gets:{<sourceName>:shots} } },
//     nameToCode: { "<lowercased name>": "<code>" } }
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

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
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const strip = (s) => s.replace(/<[^>]+>/g, ' ').replace(/&[a-z#0-9]+;/g, ' ').replace(/\s+/g, ' ').trim();
const num = (s) => { const v = parseInt(String(s).replace(/[^\d-]/g, ''), 10); return Number.isNaN(v) ? 0 : v; };

const jar = new Map();
const cookieHeader = () => [...jar].map(([k, v]) => `${k}=${v}`).join('; ');
async function get(url) {
  const res = await fetch(url, { headers: { Cookie: cookieHeader(), 'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)' }, redirect: 'follow' });
  const sc = res.headers.getSetCookie ? res.headers.getSetCookie() : [];
  for (const c of sc) { const p = c.split(';')[0]; const i = p.indexOf('='); jar.set(p.slice(0, i).trim(), p.slice(i + 1).trim()); }
  return await res.text();
}

function parseUnit(html) {
  // Rows -> [label, value]. Preserve order; split by the two section headers.
  const body = html.replace(/<script[\s\S]*?<\/script>/gi, '').replace(/<style[\s\S]*?<\/style>/gi, '');
  const rows = [...body.matchAll(/<tr[^>]*>([\s\S]*?)<\/tr>/gi)].map((m) =>
    [...m[1].matchAll(/<(?:th|td)[^>]*>([\s\S]*?)<\/(?:th|td)>/gi)].map((c) => strip(c[1])));

  const out = { makes: {}, gets: {} };
  let mode = null;
  for (const cells of rows) {
    if (!cells.length) continue;
    const label = cells[0].toLowerCase();
    const val = cells[1] || '';
    if (label.includes('makes shots per round')) { mode = 'makes'; continue; }
    if (label.includes('gets shots per round')) { mode = 'gets'; continue; }
    if (label === 'weapon type') { mode = 'weapon'; out.weaponType = val; continue; }
    if (label === 'structural armor') { mode = null; out.armor = num(val); continue; }
    if (label === 'shields') { mode = null; out.shield = num(val); continue; }
    if (label === 'fuel used(deuterium)') { mode = null; out.fuel = num(val); continue; }
    if (label === 'cargo capacity') { mode = null; out.cargo = num(val); continue; }
    if (mode === 'weapon') { out.attack = num(val); mode = null; continue; }
    if (mode === 'makes' && cells.length >= 2) { out.makes[cells[0]] = num(cells[1]); continue; }
    if (mode === 'gets' && cells.length >= 2) { out.gets[cells[0]] = num(cells[1]); continue; }
    mode = null;
  }
  const titleM = /<title>([\s\S]*?)<\/title>/.exec(html);
  out.name = titleM ? strip(titleM[1]).replace(/\s*-\s*Niburu Space.*/, '').trim() : '';
  return out;
}

async function main() {
  const ids = [];
  for (let i = 202; i <= 228; i++) ids.push(i);
  for (let i = 401; i <= 419; i++) ids.push(i);
  ids.push(502, 503);

  await get(`${BASE}/`);
  await get(`${BASE}/index.php?page=login&mode=send&username=${encodeURIComponent(USER)}&password=${encodeURIComponent(PASS)}&remember=1`);
  await get(`${BASE}/game.php`);

  const units = {};
  for (const id of ids) {
    await sleep(300);
    try {
      const html = await get(`${BASE}/game/game.php?page=information&id=${id}`);
      if (!/makes shots per round/i.test(html)) { console.log(`[skip] ${id} (no stat page)`); continue; }
      const u = parseUnit(html);
      units[id] = u;
      console.log(`[ok] ${id} ${u.name} atk=${u.attack} sh=${u.shield} armor=${u.armor} makes=${Object.keys(u.makes).length} gets=${Object.keys(u.gets).length}`);
    } catch (e) { console.log(`[ERR] ${id}: ${e.message}`); }
  }

  const nameToCode = {};
  for (const [code, u] of Object.entries(units)) if (u.name) nameToCode[u.name.toLowerCase()] = code;

  fs.mkdirSync(DATA_DIR, { recursive: true });
  fs.writeFileSync(path.join(DATA_DIR, 'rapidfire-matrix.json'), JSON.stringify({ units, nameToCode }, null, 2));
  console.log(`WROTE data/rapidfire-matrix.json (${Object.keys(units).length} units)`);
}

main().catch((e) => { console.error('[FATAL]', e); process.exit(1); });

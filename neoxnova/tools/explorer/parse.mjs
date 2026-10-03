// parse.mjs — HTML parsers for niburuspace.com (XNova "GOW" theme).
// Pure functions: take response HTML, return plain JSON-able objects.

export function stripTags(s) {
  return s.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
}

export function num(s) {
  if (s == null) return 0;
  const cleaned = String(s).replace(/[.\u00a0,\s]/g, '').replace(/[^0-9-]/g, '');
  const n = parseInt(cleaned, 10);
  return Number.isNaN(n) ? 0 : n;
}

export function parseDurationSec(s) {
  const m = /(?:(\d+)d)?\s*(?:(\d+)h)?\s*(?:(\d+)m)?\s*(?:(\d+)s)?/.exec(String(s));
  if (!m) return 0;
  const d = +(m[1] || 0), h = +(m[2] || 0), mi = +(m[3] || 0), se = +(m[4] || 0);
  return d * 86400 + h * 3600 + mi * 60 + se;
}

// Parse a buildings/research/shipyard page into items with level + next cost + duration.
export function parseBuildPage(html) {
  const items = [];
  const parts = html.split('class="build_box');
  for (const ch of parts.slice(1)) {
    const codeM = /(?:id="[sd]_|Dialog\.info\()(\d+)/.exec(ch);
    const titleM = /class="title">\s*([\s\S]*?)\s*<\/a>/.exec(ch);
    if (!codeM || !titleM) continue;
    const code = parseInt(codeM[1], 10);
    let title = stripTags(titleM[1]);
    let locked = title.includes('(locked)') || title.includes('???');
    let name = title, level = 0;
    const lm = /^(.*?)\s+(\d+)\s+Level$/.exec(title);
    if (lm) { name = lm[1].trim(); level = parseInt(lm[2], 10); }
    const cost = { metal: 0, crystal: 0, deuterium: 0 };
    const priceRe = /class="price res90([123])[^"]*"[\s\S]*?class="text[^"]*"[^>]*>\s*([\d.]+)\s*<\/div>/g;
    let pm;
    while ((pm = priceRe.exec(ch)) !== null) {
      const v = num(pm[2]);
      if (pm[1] === '1') cost.metal = v;
      else if (pm[1] === '2') cost.crystal = v;
      else if (pm[1] === '3') cost.deuterium = v;
    }
    const dm = /Duration:\s*<span>([^<]+)<\/span>/.exec(ch);
    const hasBuild = /name="cmd"\s+value="insert"/.test(ch) || /name="cmd" value="insert"/.test(ch);
    items.push({ code, name, level, cost, durationSec: dm ? parseDurationSec(dm[1]) : 0, locked, hasBuild });
  }
  return items;
}

// Parse an information&id=X page: description, effect text and unit stat table.
export function parseInfo(html) {
  const titleM = /class="title">\s*([\s\S]*?)\s*<\/a>/.exec(html) || /<title>([\s\S]*?)<\/title>/.exec(html);
  const name = titleM ? stripTags(titleM[1]).replace(/\s*-\s*Niburu.*$/, '').trim() : '';

  // Remove scripts/styles, then flatten table rows into [label,value] pairs.
  const body = html.replace(/<script[\s\S]*?<\/script>/gi, '').replace(/<style[\s\S]*?<\/style>/gi, '');
  const rows = [...body.matchAll(/<tr[^>]*>([\s\S]*?)<\/tr>/gi)].map((m) => {
    const cells = [...m[1].matchAll(/<(th|td)[^>]*>([\s\S]*?)<\/\1>/gi)].map((c) => stripTags(c[2]));
    return cells;
  });

  const stats = {};
  const weapons = [];
  const rapidfire = [];
  let mode = null;
  for (const cells of rows) {
    if (cells.length === 0) continue;
    const label = cells[0].toLowerCase();
    if (label === 'weapon type') { mode = 'weapon'; continue; }
    if (label === 'structural armor') { mode = 'armor'; continue; }
    if (label === 'shields') { mode = 'shield'; continue; }
    if (label === 'engine') { mode = 'engine'; continue; }
    if (label.includes('shots per round')) { mode = 'rapidfire'; continue; }
    if (label === 'fuel used(deuterium)') { mode = null; stats.fuel = num(cells[1]); continue; }
    if (label === 'cargo capacity') { mode = null; stats.cargo = num(cells[1]); continue; }

    if (mode === 'weapon' && cells.length >= 2) weapons.push({ type: cells[0], attack: num(cells[1]) });
    else if (mode === 'armor' && cells.length >= 2) stats.armor = num(cells[1]);
    else if (mode === 'shield' && cells.length >= 2) stats.shield = num(cells[1]);
    else if (mode === 'engine' && cells.length >= 2) { stats.engine = cells[0]; stats.speed = num(cells[1]); }
    else if (mode === 'rapidfire' && cells.length >= 2) rapidfire.push({ target: cells[0], shots: num(cells[1]) });
    else mode = null;
  }
  if (weapons.length) stats.weapons = weapons;
  if (rapidfire.length) stats.rapidfire = rapidfire;

  const text = stripTags(body);
  return { name, stats, description: text.slice(0, 1200) };
}

// Best-effort requirement extraction from the techtree page.
export function parseTechtree(html) {
  const body = html.replace(/<script[\s\S]*?<\/script>/gi, '');
  const reqs = [];
  const re = /Dialog\.info\((\d+)\)[^>]*>\s*([^<(]+?)\s*\(Level\s+(\d+)\s*\/\s*(\d+)\)/gi;
  let m;
  while ((m = re.exec(body)) !== null) {
    reqs.push({ id: +m[1], name: m[2].trim(), current: +m[3], required: +m[4] });
  }
  return reqs;
}

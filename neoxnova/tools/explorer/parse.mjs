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
    // Shipyard "factory" rate: "Building: 288 per second". Per-ship Duration is
    // rounded to 0 (instant), so this rate is the real throughput.
    const rm = /Building:\s*(?:<span>\s*)?([\d.,]+)\s*per second/i.exec(ch);
    const hasBuild = /name="cmd"\s+value="insert"/.test(ch) || /name="cmd" value="insert"/.test(ch);
    items.push({ code, name, level, cost, durationSec: dm ? parseDurationSec(dm[1]) : 0, perSec: rm ? num(rm[1]) : 0, locked, hasBuild });
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

// Parse an information&id=X unit card. A unit can list SEVERAL weapon rows
// (each with its own base attack), so `weapons` is captured in full; the other
// slots are a labelled header row followed by one data row:
//   Weapon type / Attack Power   -> weapons: [{type, attack}, ...]
//   Structural armor             -> [armorClass, hull]
//   Shields                      -> [shieldClass, shield]
//   Engine / Base speed          -> [engineClass, speed]
// The stat cells carry inline bonus tooltips (e.g. "225.000 +117.000"), so only
// the leading numeric token is read. All values are the unit's BASE stats.
export function parseInfoCard(html) {
  const body = html.replace(/<script[\s\S]*?<\/script>/gi, '').replace(/<style[\s\S]*?<\/style>/gi, '');
  // Cells are split per row so an unclosed final cell (the engine speed cell is
  // missing its </td> on the live cards) still runs to the end of its own row.
  const rows = [...body.matchAll(/<tr[^>]*>([\s\S]*?)<\/tr>/gi)].map((m) =>
    [...m[1].matchAll(/<(?:th|td)[^>]*>([\s\S]*?)(?:<\/(?:th|td)>|$)/gi)].map((c) => c[1]));
  const leadNum = (s) => { const m = /(-?[\d][\d.,]*)/.exec(stripTags(s)); return m ? num(m[1]) : 0; };
  // Inline bonuses are spans like: <span data-tooltip-content="Laser technology">+117.000</span>
  const bonuses = (raw) => [...String(raw).matchAll(/data-tooltip-content="([^"]*)"[^>]*>\s*([+-]?[\d.,]+)/gi)]
    .map((m) => ({ source: stripTags(m[1]), amount: num(m[2].replace(/[+-]/g, '')) }));

  const out = { weapons: [] };
  let mode = null;
  for (const cells of rows) {
    const first = stripTags(cells[0] || '');
    const label = first.toLowerCase();
    if (label === 'weapon type') { mode = 'weapon'; continue; }
    if (label === 'structural armor') { mode = 'armor'; continue; }
    if (label === 'shields') { mode = 'shield'; continue; }
    if (label === 'engine') { mode = 'engine'; continue; }
    if (label === 'fuel used(deuterium)') { mode = null; out.fuel = leadNum(cells[1] || ''); continue; }
    if (label === 'cargo capacity') { mode = null; out.cargo = leadNum(cells[1] || ''); continue; }
    if (label.includes('shots per round')) { mode = null; continue; }

    if (mode === 'weapon') {
      if (first) out.weapons.push({ type: first, attack: leadNum(cells[1] || ''), bonuses: bonuses(cells[1] || '') });
    } else if (mode === 'armor') { out.armorClass = first; out.hull = leadNum(cells[1] || ''); out.armorBonuses = bonuses(cells[1] || ''); mode = null; }
    else if (mode === 'shield') { out.shieldClass = first; out.shield = leadNum(cells[1] || ''); out.shieldBonuses = bonuses(cells[1] || ''); mode = null; }
    else if (mode === 'engine') { out.engineClass = first; out.speed = leadNum(cells[1] || ''); out.engineBonuses = bonuses(cells[1] || ''); mode = null; }
  }
  out.attack = out.weapons.reduce((s, w) => s + w.attack, 0);
  if (out.weapons.length) out.weaponType = out.weapons[0].type;
  const nameM = /<div id="content">[\s\S]*?<th[^>]*>\s*([^<]+?)\s*<\/th>/i.exec(body)
    || /<title>([\s\S]*?)<\/title>/i.exec(html);
  out.name = nameM ? stripTags(nameM[1]).replace(/\s*-\s*Niburu.*$/, '').trim() : '';
  return out;
}

// Parse the Arsenal page (game.php?page=arsenal): one entry per upgrade with the
// current accumulated bonus, the bracket the NEXT activation would add, how many
// un-activated drawings are held, and the activate-form `greid` key when present
// (Absent upgrades have no form). See docs/ARSENAL_UPGRADES_IMPLEMENTATION.md §1.1.
export function parseArsenalPage(html) {
  const out = [];
  for (const ch of html.split('class="build_box"').slice(1)) {
    const headM = /class="head">\s*([\s\S]*?)\s*<\/div>/.exec(ch);
    const name = headM ? stripTags(headM[1]) : '';
    if (!name) continue;
    const bonusM = /Bonus:\s*<span[^>]*>\s*([+-]?[\d.]+)\s*%\s*<\/span>\s*(?:<sup>\s*\(([+-]?[\d.]+)\)\s*<\/sup>)?/i.exec(ch);
    const availM = /Avaiable:\s*<span[^>]*>\s*([\d.]+)/i.exec(ch);
    const greidM = /name="greid"\s+value="([^"]+)"/i.exec(ch);
    out.push({
      name,
      bonus: bonusM ? Number(bonusM[1]) : 0,
      nextBonus: bonusM && bonusM[2] != null ? Number(bonusM[2]) : null,
      available: availM ? num(availM[1]) : 0,
      greid: greidM ? greidM[1] : null,
    });
  }
  return out;
}

// Parse live market lots (game.php?page=market): each lot is a BuyUpgrade form
// wrapping a table row. Returns { id, name, amount, priceAtm } per lot.
// See docs/ARSENAL_UPGRADES_IMPLEMENTATION.md §1.2.
export function parseMarketLots(html) {
  const lots = [];
  for (const m of html.matchAll(/<form[^>]*action="game\.php\?page=market"[^>]*>([\s\S]*?)<\/form>/gi)) {
    const block = m[1];
    if (!/name="mode"\s+value="BuyUpgrade"/.test(block)) continue;
    const idM = /name="id"\s+value="(\d+)"/.exec(block);
    const cells = [...block.matchAll(/<td[^>]*>([\s\S]*?)<\/td>/gi)].map((c) => stripTags(c[1]).replace(/\s+/g, ' ').trim());
    if (cells.length < 4) continue;
    lots.push({
      id: idM ? Number(idM[1]) : num(cells[0]),
      name: cells[1],
      amount: num(cells[2]),
      priceAtm: num(cells[3]),
    });
  }
  return lots;
}

// Parse the active construction queue rows: [{ name, level }].
export function parseQueue(html) {
  const out = [];
  const re = /class="band_process"[\s\S]*?<span>\s*\d+\.\s*<\/span>\s*([^<]+?)\s+(\d+)\s*</g;
  let m;
  while ((m = re.exec(html)) !== null) out.push({ name: m[1].trim(), level: parseInt(m[2], 10) });
  return out;
}

// Parse the techtree into a requirement graph: { itemId: [{id, required}] }.
// Each table row is one item; its third cell holds required_block anchors of
// the form "Name (Level current / required)".
export function parseTechtreeGraph(html) {
  const graph = {};
  for (const row of html.split('<tr').slice(1)) {
    const itemM = /Dialog\.info\((\d+)\)/.exec(row);
    if (!itemM) continue;
    const itemId = +itemM[1];
    if (!graph[itemId]) graph[itemId] = [];
    const blockRe = /required_block[\s\S]*?Dialog\.info\((\d+)\)[\s\S]*?class="text"[^>]*>\s*(\d+)\s*\/\s*(\d+)/g;
    let m;
    while ((m = blockRe.exec(row)) !== null) {
      const id = +m[1], required = +m[3];
      if (id !== itemId && !graph[itemId].some((r) => r.id === id)) graph[itemId].push({ id, required });
    }
  }
  return graph;
}

// Parse a CombatReport.php battle report (XNova GOW theme).
// Returns per-round attacker/defender unit snapshots, per-round damage summary,
// the final result/losses and the debris/moon/recycle block.
export function parseCombatReport(html) {
  const n = (s) => { if (s == null) return 0; const v = parseInt(String(s).replace(/[^\d-]/g, ''), 10); return Number.isNaN(v) ? 0 : v; };
  const derived = (b) => ({
    firepower: (/Firepower:<\/td>[\s\S]*?<span\s*>([\d.]+)<\/span>/.exec(b) || [])[1],
    shield: (/Shield:<\/td>[\s\S]*?<span\s*>([\d.]+)<\/span>/.exec(b) || [])[1],
    armour: (/Armour:<\/td>[\s\S]*?>([\d.]+)</.exec(b) || [])[1],
  });
  const parseUnits = (str) => {
    const out = [];
    for (const b of str.split('<div class="batle_unit">').slice(1)) {
      const name = (/class="name_unit">([^<]*)</.exec(b) || [])[1];
      const code = Number((/gebaeude\/(\d+)\.gif/.exec(b) || [])[1] || 0);
      const cnt = /<\/span><br\/>\s*([\d.]+)\s*<br\/>/.exec(b);
      const lost = /class="destruct_unit">-?([\d.]+)</.exec(b);
      const d = derived(b);
      out.push({
        code, name: name ? name.trim() : '',
        count: cnt ? n(cnt[1]) : 0,
        lost: lost ? n(lost[1]) : 0,
        firepower: d.firepower != null ? n(d.firepower) : null,
        shield: d.shield != null ? n(d.shield) : null,
        armour: d.armour != null ? n(d.armour) : null,
      });
    }
    return out;
  };

  const rounds = [];
  for (const m of html.matchAll(/<div class="batle_round" id="round_(\d+)">([\s\S]*?)<!--\/round-->/g)) {
    const body = m[2];
    const di = body.indexOf('batle_part_def');
    rounds.push({
      n: Number(m[1]),
      attacker: parseUnits(di >= 0 ? body.slice(0, di) : body),
      defender: parseUnits(di >= 0 ? body.slice(di) : ''),
    });
  }

  // Round-1 member header: name/coords + the aggregated Firepower/Shield/Armour
  // bonuses actually applied in the fight. Expedition NPCs (Pirates/Aliens) show
  // ONE rolled Weapons/Shield/Armour value on all three (e.g. "Aliens +202%"),
  // unlike the player's three distinct research values. This is the only place
  // the mirrored enemy research is visible, so it must be captured.
  const header = (b) => {
    const m = /batle_mem_header">\s*<span>([^<]*)<\/span>(?:\s*\(([^)]*)\))?<br>\s*Firepower <span>([^<]*)<\/span>[^<]*?Shield <span>([^<]*)<\/span>[^<]*?Armour <span>([^<]*)<\/span>/.exec(b);
    if (!m) return null;
    const pct = (s) => { const v = parseFloat(String(s).replace(/[^\d.-]/g, '')); return Number.isNaN(v) ? null : v; };
    return { name: m[1].trim(), coords: (m[2] || '').trim() || null, firepower: pct(m[3]), shield: pct(m[4]), armour: pct(m[5]) };
  };
  const r1 = (html.match(/<div class="batle_round" id="round_1">([\s\S]*?)<!--\/round-->/) || [])[1] || html;
  const di = r1.indexOf('batle_part_def');
  const attackerInfo = header(di >= 0 ? r1.slice(0, di) : r1);
  const defenderInfo = header(di >= 0 ? r1.slice(di) : '');

  const bands = [...html.matchAll(/class="band_att tooltip"[^>]*data-tooltip-content="([\s\S]*?)">/g)]
    .map((m) => m[1].replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim());
  rounds.forEach((r, i) => {
    const raw = bands[i];
    if (!raw) return;
    const nums = [...raw.matchAll(/(\d[\d.]*)/g)].map((x) => n(x[1]));
    r.damage = {
      raw,
      attackerFirepower: nums[0] ?? null,
      defenderShieldAbsorb: nums[1] ?? null,
      defenderFirepower: nums[2] ?? null,
      attackerShieldAbsorb: nums[3] ?? null,
    };
  });

  const rinfos = [...html.matchAll(/<div class="batle_round_info"[^>]*>\s*<h2[^>]*>([\s\S]*?)<\/h2>/g)]
    .map((m) => m[1].replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim());
  const itog = /class="band_itog tooltip"[^>]*data-tooltip-content="([\s\S]*?)">/.exec(html);
  const text = ((/<div class="batle_text">([\s\S]*?)<\/div>/.exec(html) || [])[1] || '')
    .replace(/<[^>]+>/g, ' ').replace(/&nbsp;/g, ' ').replace(/\s+/g, ' ').trim();

  const resultText = rinfos[rinfos.length - 1] || '';
  const result = /draw/i.test(resultText) ? 'draw'
    : /attacker.*(won|winner|wins)/i.test(resultText) ? 'attacker'
    : /defender.*(won|winner|wins)/i.test(resultText) ? 'defender'
    : resultText || null;

  return {
    rounds,
    roundCount: rounds.length,
    result,
    resultText,
    attackerInfo,
    defenderInfo,
    lossesRaw: itog ? itog[1].replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim() : null,
    debris: {
      metal: n((/now:\s*([\d.]+)\s*Metal/i.exec(text) || [])[1]),
      crystal: n((/and\s*([\d.]+)\s*Crystal/i.exec(text) || [])[1]),
    },
    moonChance: n((/Moon Chance:\s*([\d.]+)\s*%/i.exec(text) || [])[1]),
    text,
  };
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

// Parse an in-game Phalanx scan (page=phalanx&galaxy=&system=&planet=&planettype=).
// The scan only works from a moon carrying a Phalanx Sensor, SAME galaxy as the
// target, within level^2-1 systems. Returns the investigated target, the fleets
// in movement (composition revealed, independent of owner) and an error string
// ("Out of reach" / "This is your planet!") when the scan is refused.
export function parsePhalanx(html) {
  const n = (s) => { const v = parseInt(String(s).replace(/[^\d-]/g, ''), 10); return Number.isNaN(v) ? 0 : v; };
  const targetM = /Investigate position \[([^\]]+)\](?:\s*\(([^)]*)\))?/.exec(html);
  const bodyM = /ally_contents">\s*([\s\S]*?)<\/div>/.exec(html);
  const error = !targetM && bodyM ? stripTags(bodyM[1]).trim() : null;
  const fleets = [];
  // The tooltip table contains its own <tr>/<td>, so never split on those: take
  // the span between consecutive `class="fleets"` cells instead.
  const starts = [...html.matchAll(/<td[^>]*class="fleets"[^>]*>/g)].map((m) => m.index);
  for (let i = 0; i < starts.length; i++) {
    const seg = html.slice(starts[i], starts[i + 1] ?? starts[i] + 6000);
    const tip = (/data-tooltip-content="([^"]*)"/.exec(seg) || [])[1] || '';
    const ships = {};
    for (const sm of tip.matchAll(/>([^<>:]+):<\/td><td[^>]*>([\d.]+)</g)) ships[sm[1].trim()] = n(sm[2]);
    const stripped = stripTags(seg).replace(/\s+/g, ' ').trim();
    const dm = /(Fleets\b[\s\S]*)$/.exec(stripped);
    fleets.push({
      seconds: n((/data-fleet-time="(\d+)"/.exec(seg) || [])[1]),
      endTime: n((/data-fleet-end-time="(\d+)"/.exec(seg) || [])[1]) || null,
      ships,
      text: (dm ? dm[1] : stripped).slice(0, 300),
    });
  }
  const target = targetM ? {
    coords: targetM[1], name: (targetM[2] || '').trim() || null,
    galaxy: +targetM[1].split(':')[0], system: +targetM[1].split(':')[1], position: +targetM[1].split(':')[2],
  } : null;
  return { target, fleets, error };
}

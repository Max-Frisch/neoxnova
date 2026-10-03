// achievements.mjs — extract achievement definitions from a niburuspace.com HAR.
// Usage: node achievements.mjs "<path to .har>" [out.json]
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { stripTags } from './parse.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const harPath = process.argv[2];
if (!harPath) { console.error('usage: node achievements.mjs <file.har> [out.json]'); process.exit(1); }
const outPath = process.argv[3] || path.join(__dirname, 'data', 'achievements.json');

const har = JSON.parse(fs.readFileSync(harPath, 'utf8'));
const body = (e) => {
  const c = e.response?.content || {};
  if (c.encoding === 'base64') return Buffer.from(c.text || '', 'base64').toString('utf8');
  return c.text || '';
};

const groups = {};
for (const e of har.log.entries) {
  const url = e.request?.url || '';
  if (!/page=achievement/.test(url)) continue;
  const g = (/group=(\w+)/.exec(url) || [, 'all'])[1];
  groups[g] = body(e);
}

// Each achievement block looks like:
//   <div class="achiv-wrap ...">
//     <div class="achiv-points">...343</div>
//     <div ... data-tooltip-content="Bonus at the next level: <br> &bull; 1.217 Antimatter <br> &bull; 122 Achievement Points">
//     <span>Metal Miner  16 lvl. </span>
//     ... Requirements: <br><span>Metal mine 51 lvl.</span>
const items = [];
for (const [group, html] of Object.entries(groups)) {
  for (const ch of html.split('class="achiv-wrap').slice(1)) {
    const pointsM = /achiv-points[\s\S]*?>(\d[\d.]*)</.exec(ch);
    const bonusM = /Bonus at the next level:[\s\S]*?([\d.]+)\s*Antimatter\s*<br>[\s\S]*?([\d.]+)\s*Achievement Points/.exec(ch);
    const nameM = /text-center b-col1">\s*<span>([\s\S]*?)<\/span>/.exec(ch);
    const reqM = /achiv-req">Requirements:<\/span>\s*<br>\s*([\s\S]*?)<\/div>/.exec(ch);
    if (!nameM) continue;
    const toInt = (s) => Number(String(s || '').replace(/\./g, '').replace(/[^\d]/g, '') || 0);
    items.push({
      group,
      name: stripTags(nameM[1]),
      requirement: reqM ? stripTags(reqM[1]) : '',
      levelPoints: toInt(pointsM && pointsM[1]),
      nextAntimatter: bonusM ? toInt(bonusM[1]) : 0,
      nextPoints: bonusM ? toInt(bonusM[2]) : 0,
    });
  }
}

const out = { source: path.basename(harPath), extractedAt: new Date().toISOString(), count: items.length, items };
fs.mkdirSync(path.dirname(outPath), { recursive: true });
fs.writeFileSync(outPath, JSON.stringify(out, null, 2));

const byGroup = {};
for (const it of items) byGroup[it.group] = (byGroup[it.group] || 0) + 1;
console.log(`extracted ${items.length} achievements from ${Object.keys(groups).length} group pages`);
console.log('by group:', byGroup);
for (const it of items) console.log(`  [${it.group}] ${it.name.padEnd(26)} need: ${it.requirement.padEnd(30)} reward: +${it.nextAntimatter} AM, +${it.nextPoints} pts (tier ${it.levelPoints})`);
console.log(`wrote ${outPath}`);

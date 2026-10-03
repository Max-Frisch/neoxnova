// summarize.mjs — print a readable summary of a scan JSON file.
// Usage: node summarize.mjs [data/scan-*.json]
import fs from 'node:fs';
import path from 'node:path';

const file = process.argv[2] || fs.readdirSync('data').filter((f) => f.startsWith('scan-')).sort().pop();
const j = JSON.parse(fs.readFileSync(path.resolve('data', file), 'utf8'));

const line = (it) => `${String(it.code).padStart(3)} ${(it.name || '?').padEnd(28)} L${String(it.level).padStart(3)} locked=${it.locked ? 'Y' : 'n'} build=${it.hasBuild ? 'Y' : 'n'} M${it.cost.metal} C${it.cost.crystal} D${it.cost.deuterium} T${it.durationSec}`;

console.log(`file: ${file}`);
console.log(`structures=${Object.keys(j.structures || {}).length} techs=${Object.keys(j.techs || {}).length} ships=${Object.keys(j.ships || {}).length} defenses=${Object.keys(j.defenses || {}).length} requirements=${(j.requirements || []).length}`);
console.log('\n--- buildings ---');
for (const it of j.pages?.buildings || []) console.log(line(it));
console.log('\n--- research ---');
for (const it of j.pages?.research || []) console.log(line(it));
console.log('\n--- shipyard fleet ---');
for (const it of j.pages?.shipyardFleet || []) console.log(line(it));
console.log('\n--- shipyard defense ---');
for (const it of j.pages?.shipyardDefense || []) console.log(line(it));
console.log('\n--- techtree requirements (first 60) ---');
for (const r of (j.requirements || []).slice(0, 60)) console.log(`${r.name.padEnd(30)} id${String(r.id).padStart(3)}  ${r.current}/${r.required}`);
console.log('\n--- premium text (trimmed) ---');
console.log(String(j.premium || '').slice(0, 1500));

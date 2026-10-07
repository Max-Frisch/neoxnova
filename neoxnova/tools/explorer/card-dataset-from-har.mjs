// card-dataset-from-har.mjs — extract every unit information card from a saved
// HAR capture (the explorer HAR records page=information&id=<code> fetches) into
// data/unit-info.json, the same raw shape `httpbot.mjs cards` writes, so
// unit-classes-dataset.mjs can fold it into the committed fixture.
//
// Usage (from tools/explorer/):
//   node card-dataset-from-har.mjs [path/to/file.har] [data/unit-info.json]
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseInfoCard } from './parse.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const SRC = process.argv[2] || path.join(__dirname, '../../niburuspace.com_recording_newAccount_Bratwurst.har');
const OUT = process.argv[3] || path.join(__dirname, 'data/unit-info.json');

const har = JSON.parse(fs.readFileSync(SRC, 'utf8'));
const entries = (har.log && har.log.entries) || [];

const units = {};
let kept = 0;
for (const e of entries) {
  const url = (e.request && e.request.url) || '';
  const m = /game\.php\?page=information&id=(\d+)/.exec(url);
  if (!m) continue;
  const id = Number(m[1]);
  if (id < 200) continue; // units are 2xx/4xx; skip techs (1xx)
  if (units[id]) continue;
  const content = (e.response && e.response.content) || {};
  let html = content.text || '';
  if (content.encoding === 'base64') html = Buffer.from(html, 'base64').toString('utf8');
  if (!/Structural armor/i.test(html)) continue;
  units[id] = parseInfoCard(html);
  kept++;
}

fs.mkdirSync(path.dirname(OUT), { recursive: true });
fs.writeFileSync(OUT, JSON.stringify({
  updatedAt: (har.log && har.log.pages && har.log.pages[0] && har.log.pages[0].startedDateTime) || null,
  account: 'Bratwurst (HAR)',
  source: path.basename(SRC),
  units,
}, null, 2));
console.log(`WROTE ${OUT} (${kept} unit cards from ${entries.length} entries)`);

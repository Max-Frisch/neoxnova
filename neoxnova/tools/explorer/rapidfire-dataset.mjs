// rapidfire-dataset.mjs — fold data/rapidfire-matrix.json (raw info-page parse)
// into the committed testdata/niburus_rapidfire.json, translating RF target
// NAMES to numeric unit codes. Run from tools/explorer/.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const SRC = path.join(__dirname, 'data', 'rapidfire-matrix.json');
const OUT = path.resolve(__dirname, '../../testdata/niburus_rapidfire.json');

// Target-name -> numeric code. Names are as they appear in the info-page
// "He makes shots per round" table.
const NAME_TO_CODE = {
  'light cargo': '202', 'heavy cargo': '203', 'light fighter': '204', 'heavy fighter': '205',
  'cruiser': '206', 'battleship': '207', 'colony ship': '208', 'recycler': '209',
  'spy probe': '210', 'planet bomber': '211', 'solar satellite': '212', 'star fighter': '213',
  'battle fortress': '214', 'battle cruiser': '215', 'black moon': '216', 'battle transporter': '217',
  'avatar': '218', 'battle recycler': '219', 'dark matter collector': '220',
  'battleship class onill': '221', 'flying death': '222', 'scrappy': '223',
  'galleon': '225', 'destroyer': '226', 'frigate': '227', 'black wanderer': '228',
  'missile launcher': '401', 'light laser turret': '402', 'heavy laser turret': '403',
  'gauss cannon': '404', 'ion cannon': '405', 'plasma cannon': '406',
  'small shield dome': '407', 'large shield dome': '408', 'atmospheric shield': '409',
  'gravitons cannon': '410', 'orbital defence platform': '411', 'lepton gun': '412',
  'proton gun': '413', 'canyon': '414', 'quantum gun': '415', 'hydrogen gun': '416',
  'dora gun': '417', 'photon cannon': '418', 'particle emitter': '419',
};

// Sim-validated steady shots/round vs satellites / light fighters (no-screen
// runs, tech 0). Used as a sanity cross-check that the info table is truthful.
const VALIDATED = {
  '206': { target: '212', sim: 11, info: 10 },
  '207': { target: '212', sim: 25, info: 25 },
  '211': { target: '212', sim: 1, info: null },
  '213': { target: '212', sim: 50, info: 50 },
  '215': { target: '212', sim: 50, info: 50 },
  '216': { target: '212', sim: 350, info: 350 },
  '225': { target: '212', sim: 125, info: 125 },
  '226': { target: '212', sim: 1, info: null },
  '227': { target: '212', sim: 1, info: null },
  '228': { target: '212', sim: 500, info: 500 },
  '216_lf': { target: '204', sim: 181, info: 180 },
  '228_lf': { target: '204', sim: 301, info: 300 },
};

const { units } = JSON.parse(fs.readFileSync(SRC, 'utf8'));

const rapidFire = {};
const unmapped = {};
for (const [code, u] of Object.entries(units)) {
  const rows = {};
  for (const [name, shots] of Object.entries(u.makes || {})) {
    const tc = NAME_TO_CODE[name.toLowerCase().trim()];
    if (tc) rows[tc] = shots;
    else { (unmapped[code] ||= {})[name] = shots; }
  }
  if (Object.keys(rows).length) rapidFire[code] = rows;
}

const out = {
  meta: {
    source: 'niburuspace.com unit information pages ("He makes shots per round")',
    capturedAt: new Date().toISOString(),
    note: 'RF is directional: attacker[shooterCode][targetCode] = shots per round vs that target. Incoming RF is the transpose. The earlier generic scan merged "makes"+"gets" tables and is unreliable — this fixture exists because of that.',
    round1Note: 'Simulator round 1 yields ~0.70x the steady shots; rounds 2+ use the steady value. Steady shots/round == the info-table value (validated).',
  },
  rapidFire,
  unmappedTargets: unmapped,
  validated: VALIDATED,
};

fs.writeFileSync(OUT, JSON.stringify(out, null, 2));
const n = Object.values(rapidFire).reduce((a, r) => a + Object.keys(r).length, 0);
console.log(`WROTE ${OUT}`);
console.log(`shooters=${Object.keys(rapidFire).length} entries=${n} unmappedShooters=${Object.keys(unmapped).length}`);

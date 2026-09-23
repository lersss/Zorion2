// tools/e2e/race-ships-variant-check.js
// Node check for the race-ship visual logic (spec 2026-09-23-корабли-рас... §6.3,
// §6.4, N8): imports the real client module (web/static/js/map/ship_sprites.js)
// and exercises spriteForAgent(race_id, id) / setShipOptions without a browser
// (the module touches DOM only in getShipImage/recolorShipSprite, which are not
// called here). Verifies: determinism, different races -> different ships,
// races without a pool (cryo_sky/geysers/ice_plankton) and unknown/empty race ->
// neutral pool, incomplete suffix sets never fall to neutral, and registry-growth
// stability (adding a variant changes only its slot).
// Run: node race-ships-variant-check.js
import { setShipOptions, spriteForAgent } from '../../web/static/js/map/ship_sprites.js';

let ok = 0;
let fail = 0;
const check = (name, cond, extra = '') => {
  if (cond) { ok++; console.log('PASS  ' + name); }
  else { fail++; console.log('FAIL  ' + name + (extra ? '  ' + extra : '')); }
};

// Synthetic /me.ship_options: humans (12) + coastal (3, full) + ammonia (2,
// incomplete {_02,_03}) + neutral. Races cryo_sky/geysers/ice_plankton are
// deliberately absent (no ship in the registry -> neutral pool).
const baseOptions = [
  { id: 'race_humans_starship', file: 'race_humans_starship.png', race: 'humans' },
  { id: 'race_humans_starship_02', file: 'race_humans_starship_02.png', race: 'humans' },
  { id: 'race_humans_starship_03', file: 'race_humans_starship_03.png', race: 'humans' },
  { id: 'race_humans_cruiser', file: 'race_humans_cruiser.png', race: 'humans' },
  { id: 'race_humans_cruiser_02', file: 'race_humans_cruiser_02.png', race: 'humans' },
  { id: 'race_humans_cruiser_03', file: 'race_humans_cruiser_03.png', race: 'humans' },
  { id: 'race_humans_carrier', file: 'race_humans_carrier.png', race: 'humans' },
  { id: 'race_humans_carrier_02', file: 'race_humans_carrier_02.png', race: 'humans' },
  { id: 'race_humans_carrier_03', file: 'race_humans_carrier_03.png', race: 'humans' },
  { id: 'race_humans_fighter', file: 'race_humans_fighter.png', race: 'humans' },
  { id: 'race_humans_fighter_02', file: 'race_humans_fighter_02.png', race: 'humans' },
  { id: 'race_humans_fighter_03', file: 'race_humans_fighter_03.png', race: 'humans' },
  { id: 'race_coastal_01', file: 'race_coastal_01.png', race: 'coastal' },
  { id: 'race_coastal_02', file: 'race_coastal_02.png', race: 'coastal' },
  { id: 'race_coastal_03', file: 'race_coastal_03.png', race: 'coastal' },
  { id: 'race_ammonia_02', file: 'race_ammonia_02.png', race: 'ammonia' },
  { id: 'race_ammonia_03', file: 'race_ammonia_03.png', race: 'ammonia' },
  { id: 'neutral', file: 'neutral.png', race: '' },
];

const coastalPool = new Set(['race_coastal_01.png', 'race_coastal_02.png', 'race_coastal_03.png']);
const ammoniaPool = new Set(['race_ammonia_02.png', 'race_ammonia_03.png']);

// --- determinism + race pool ---
setShipOptions(baseOptions);
const first = spriteForAgent('coastal', 'agent-1');
const second = spriteForAgent('coastal', 'agent-1');
check('deterministic: same (race,id) -> same file', first && second && first.file === second.file);
check('coastal: file from its own pool', !!first && coastalPool.has(first.file), first && first.file);
check('coastal: never neutral', !!first && first.file !== 'neutral.png');
check('color from 9-color palette', !!first && typeof first.color === 'string' && first.color.startsWith('#'));

// different races -> different ships (coastal vs ammonia)
const coastalSet = new Set();
const ammoniaSet = new Set();
for (let i = 0; i < 200; i++) {
  coastalSet.add(spriteForAgent('coastal', 'a' + i).file);
  ammoniaSet.add(spriteForAgent('ammonia', 'a' + i).file);
}
check('coastal: multiple variants used', coastalSet.size >= 2, 'variants=' + coastalSet.size);
check('ammonia: only its own files', [...ammoniaSet].every(f => ammoniaPool.has(f)), [...ammoniaSet].join(','));
check('ammonia: never neutral (incomplete suffix set)', !ammoniaSet.has('neutral.png'));
check('different races -> disjoint files', [...coastalSet].every(f => !ammoniaSet.has(f)));

// humans -> only human files
let humansOk = true;
for (let i = 0; i < 200; i++) {
  const s = spriteForAgent('humans', 'h' + i);
  if (!s || !s.file.startsWith('race_humans_')) { humansOk = false; break; }
}
check('humans: only human ships', humansOk);

// --- neutral fallback (3 races without ships, unknown, empty) ---
for (const race of ['cryo_sky', 'geysers', 'ice_plankton', 'unknown_race', '']) {
  const s = spriteForAgent(race, 'x1');
  check('neutral fallback for race "' + race + '"', !!s && s.file === 'neutral.png', s && s.file);
}

// --- registry-growth stability (N8/И7): adding a variant changes only its slot ---
const before = new Map();
for (let i = 0; i < 400; i++) before.set('c' + i, spriteForAgent('coastal', 'c' + i).file);
setShipOptions(baseOptions.concat([{ id: 'race_coastal_04', file: 'race_coastal_04.png', race: 'coastal' }]));
let changed = 0;
let stillPool = true;
for (let i = 0; i < 400; i++) {
  const s = spriteForAgent('coastal', 'c' + i);
  if (!s || !(coastalPool.has(s.file) || s.file === 'race_coastal_04.png')) stillPool = false;
  if (s.file !== before.get('c' + i)) changed++;
}
check('growth: stays in race pool', stillPool);
check('growth: changes only the new slot (~1/9)', changed > 0 && changed <= 90, 'changed=' + changed);

// --- null when even the neutral pool is missing ---
setShipOptions([]);
check('null when registry empty (fallback diamond)', spriteForAgent('coastal', 'x') === null);

console.log('');
console.log('RESULT: ' + (fail === 0 ? 'PASS (' + ok + ')' : 'FAIL (' + fail + ' of ' + (ok + fail) + ')'));
process.exit(fail === 0 ? 0 : 1);

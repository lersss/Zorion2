// tools/e2e/race-ships-variant-check.js
// Node check for the race-ship visual logic (spec 2026-09-23-корабли-рас... §6.3,
// §6.4, N8): imports the real client module (web/static/js/map/ship_sprites.js)
// and exercises spriteForAgent(race_id, id) / setShipOptions without a browser
// (the module touches DOM only in getShipImage/recolorShipSprite, which are not
// called here). Verifies: determinism, different races -> different ships,
// races without a pool (cryo_sky/geysers/ice_plankton) and unknown/empty race ->
// neutral pool, incomplete suffix sets never fall to neutral, registry-growth
// stability (adding a variant changes only its slot) and deletion degradation
// (spec 2026-09-25-арт-студия-удаление... §6: removing a variant moves only the
// slots that relied on it, never to neutral while the pool is non-empty).
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

// --- deletion degradation (spec 2026-09-25-арт-студия-удаление... §6, И7) ---
// Deleting a variant does NOT move slots that still have an exact suffix; only
// the slots that relied on the removed suffix move (to the smallest survivor).
// While the race pool is non-empty an agent NEVER falls to the neutral ship.
function snapshotFiles(race, n) {
  const m = new Map();
  for (let i = 0; i < n; i++) m.set('d' + i, spriteForAgent(race, 'd' + i).file);
  return m;
}

// (a) remove a middle suffix from a full pool {_01,_02,_03}: only slot 2 moves
setShipOptions(baseOptions);
const coastalBefore = snapshotFiles('coastal', 400);
setShipOptions(baseOptions.filter(o => o.file !== 'race_coastal_02.png'));
let midExactKept = true, midMovedOk = true, midNoNeutral = true, midMoved = 0;
for (const [k, b] of coastalBefore) {
  const a = spriteForAgent('coastal', k).file;
  if (a === 'neutral.png') midNoNeutral = false;
  if (b === 'race_coastal_02.png') { midMoved++; if (a !== 'race_coastal_01.png') midMovedOk = false; }
  else if (a !== b) midExactKept = false;
}
check('deletion(middle suffix): exact slots unchanged', midExactKept);
check('deletion(middle suffix): removed-slot agents -> smallest survivor', midMoved > 0 && midMovedOk, 'moved=' + midMoved);
check('deletion(middle suffix): never neutral (non-empty pool)', midNoNeutral);

// (b) remove the smallest suffix of a sparse pool {_02,_03}: exact _03 stays,
// everyone who relied on _02 moves to the new smallest _03
setShipOptions(baseOptions);
const ammoniaBefore = snapshotFiles('ammonia', 400);
setShipOptions(baseOptions.filter(o => o.file !== 'race_ammonia_02.png'));
let smallExactKept = true, smallMovedOk = true, smallNoNeutral = true, smallMoved = 0;
for (const [k, b] of ammoniaBefore) {
  const a = spriteForAgent('ammonia', k).file;
  if (a === 'neutral.png') smallNoNeutral = false;
  if (b === 'race_ammonia_02.png') { smallMoved++; if (a !== 'race_ammonia_03.png') smallMovedOk = false; }
  else if (a !== b) smallExactKept = false;
}
check('deletion(smallest suffix): exact _03 slots unchanged', smallExactKept);
check('deletion(smallest suffix): slots relying on _02 move to new smallest', smallMoved > 0 && smallMovedOk, 'moved=' + smallMoved);
check('deletion(smallest suffix): never neutral (non-empty pool)', smallNoNeutral);

// --- null when even the neutral pool is missing ---
setShipOptions([]);
check('null when registry empty (fallback diamond)', spriteForAgent('coastal', 'x') === null);

console.log('');
console.log('RESULT: ' + (fail === 0 ? 'PASS (' + ok + ')' : 'FAIL (' + fail + ' of ' + (ok + fail) + ')'));
process.exit(fail === 0 ? 0 : 1);

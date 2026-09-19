// web/static/js/dashboard/races_section.js
// Раздел «Расы» энциклопедии (спека 86a §5.1): сетка 60 карточек. До встречи —
// силуэт «—» с плейсхолдером (решение гейта 1, §12: имя/числа/лор открываются
// только по действию); открытая карточка (раса в journal.metRaces; люди — по
// умолчанию, §5.1.3) — имя, бейдж семейства, основа, окна, атрибуты-слова,
// вид/ниша/размер, дом, корм + лор-блок (99.2.26 §2).

import { kelvinToCelsius } from '../modal/panel.js';

// FAMILY_LABELS — бейджи семейств (22_races.md §2.2/§4).
const FAMILY_LABELS = {
    F1: 'F1 · Водные', F2: 'F2 · Крио-аммиачные', F3: 'F3 · Метановые',
    F4: 'F4 · Серные', F5: 'F5 · Терморедокс', F6: 'F6 · Кремниевые',
    F7: 'F7 · Водородные/небесные', F8: 'F8 · Углекислые', F9: 'F9 · Экзотика',
    robotic: 'Роботы',
};

// ATTRIBUTE_LABELS — подписи атрибутов-слов (порядок карточки §3.2).
const ATTRIBUTE_LABELS = [
    ['aggression', 'Агрессия'],
    ['curiosity', 'Любопытство'],
    ['reproduction', 'Размножение'],
    ['intelligence', 'Интеллект'],
    ['diplomacy', 'Дипломатия'],
    ['resilience', 'Устойчивость'],
];

// formatRange — «lo–hi» (hi = null → «lo+»).
function formatRange(r) {
    if (!r) return '—';
    return r.hi === null || r.hi === undefined ? `${r.lo}+` : `${r.lo}–${r.hi}`;
}

// windowLine — окно по оси: «T: 0–37 °C (выж. −23…57 °C)».
function windowLine(label, w, unit, conv) {
    if (!w) return '';
    const fmt = v => (conv ? conv(v) : v);
    const opt = `${fmt(w.opt.lo)}–${fmt(w.opt.hi)}`;
    const surv = `${fmt(w.surv.lo)}–${fmt(w.surv.hi)}`;
    return `<div><strong>${label}:</strong> ${opt} ${unit} <span style="color:#94a3b8;">(выж. ${surv})</span></div>`;
}

// atmosphereLine — требования к атмосфере: «нужен O₂ ≥ 10%; яды: CO₂ ≤ 5%».
function atmosphereLine(atm) {
    if (!atm) return '';
    const need = Object.entries(atm.need || {}).map(([g, v]) => `${g} ≥ ${v}%`).join(', ');
    const poison = Object.entries(atm.poison || {}).map(([g, v]) => `${g} ≤ ${v}%`).join(', ');
    const parts = [];
    if (need) parts.push(`нужен: ${need}`);
    if (poison) parts.push(`яды: ${poison}`);
    return parts.length ? `<div><strong>Атмосфера:</strong> ${parts.join('; ')}</div>` : '';
}

// attributeWords — 6 строк атрибутов-слов (спека 99.2.26 §2 п.4): подпись оси
// + слово из race.lore.attributes_words; цифры и бары не показываются. При
// отсутствии attributes_words — строка «—» (деградация при незагруженном
// лоре, сервер живёт).
function attributeWords(lore) {
    const aw = (lore && lore.attributes_words) || {};
    return ATTRIBUTE_LABELS.map(([key, label]) => {
        const word = aw[key] || '—';
        return `
            <div style="display:flex; justify-content:space-between; margin-top:4px; font-size:0.8rem;">
                <span style="color:#94a3b8;">${label}</span><span style="color:#e2e8f0;">${word}</span>
            </div>`;
    }).join('');
}

// openedCard — полная карточка встреченной расы (§5.1.2).
function openedCard(race) {
    const c = race.conditions || {};
    const lore = race.lore || {};
    const home = race.home || {};
    const robotic = race.robotic;
    const windows = [
        windowLine('T', c.temperature, '°C', kelvinToCelsius),
        windowLine('P', c.pressure, 'атм'),
        windowLine('g', c.gravity, 'g'),
        windowLine('Радиация', c.radiation, ''),
        atmosphereLine(c.atmosphere),
    ].join('');

    let roboticBlock = '';
    if (robotic) {
        const mats = robotic.materials || {};
        roboticBlock = `
            <div style="margin-top:8px; padding-top:8px; border-top:1px solid #334155;">
                <div><strong>Энергия:</strong> ${(robotic.power_source || []).join(', ')}</div>
                <div><strong>Сброс тепла:</strong> ${robotic.heat || '—'}</div>
                <div><strong>Сырьё:</strong> ${(mats.categories || []).join(' ')}</div>
            </div>`;
    }

    // «Вид · Ниша · Размер (+ Рой/стая)» (спека 99.2.26 §2 п.5): kind · niche ·
    // size_individual; size_group выводится, только если не null.
    const sizeParts = [lore.kind, lore.niche, lore.size_individual];
    if (lore.size_group) sizeParts.push(lore.size_group);
    const kindNicheSize = `
        <div style="font-size:0.85rem; margin-top:8px; color:#e2e8f0;">
            <div><strong>Вид · Ниша · Размер:</strong> ${sizeParts.filter(Boolean).join(' · ')}</div>
        </div>`;

    // «Дом»: home_words словами; фолбэк на коды home.star_classes +
    // home.planet_niche, если home_words нет (спека 99.2.26 §2 п.6).
    const homeLine = lore.home_words || `${(home.star_classes || []).join(', ')} · ${home.planet_niche || '—'}`;

    // Лор-блок (спека 99.2.26 §2 п.9): lore абзацами (\n\n → блоки) вместо
    // character/how_live/why; coexistence и origin (роботы) остаются.
    const loreBlock = `
        <div style="margin-top:8px; padding-top:8px; border-top:1px solid #334155; font-size:0.85rem; color:#cbd5e1;">
            ${lore.lore ? lore.lore.split('\n\n').map(p => `<div style="margin-top:4px;">${p}</div>`).join('') : ''}
            ${lore.coexistence ? `<div style="margin-top:4px;"><strong>Сосуществование:</strong> ${lore.coexistence}</div>` : ''}
            ${lore.origin ? `<div style="margin-top:4px;"><strong>Происхождение:</strong> ${lore.origin}</div>` : ''}
        </div>`;

    return `
        <div style="background:#1e293b; border:1px solid #334155; border-radius:12px; padding:12px; display:flex; flex-direction:column;">
            <div style="display:flex; justify-content:space-between; align-items:center; gap:8px;">
                <strong style="font-size:1.05rem;">${race.name || race.id}</strong>
                <span style="font-size:0.7rem; background:#334155; color:#cbd5e1; border-radius:10px; padding:2px 8px; white-space:nowrap;">${FAMILY_LABELS[race.family] || race.family || ''}</span>
            </div>
            <div style="font-size:0.8rem; color:#94a3b8; margin-top:2px;">${race.basis || ''}</div>
            <div style="font-size:0.85rem; margin-top:8px; color:#e2e8f0;">${windows}</div>
            <div style="margin-top:8px;">${attributeWords(lore)}</div>
            ${kindNicheSize}
            <div style="font-size:0.85rem; margin-top:8px; color:#e2e8f0;">
                <div><strong>Дом:</strong> ${homeLine}</div>
                ${race.forage && race.forage.source ? `<div><strong>Корм:</strong> ${race.forage.source}</div>` : ''}
            </div>
            ${roboticBlock}
            ${loreBlock}
        </div>`;
}

// silhouetteCard — силуэт «—» до встречи (§5.1.2, решение гейта 1).
function silhouetteCard() {
    return `
        <div style="background:#1e293b; border:1px dashed #334155; border-radius:12px; padding:12px; display:flex; flex-direction:column; align-items:center; justify-content:center; min-height:120px; color:#64748b;">
            <div style="font-size:1.6rem; letter-spacing:4px;">———</div>
            <div style="font-size:0.9rem; margin-top:6px;">???</div>
            <div style="font-size:0.75rem; margin-top:4px; text-align:center;">Раса не встречалась — посетите её поселение</div>
        </div>`;
}

// renderRacesSection — сетка 60 карточек рас.
// racesData — массив из /api/encyclopedia/races; journal — журнал (get()).
export function renderRacesSection(container, racesData, journal) {
    const met = new Set(journal.metRaces || []);
    met.add('humans'); // люди открыты по умолчанию (§5.1.3)
    const cards = (racesData || []).map(race =>
        met.has(race.id) ? openedCard(race) : silhouetteCard()
    );
    container.innerHTML = `
        <div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(240px, 1fr)); gap:10px;">
            ${cards.join('')}
        </div>`;
}
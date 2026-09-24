// web/static/js/modal/structures.js
// Вкладка «Строения» карточки планеты (спека 2026-09-24-постройка-структур
// §10.1, решение Р14): список ВСЕХ buildings планеты, включая столицы, без
// фильтра по «живости». Имя — type_name (у столицы пусто → buildingTypeLabel),
// владелец — owner_name с цветной точкой (best-effort цвет из planet.factions),
// пометки: столица (producer_type_id IS NULL) — «не производит», строение с
// типом без рантайм-производства — «производство ещё не реализовано». Клик по
// строке — инлайновый тоггл деталей (как у столицы во «Фракциях»).
//
// Чистые функции без DOM/сети на верхнем уровне: модуль тянется в import-граф
// админки (modal/tabs.js → panel.js → index.js) и исполняется в Node
// (web/frontend_test.go, web/frontend_knowledge_tabs_test.go).
import { modalState } from './state.js';
import { escapeHtml, safeCssColor } from './contracts.js';

// isAdmin — роль из /me (как tabs.js isAdmin): админ видит вкладку всегда.
function isAdmin() {
    return modalState.role === 'admin' || modalState.role === 'skycomposer';
}

// structuresTabVisible — видимость вкладки «Строения» (§10.1): админ — всегда;
// игроку — когда блок buildings пришёл. В режиме none сервер обнуляет buildings
// → вкладки нет.
export function structuresTabVisible(planet) {
    if (isAdmin()) return true;
    return !!(planet && planet.buildings != null);
}

// BUILDING_TYPE_LABELS — словарь подписей типов строений: единственное место,
// где ключ (buildings.building_type) превращается в человекочитаемое имя.
// Неизвестный ключ показывается как есть (выдуманных имён не вводим, §6).
const BUILDING_TYPE_LABELS = {
    capital: 'Столица'
};

// buildingTypeLabel — подпись типа строения по ключу. Живёт здесь (тип —
// атрибут строения); tabs.js импортирует её для строки столицы во «Фракциях».
export function buildingTypeLabel(type) {
    if (!type) return '—';
    return BUILDING_TYPE_LABELS[type] || type;
}

// ownerDotColor — цвет точки владельца (§10.1): best-effort из planet.factions
// по owner_id для владельца-фракции; иначе нейтральный серый.
function ownerDotColor(planet, b) {
    if (b && b.owner_type === 'faction' && planet && Array.isArray(planet.factions)) {
        const f = planet.factions.find(x => x.id === b.owner_id);
        if (f && f.color) return f.color;
    }
    return '#64748b';
}

// structureChipHtml — пометка класса строения (§10.1): столица (нет
// producer_type_id) — «не производит»; иначе — «производство ещё не
// реализовано» (механики производства строений пока нет).
function structureChipHtml(b) {
    const neutral = b.producer_type_id == null;
    const text = neutral ? 'не производит' : 'производство ещё не реализовано';
    const color = neutral ? '#64748b' : '#facc15';
    return `<span style="border:1px solid ${color}; color:${color}; background:rgba(15,23,42,0.72); border-radius:10px; padding:1px 8px; font-size:0.72rem;">${text}</span>`;
}

// structureRowHtml — строка строения: имя типа, владелец, пометка класса и
// скрытый блок деталей (инлайновый тоггл).
function structureRowHtml(planet, b) {
    const name = b.type_name || buildingTypeLabel(b.building_type);
    const owner = b.owner_name || 'NPC (без владельца)';
    const color = ownerDotColor(planet, b);
    const producerLine = b.producer_type_id != null
        ? ` · тип производителя #${escapeHtml(b.producer_type_id)}`
        : ' — не производит';
    return `
        <div style="margin:6px 0; padding:10px; background:#1a1a2e; border-radius:4px;">
            <div data-building-toggle="${escapeHtml(b.id)}" style="cursor:pointer;">
                <div><span style="display:inline-block; width:10px; height:10px; border-radius:50%; background:${safeCssColor(color)}; margin-right:6px;"></span><strong>${escapeHtml(name)}</strong> ${structureChipHtml(b)}</div>
                <div style="color:#ccc; margin-top:4px;">Владелец: ${escapeHtml(owner)}</div>
                <div style="color:#94a3b8; font-size:0.8rem; margin-top:4px;">▸ нажмите, чтобы раскрыть</div>
            </div>
            <div data-building-details="${escapeHtml(b.id)}" style="display:none; margin-top:8px; padding-top:8px; border-top:1px solid #2a2a4a;">
                <div>Владелец: ${escapeHtml(owner)}</div>
                <div>Тип: ${escapeHtml(b.building_type || '—')}${producerLine}</div>
            </div>
        </div>`;
}

// renderStructures — вкладка «Строения» (§10.1): все buildings планеты. Пусто —
// «Строений нет».
export function renderStructures(planet) {
    const list = Array.isArray(planet && planet.buildings) ? planet.buildings : [];
    if (list.length === 0) {
        return `<p style="color: #666; text-align: center; padding: 20px 0;">Строений нет</p>`;
    }
    let html = `<p style="color:#888; font-size:0.9rem; text-transform:uppercase;">Строения (${list.length})</p>`;
    list.forEach(b => { html += structureRowHtml(planet, b); });
    return html;
}

// initStructures — вешает инлайновый тоггл деталей строки (как у столицы во
// «Фракциях»: повторный клик сворачивает, серверных вызовов нет).
export function initStructures(planet, container) {
    container.querySelectorAll('[data-building-toggle]').forEach(el => {
        el.addEventListener('click', () => {
            const details = container.querySelector(`[data-building-details="${el.dataset.buildingToggle}"]`);
            if (details) details.style.display = details.style.display === 'none' ? 'block' : 'none';
        });
    });
}

// web/static/js/modal/contracts.js
// Доска контрактов планеты для карточки планеты (спека 2026-09-22-контракт-
// перелёт-и-доска §2.1/§2.2/§3): строка контракта — тип, заголовок, цена,
// «осталось N», требования читаемо, пометка автора. Публикация — только с
// планеты, где стоит игрок (орбита/поверхность; на орбите звезды нельзя).
//
// Чистые функции без DOM/сети на верхнем уровне: модуль тянется в import-граф
// админки (modal/tabs.js) и исполняется в Node (web/frontend_contracts_test.go).

// escapeHtml — экранирование строк, приходящих с сервера, перед вставкой в
// innerHTML. Заголовок/описание контракта задаёт ДРУГОЙ игрок при публикации,
// сервер их не чистит → без экранирования открытие доски исполнило бы чужой
// скрипт (stored XSS, многопользовательский проект). Локальная копия: единого
// общего модуля в проекте нет (такие же локальные копии — search.js:196,
// admin/tests.js:255); вынос в общий модуль — отдельная правка, здесь не
// трогаем чужие файлы. Экспортируется — tabs.js экранирует им имена систем.
export function escapeHtml(s) {
    return String(s == null ? '' : s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

// CONTRACT_TYPE_LABELS — словарь подписей типов контракта: единственное место,
// где ключ (contracts.type) превращается в человекочитаемое имя. Неизвестный
// ключ показывается как есть (выдуманных имён не вводим).
const CONTRACT_TYPE_LABELS = {
    travel: 'Перелёт'
};

// AUTHOR_TYPE_LABELS — подписи авторов (contracts.author_type, §2.1).
const AUTHOR_TYPE_LABELS = {
    player: 'игрок',
    faction: 'фракция',
    building: 'постройка',
    agent: 'агент'
};

// AUTHOR_TYPE_ICONS — иконки авторов (§2.1: «иконка/подпись»).
const AUTHOR_TYPE_ICONS = {
    player: '👤',
    faction: '🏛',
    building: '🏗',
    agent: '🤖'
};

// contractTypeLabel — подпись типа контракта по ключу.
export function contractTypeLabel(type) {
    if (!type) return '—';
    return CONTRACT_TYPE_LABELS[type] || type;
}

// authorLabel — подпись автора по ключу (неизвестный — как есть).
export function authorLabel(authorType) {
    if (!authorType) return '—';
    return AUTHOR_TYPE_LABELS[authorType] || authorType;
}

// authorIcon — иконка автора по ключу (неизвестный — нейтральная).
export function authorIcon(authorType) {
    return AUTHOR_TYPE_ICONS[authorType] || '•';
}

// requirementText — требование читаемо (§2.1: «двигатель не хуже …»).
// kind='gear', subject='speed_factor' — единственное требование итерации 1
// (спека перелёта §1.3, op='le'). Неизвестное требование показывается как есть.
// threshold_text приходит с сервера — экранируется (stored XSS).
export function requirementText(req) {
    if (!req) return '';
    if (req.kind === 'gear' && req.subject === 'speed_factor') {
        // op='le' — «не хуже» (меньше — быстрее, спека §1.3). op='ge' в
        // итерации 1 не встречается; формулировка для него — заглушка
        // «не медленнее» (калибровать вместе с типами требований).
        const op = req.op === 'le' ? 'не хуже' : req.op === 'ge' ? 'не медленнее' : '';
        const val = req.threshold_num != null ? req.threshold_num : '—';
        return op ? `двигатель ${op} ${val}` : `двигатель ${val}`;
    }
    if (req.threshold_text) return escapeHtml(req.threshold_text);
    return escapeHtml([req.kind, req.subject, req.op, req.threshold_num].filter(v => v != null && v !== '').join(' '));
}

// timeLeftText — «осталось N» от expires_at (§2.1). now — миллисекунды.
// Истёк/нет даты → пусто (истёкшие на доске не показываются).
export function timeLeftText(expiresAt, now) {
    if (!expiresAt) return '';
    const end = Date.parse(expiresAt);
    if (isNaN(end)) return '';
    const ms = end - now;
    if (ms <= 0) return '';
    const min = Math.floor(ms / 60000);
    if (min < 60) return `осталось ${min} мин`;
    const hours = Math.floor(min / 60);
    if (hours < 24) return `осталось ${hours} ч`;
    const days = Math.floor(hours / 24);
    return `осталось ${days} дн`;
}

// contractRowHtml — строка контракта на доске (§2.1): тип, заголовок, цена,
// «осталось N», требования, пометка автора. Кнопка «Взять» — data-атрибут,
// обработчик вешает initContracts (tabs.js). Балансы не показываются (§2.2).
// Все строки с сервера (title/description/type/id) экранируются — stored XSS.
export function contractRowHtml(contract, now) {
    if (!contract) return '';
    const type = escapeHtml(contractTypeLabel(contract.type));
    const left = timeLeftText(contract.expires_at, now);
    const reqs = Array.isArray(contract.requirements) ? contract.requirements : [];
    const reqHtml = reqs.length
        ? `<div style="color:#94a3b8; font-size:0.85rem; margin-top:4px;">Требования: ${reqs.map(requirementText).join(', ')}</div>`
        : '';
    const desc = contract.description
        ? `<div style="color:#888; font-size:0.85rem; margin-top:4px;">${escapeHtml(contract.description)}</div>`
        : '';
    return `
        <div style="margin:6px 0; padding:10px; background:#1a1a2e; border-radius:4px;">
            <div style="display:flex; justify-content:space-between; align-items:flex-start; gap:8px;">
                <div>
                    <div><strong>${escapeHtml(contract.title) || '—'}</strong> <span style="color:#888; font-size:0.85rem;">${type}</span></div>
                    <div style="color:#fde68a; margin-top:4px;">${contract.reward != null ? contract.reward : '—'} кр.</div>
                    ${left ? `<div style="color:#94a3b8; font-size:0.85rem; margin-top:4px;">${left}</div>` : ''}
                    ${reqHtml}
                    ${desc}
                    <div style="color:#888; font-size:0.85rem; margin-top:4px;">${authorIcon(contract.author_type)} ${escapeHtml(authorLabel(contract.author_type))}</div>
                </div>
                <button data-contract-take="${escapeHtml(contract.id)}" style="background:#2a2a4a; border:none; color:#ddd; padding:6px 14px; border-radius:4px; cursor:pointer; white-space:nowrap;">Взять</button>
            </div>
        </div>`;
}

// boardHtml — доска планеты (§2.1): живые публичные контракты. Пусто — «нет
// контрактов». Взятые/закрытые и прямые сервер на доску не отдаёт (§2.1).
export function boardHtml(contracts, now) {
    const list = Array.isArray(contracts) ? contracts : [];
    if (list.length === 0) {
        return `<p style="color:#666; text-align:center; padding:12px 0;">Контрактов нет</p>`;
    }
    let html = `<p style="color:#888; font-size:0.9rem; text-transform:uppercase;">Контракты (${list.length})</p>`;
    list.forEach(c => { html += contractRowHtml(c, now); });
    return html;
}

// canPublishHere — публикация доступна только с планеты, где стоит игрок
// (орбита/поверхность; §3, решение О-п1). На орбите звезды и в полёте — нет.
export function canPublishHere(myPosition, planetId) {
    if (!myPosition || !planetId) return false;
    if (myPosition.status === 'in_flight') return false;
    return myPosition.object_type === 'planet' && myPosition.object_id === planetId;
}

// publishFormHtml — форма «Опубликовать» (§3): тип «перелёт», заголовок, цена,
// система-назначение (опц. планета-назначение). from_world_id подставляется
// текущей системой игрока (modalState.worldId) при отправке. Список систем
// наполняет initContracts (GET /worlds).
export function publishFormHtml() {
    return `
        <div style="margin-top:16px; padding-top:10px; border-top:1px solid #2a2a4a;">
            <div style="color:#888; font-size:0.9rem; text-transform:uppercase;">Опубликовать контракт</div>
            <div style="display:flex; flex-wrap:wrap; gap:6px; align-items:center; margin-top:6px;">
                <input data-contract-title type="text" placeholder="заголовок"
                    style="flex:1; min-width:160px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                <input data-contract-reward type="number" min="1" step="1" placeholder="цена, кр."
                    style="width:110px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
            </div>
            <div style="display:flex; flex-wrap:wrap; gap:6px; align-items:center; margin-top:6px;">
                <select data-contract-dest-world style="flex:1; min-width:160px; background:#1a1a2e; color:#ddd; border:1px solid #2a2a4a; border-radius:4px; padding:4px 6px;">
                    <option value="">система-назначение…</option>
                </select>
                <button data-contract-publish style="background:#2a2a4a; border:none; color:#ddd; padding:6px 14px; border-radius:4px; cursor:pointer;">Опубликовать</button>
            </div>
            <div data-contract-publish-status style="font-size:0.85rem; margin-top:6px; color:#94a3b8;"></div>
        </div>`;
}

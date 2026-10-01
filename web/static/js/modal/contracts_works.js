// web/static/js/modal/contracts_works.js
// Блок «Мои заказы (в работе)» и сдача груза (спека
// 2026-09-25-сдача-груза-и-зачёт-ЧК2б §6). Вынесен из contracts.js: рендер
// взятых заказов и тексты сдачи не относятся к доске, а contracts.js иначе
// переваливал за 300 строк. Чистые функции без DOM/сети — исполняются в Node
// (web/frontend_contracts_test.go).
//
// Идея 2026-10-01_сдача-груза-не-по-роли ЧК2/ЧК3: строка заказа показывает
// название товара (good_name с сервера, внутренний subject не показываем) и
// «осталось N» рядом с «в трюме M» (GET /api/cargo) — игрок до сдачи видит,
// хватит ли груза. Числа, которых нет, не выводятся (молчаливое «0» врало бы).

import { escapeHtml, contractTypeLabel, authorLabel, authorIcon, timeLeftText } from './contracts.js';

// cargoByGoodId — карта «good_id → количество» из GET /api/cargo
// ({items:[{good_id, quantity}]}). Строки без числовых good_id/quantity
// пропускаем: показывать «в трюме» по мусорным данным нельзя.
export function cargoByGoodId(items) {
    const map = new Map();
    const list = Array.isArray(items) ? items : [];
    list.forEach(it => {
        if (!it) return;
        // null/undefined/'' и не-число отбрасываем: Number(null) === 0, и без
        // этой проверки «в трюме 0 ед.» напечаталось бы по мусорной строке.
        if (it.good_id == null || it.quantity == null) return;
        const id = Number(it.good_id);
        const qty = Number(it.quantity);
        if (!Number.isFinite(id) || !Number.isFinite(qty)) return;
        map.set(id, qty);
    });
    return map;
}

// goodsQtyText — количество единиц текстом: целое без дробной части, дробное с
// двумя знаками (груз в трюме бывает дробным).
function goodsQtyText(qty) {
    const n = Number(qty);
    if (!Number.isFinite(n)) return '';
    return String(Math.round(n * 100) / 100);
}

// goodsLineHtml — строка товара заказа: «Пища · осталось 500 ед. · в трюме 12 ед.».
// cargo — карта cargoByGoodId; cargo === null (груз не читали) → «в трюме» не
// выводим вовсе. Название экранируется: good_name приходит из каталога товаров.
export function goodsLineHtml(req, cargo) {
    if (!req) return '';
    const name = escapeHtml(req.good_name || 'товар без названия');
    const parts = [];
    if (req.quantity != null) parts.push('осталось ' + goodsQtyText(req.quantity) + ' ед.');
    if (cargo) {
        const qty = cargo.get(Number(req.subject));
        if (qty != null) parts.push('в трюме ' + goodsQtyText(qty) + ' ед.');
    }
    return parts.length ? name + ' · ' + parts.join(' · ') : name;
}

// remainingUnits — остаток требования supply-заказа: quantity первого
// требования kind='goods' op='in' (§5.2). null — данных нет (тогда не показываем).
export function remainingUnits(contract) {
    const reqs = contract && Array.isArray(contract.requirements) ? contract.requirements : [];
    const req = reqs.find(r => r && r.kind === 'goods' && r.op === 'in');
    return req && req.quantity != null ? req.quantity : null;
}

// workRowHtml — строка «Мой заказ (в работе)» (ЧК2б §6): товар · осталось N ед.
// · в трюме M ед. · «осталось времени», автор и кнопка «Сдать».
// Обработчик вешает loadContracts (tabs.js). Все строки с сервера экранируются
// (stored XSS, как в contractRowHtml).
export function workRowHtml(contract, now, cargo) {
    if (!contract) return '';
    const reqs = Array.isArray(contract.requirements) ? contract.requirements : [];
    const req = reqs.find(r => r && r.kind === 'goods' && r.op === 'in');
    const goodsLine = req
        ? `<div style="color:#94a3b8; font-size:0.85rem; margin-top:4px;">${goodsLineHtml(req, cargo)}</div>`
        : '';
    const left = timeLeftText(contract.expires_at, now);
    return `
        <div style="margin:6px 0; padding:10px; background:#1a1a2e; border-radius:4px;">
            <div style="display:flex; justify-content:space-between; align-items:flex-start; gap:8px;">
                <div>
                    <div><strong>${escapeHtml(contract.title) || '—'}</strong> <span style="color:#888; font-size:0.85rem;">${escapeHtml(contractTypeLabel(contract.type))}</span></div>
                    ${goodsLine}
                    ${left ? `<div style="color:#94a3b8; font-size:0.85rem; margin-top:4px;">${left}</div>` : ''}
                    <div style="color:#888; font-size:0.85rem; margin-top:4px;">${authorIcon(contract.author_type)} ${escapeHtml(authorLabel(contract.author_type))}</div>
                </div>
                <button data-contract-deliver="${escapeHtml(contract.id)}" style="background:#2a4a2a; border:none; color:#ddd; padding:6px 14px; border-radius:4px; cursor:pointer; white-space:nowrap;">Сдать</button>
            </div>
        </div>`;
}

// myWorksHtml — подблок «Мои заказы (в работе)» (ЧК2б §6): мои взятые
// supply-заказы. Источник — GET /api/contracts/mine: сервер отдаёт контракты,
// где я автор ИЛИ исполнитель (только с player-автором/исполнителем и моим id,
// ListMine §2.2). Фильтр «type='supply' && status='taken' &&
// executor_type='player'» равносилен «исполнитель — я» (чужой player-исполнитель
// в mine не попадает) — id игрока на клиенте не нужен. Пусто — блок не рисуем.
// cargo — карта cargoByGoodId (груз прочитан tabs.js) или null.
export function myWorksHtml(contracts, now, cargo) {
    const list = Array.isArray(contracts) ? contracts : [];
    const works = list.filter(c => c && c.type === 'supply' && c.status === 'taken' && c.executor_type === 'player');
    if (works.length === 0) return '';
    let html = `<div data-my-works style="margin-top:16px; padding-top:10px; border-top:1px solid #2a2a4a;">
        <div style="color:#888; font-size:0.9rem; text-transform:uppercase;">Мои заказы (в работе) · ${works.length}</div>`;
    works.forEach(c => { html += workRowHtml(c, now, cargo); });
    return html + `</div>`;
}

// deliverResultText — человеческий текст успешной сдачи (ЧК2б §6): «сдано N,
// остаток требования M»; полное закрытие — «заказ выполнен» (status='completed'
// либо остаток 0).
export function deliverResultText(data) {
    const d = data || {};
    const delivered = d.delivered != null ? d.delivered : 0;
    const remaining = d.remaining != null ? d.remaining : 0;
    const paid = d.paid != null ? d.paid : 0;
    const pay = paid > 0 ? `, выплачено ${paid} кр.` : '';
    if (d.status === 'completed' || remaining <= 0) {
        return `Заказ выполнен: сдано ${delivered} ед.${pay}`;
    }
    return `Сдано ${delivered} ед., остаток требования ${remaining} ед.${pay}`;
}

// deliverErrorText — человеческий текст отказа сдачи (ЧК2б §6). Сервер шлёт
// {error:"…"} с уже читаемым текстом (например «В трюме нет товара «Вода», а по
// заказу осталось сдать 40 ед.»); при неразобранном теле — подстраховка по
// HTTP-коду (409/422/403), чтобы игрок не увидел сырой JSON.
export function deliverErrorText(body, status) {
    let msg = '';
    if (body && typeof body === 'object' && body.error) msg = String(body.error);
    else if (typeof body === 'string' && body.trim()) msg = body.trim();
    if (msg) return msg;
    if (status === 409) return 'Заказ уже не взят или хранилище недоступно';
    if (status === 422) return 'Сдать нельзя: проверьте требование заказа';
    if (status === 403) return 'Вы не исполнитель этого заказа';
    return 'Не удалось сдать груз';
}
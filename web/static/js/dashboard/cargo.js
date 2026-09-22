// web/static/js/dashboard/cargo.js
// Блок «Трюм» раздела «Корабль» дашборда (спека
// 2026-09-22-трюм-грузоподъёмность-корабля §9.1/§9.3).
//
// Данные — единственный источник GET /api/cargo (§9.1): лимиты по осям
// (limits.mass.used/total) и строки груза (good_id/name/kind/quantity/weight/
// mass). used/total/mass считает СЕРВЕР (И2) — клиент вес не перемножает.
// Сброс — POST /api/cargo/jettison (§7.1): по строке (good_id + количество)
// и «выбросить всё» ({all:true}); только уменьшает.
//
// Имена товаров приходят из каталога студии — экранируются (stored XSS):
// разметку строк собирает escapeHtml. Модуль Node-безопасен: DOM/localStorage/
// fetch трогаются только внутри initCargo (тест — web/frontend_cargo_test.go).

// escapeHtml — экранирование текста для вставки в разметку (innerHTML).
export function escapeHtml(s) {
    return String(s == null ? '' : s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

// cargoNum — число без лишних нулей: целое как есть, дробное — до 2 знаков.
// Нечисло — «—» (битые данные не рисуем как NaN).
export function cargoNum(n) {
    if (typeof n !== 'number' || !isFinite(n)) return '—';
    return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100);
}

// cargoMassLabel — человекочитаемый текст полосы: «занято / всего т».
export function cargoMassLabel(used, total) {
    return cargoNum(used) + ' / ' + cargoNum(total) + ' т';
}

// cargoPercent — ширина полосы (0..100): clamped, чтобы used > total не
// вылезал за границы (инвариант used ≤ total держит сервер).
export function cargoPercent(used, total) {
    if (typeof total !== 'number' || total <= 0) return 0;
    const p = (used / total) * 100;
    if (!isFinite(p)) return 0;
    return Math.max(0, Math.min(100, p));
}

// cargoEmptyHtml — пустое состояние.
export function cargoEmptyHtml() {
    return '<div class="cargo-empty">Трюм пуст.</div>';
}

// cargoRowHtml — строка «товар — количество — масса» + поле и кнопка сброса.
export function cargoRowHtml(it) {
    const id = escapeHtml(it && it.good_id);
    const name = escapeHtml(it && it.name);
    const qty = cargoNum(it && it.quantity);
    const mass = cargoNum(it && it.mass);
    return '<div class="cargo-row">'
        + '<div class="cargo-row-info">'
        + '<span class="cargo-row-name">' + name + '</span>'
        + '<span class="cargo-row-meta">' + qty + ' ед. · ' + mass + ' т</span>'
        + '</div>'
        + '<div class="cargo-row-act">'
        + '<input class="cargo-qty" type="number" min="0" step="any" value="' + qty + '" aria-label="Сколько выбросить">'
        + '<button class="btn-secondary cargo-jettison" type="button" data-good-id="' + id + '">Выбросить</button>'
        + '</div>'
        + '</div>';
}

// cargoItemsHtml — список строк; пусто/не-массив → пустое состояние.
export function cargoItemsHtml(items) {
    if (!Array.isArray(items) || items.length === 0) return cargoEmptyHtml();
    return items.map(cargoRowHtml).join('');
}

// initCargo — связывает блок с API: первичная загрузка, сброс по строке и
// «выбросить всё», перерисовка после сброса. DOM берётся лениво (Node-безопасно).
export function initCargo() {
    const doc = globalThis.document;
    if (!doc) return;
    const itemsEl = doc.getElementById('cargo-items');
    if (!itemsEl) return; // блока нет на этой странице

    const barFill = doc.getElementById('cargoBarFill');
    const barLabel = doc.getElementById('cargoBarLabel');
    const statusEl = doc.getElementById('cargo-status');
    const allBtn = doc.getElementById('cargoJettisonAllBtn');

    const token = () => {
        const ls = globalThis.localStorage;
        return ls ? ls.getItem('token') : null;
    };
    const setStatus = (text) => { if (statusEl) statusEl.textContent = text; };
    const redirectToLogin = () => {
        if (globalThis.location) globalThis.location.href = '/login-page';
    };

    // render — перерисовка из ответа §9.1 (used/total/mass — с сервера, И2).
    function render(data) {
        const mass = (data && data.limits && data.limits.mass) || {};
        const items = (data && data.items) || [];
        itemsEl.innerHTML = cargoItemsHtml(items);
        if (barFill) barFill.style.width = cargoPercent(mass.used, mass.total) + '%';
        if (barLabel) barLabel.textContent = cargoMassLabel(mass.used, mass.total);
        if (allBtn) allBtn.hidden = !(Array.isArray(items) && items.length > 0);
    }

    async function load() {
        const t = token();
        if (!t) return;
        try {
            const res = await fetch('/api/cargo', { headers: { 'Authorization': 'Bearer ' + t } });
            if (res.status === 401 || res.status === 403) { redirectToLogin(); return; }
            if (!res.ok) throw new Error('Не удалось загрузить трюм');
            render(await res.json());
            setStatus('');
        } catch (e) {
            setStatus('❌ ' + e.message);
            itemsEl.innerHTML = '<div class="cargo-empty">Не удалось загрузить трюм.</div>';
        }
    }

    // jettison — единственная player-facing запись (§7.1): POST, ответ — трюм.
    async function jettison(body) {
        const t = token();
        if (!t) { redirectToLogin(); return; }
        setStatus('⏳ Сброс...');
        try {
            const res = await fetch('/api/cargo/jettison', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + t },
                body: JSON.stringify(body),
            });
            if (res.status === 401 || res.status === 403) { redirectToLogin(); return; }
            if (!res.ok) {
                let msg = 'Не удалось сбросить груз';
                try { const d = await res.json(); if (d && d.error) msg = d.error; } catch (_) { /* тело не JSON */ }
                throw new Error(msg);
            }
            render(await res.json());
            setStatus('✅ Груз сброшен за борт');
        } catch (e) {
            setStatus('❌ ' + e.message);
        }
    }

    itemsEl.addEventListener('click', (e) => {
        const target = e.target;
        const btn = target && target.closest ? target.closest('.cargo-jettison') : null;
        if (!btn) return;
        const row = btn.closest('.cargo-row');
        const input = row ? row.querySelector('.cargo-qty') : null;
        const qty = input ? parseFloat(input.value) : NaN;
        if (!isFinite(qty) || qty <= 0) { setStatus('❌ Укажите количество больше 0'); return; }
        if (!globalThis.confirm('Выбросить ' + qty + ' за борт? Груз исчезнет навсегда.')) return;
        jettison({ good_id: Number(btn.dataset.goodId), quantity: qty });
    });

    if (allBtn) {
        allBtn.addEventListener('click', () => {
            if (!globalThis.confirm('Выбросить ВЕСЬ груз за борт? Он исчезнет навсегда.')) return;
            jettison({ all: true });
        });
    }

    load();
}

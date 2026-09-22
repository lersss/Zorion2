// web/static/js/modal/deposits.js
// Группировка залежей поверхности по ресурсу для карточки планеты (спека
// 2026-09-22-поселение-добыча-сырья-биома-ленивый-буфер §5.2/T13): сервер
// отдаёт залежи построчно (deposits[]), клиент сводит пятна одного ресурса —
// имя ресурса, число пятен, суммарный запас, диапазон богатства.
//
// Чистая функция без DOM/сети: тянется в import-граф админки (modal/tabs.js)
// и исполняется в Node (web/frontend_deposits_test.go).

// groupDeposits — [{good_id, good_name, stratum, wealth, amount}] →
// [{good_id, good_name, count, depleted, amount, wealthMin, wealthMax}].
// Порядок групп — по первому появлению ресурса в ответе. Некорректные/пустые
// элементы пропускаются; wealthMin/Max = null, если у пятен нет числа.
// depleted — число выработанных пятен (amount <= 0; спека итерации 3 §6/п.26):
// игрок их не получает (сервер фильтрует), админ видит с пометкой «выработано».
export function groupDeposits(deposits) {
    const groups = [];
    const byKey = new Map();
    (deposits || []).forEach(d => {
        if (!d) return;
        const key = d.good_id != null ? 'id:' + d.good_id : 'name:' + (d.good_name || '');
        let g = byKey.get(key);
        if (!g) {
            g = {
                good_id: d.good_id,
                good_name: d.good_name || '',
                count: 0,
                depleted: 0,
                amount: 0,
                wealthMin: null,
                wealthMax: null
            };
            byKey.set(key, g);
            groups.push(g);
        }
        g.count += 1;
        if (typeof d.amount === 'number') {
            g.amount += d.amount;
            if (d.amount <= 0) g.depleted += 1;
        }
        if (typeof d.wealth === 'number') {
            if (g.wealthMin === null || d.wealth < g.wealthMin) g.wealthMin = d.wealth;
            if (g.wealthMax === null || d.wealth > g.wealthMax) g.wealthMax = d.wealth;
        }
    });
    return groups;
}

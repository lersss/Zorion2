// utils.js
export function hashStringToNumber(str) {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
        const char = str.charCodeAt(i);
        hash = ((hash << 5) - hash) + char;
        hash |= 0;
    }
    return Math.abs(hash);
}

export function getClimateId(planet) {
    const type = (planet.type || '').toLowerCase();
    const temp = planet.temperature || 0;
    const water = planet.water_percent || 0;

    if (type.includes('вулканическая') || type.includes('лавовая')) return 'экстремальный';
    if (type.includes('пустынная') && temp > 300) return 'жаркий';
    if (type.includes('ледяная') || temp < 200) return 'холодный';
    if (type.includes('океаническая') || water > 60) return 'умеренный';
    if (temp > 350) return 'жаркий';
    if (temp > 200) return 'умеренный';
    if (temp > 100) return 'холодный';
    return 'изменчивый';
}

export function getStarColor(spectralClass, starType) {
    const colors = {
        'O': '#9bb0ff', 'B': '#aabfff', 'A': '#cad7ff',
        'F': '#f8f7ff', 'G': '#fff4a3', 'K': '#ffd2a1',
        'M': '#ffb47c', 'L': '#ff8c5a', 'T': '#d95c14',
        'Y': '#9e4b2c',
        // Экзотика (99.2.4 §8): свои цвета, ветка раньше ветки по классу.
        'black_hole': '#2a1a4a', 'neutron': '#a0d8ef',
        'white_dwarf': '#f0f0f0', 'protostar': '#ff7950',
    };
    if (starType && colors[starType]) return colors[starType];
    return colors[spectralClass] || '#ffffff';
}

export function getStarSize(spectralClass, starType) {
    const sizes = {
        'O': 120, 'B': 105, 'A': 90,
        'F': 75, 'G': 60,
        'K': 48, 'M': 36,
        'L': 30, 'T': 24,
        'Y': 18
    };
    if (starType && starType !== 'star') {
        // Экзотика — фиксированный малый размер (ЧД/НЗ/WD — компактные, §8).
        if (starType === 'black_hole' || starType === 'neutron') return 7;
        if (starType === 'white_dwarf') return 11;
        return 20; // протозвезда
    }
    return sizes[spectralClass] || 60;
}

// starTypeLabel — человекочитаемый тип объекта для модалки (99.2.4 §8).
export function starTypeLabel(starType) {
    const labels = {
        'star': 'обычная звезда',
        'white_dwarf': 'белый карлик',
        'neutron': 'нейтронная звезда',
        'black_hole': 'чёрная дыра',
        'protostar': 'протозвезда',
    };
    return labels[starType] || '';
}

// systemTypeLabel — человекочитаемый тип системы для модалки (99.2.4 §8).
export function systemTypeLabel(systemType) {
    const labels = {
        'single': 'одиночная',
        'binary': 'двойная',
        'multiple': 'кратная (3+)',
    };
    return labels[systemType] || '';
}

// starModsBadges — человекочитаемые строки модификаторов (99.2.4 §8):
// фаза (гигант/сверхгигант), переменность (тип + период/амплитуда),
// подтипы (пульсар/магнетар/микроквазар), параметры двойной.
// belts — пояса системы из ответа модалки (спека поясов этап 2 §4.4): значок
// disk_state='debris' вытесняется показом пояса kind='debris' (один факт —
// одно место, РБ3). Сервер уже отфильтровал пояса по visible для игрока;
// admin видит все — подавление по наличию записи.
export function starModsBadges(mods, belts) {
    if (!mods || typeof mods !== 'object') return [];
    const badges = [];
    if (mods.phase === 'III') badges.push('гигант (фаза III)');
    if (mods.phase === 'I') badges.push('сверхгигант (фаза I)');
    if (mods.subtype === 'pulsar') badges.push('пульсар');
    if (mods.subtype === 'magnetar') badges.push('магнетар');
    if (mods.subtype === 'accretion') badges.push('микроквазар (аккреция)');
    if (mods.subtype === 'lbv') badges.push('яркая голубая переменная (LBV)');
    if (mods.subtype === 'wr') badges.push('звезда Вольфа–Райе (WR)');
    if (mods.variable_type) {
        const names = {
            eclipsing: 'затменная', mira: 'мирида', cepheid: 'цефеида',
            uv_ceti: 'вспыхивающая (UV Кита)', t_tauri: 'T Тельца',
            nova: 'новая', dwarf_nova: 'карликовая новая',
        };
        let s = 'переменная: ' + (names[mods.variable_type] || mods.variable_type);
        if (mods.variable_period_days) s += ', период ' + Number(mods.variable_period_days).toFixed(1) + ' сут';
        if (mods.variable_amplitude) s += ', амплитуда ' + Number(mods.variable_amplitude).toFixed(2) + 'm';
        badges.push(s);
    }
    if (mods.binary_type) {
        badges.push(mods.binary_type === 'wide' ? 'двойная широкая (S-тип)' : 'двойная тесная (P-тип)');
    }
    if (mods.disk_state) {
        // Пояс kind='debris' материализует disk_state='debris' — значок не
        // показывается, факт идёт секцией «Пояса» (§4.4). protoplanetary/
        // accretion — не пояса, значки не трогаются.
        const materialized = mods.disk_state === 'debris' &&
            Array.isArray(belts) && belts.some(b => b && b.kind === 'debris');
        if (!materialized) {
            const disks = { protoplanetary: 'протопланетный диск', accretion: 'аккреционный диск', debris: 'обломочный пояс' };
            badges.push(disks[mods.disk_state] || 'диск');
        }
    }
    return badges;
}

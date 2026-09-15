export const CONFIG = {
    map: {
        minZoom: 0.001,        // было 0.02 — теперь можно отдалить до «всей галактики»
        maxZoom: 20,
        zoomStep: 1.2,
        wheelSensitivity: 0.9,
        minDistForClick: 30,
        shipSize: 2.8,       // полёт: спрайт = shipSize*3.2 ≈ 9 px — вровень с иконками агентов (npc_agents.js)
        nameFontSize: 10,
        nameMinFontSize: 11,   // минимум подписи звезды (читаемость на малом зуме)
        nameMaxFontSize: 40,   // потолок: на большом зуме без роста до бесконечности
        padding: 80,
        minRadius: 1.5,        // было 2 — чуть меньше, чтобы при отдалении не слипались
        baseRadius: 8,
        nameDisplayThreshold: 0.5,
        nameAlwaysShowLimit: 30,   // если одиночных звёзд на экране не больше — подписи не скрываются
        regionDisplayThreshold: 0.6,   // регионы полностью исчезают только когда уже появились названия звёзд (0.5)
        regionFontSize: 13,
        regionMaxFontSize: 48,         // потолок: названия регионов растут с зумом от fit-размера
        regionMinPxRadius: 40,         // минимальный радиус пятна региона на экране
        regionNamesZoom: 0.02,         // ниже этого зума показываются названия регионов
        starColors: {
            'O': '#9bb0ff',
            'B': '#aac7ff',
            'A': '#f8f7ff',
            'F': '#fff4e8',
            'G': '#ffd700',
            'K': '#ffa500',
            'M': '#ff6348',
            'L': '#8b5a2b',
            'T': '#6b4c3b',
            'Y': '#4d3b2b',
            // Экзотика (99.2.4 §8): ЧД — тёмно-пурпурный/чёрный, нейтронная —
            // холодный голубой, WD — белый/серебристый, протозвезда —
            // красно-оранжевый. «Прочая экзотика» (сверхгиганты) — по классу O–A.
            'black_hole': '#2a1a4a',
            'neutron': '#a0d8ef',
            'white_dwarf': '#f0f0f0',
            'protostar': '#ff7950',
            'default': '#8b5cf6'
        },
        // Диапазоны температур [мин, макс] спектральных классов — зеркально
        // spectralWeights в internal/generator/galaxy/galaxy.go. По ним считается
        // положение звезды внутри класса → лёгкий сдвиг светимости (getStarShade).
        starTempRanges: {
            'O': [30000, 50000],
            'B': [10000, 30000],
            'A': [7500, 10000],
            'F': [6000, 7500],
            'G': [5200, 6000],
            'K': [3700, 5200],
            'M': [2400, 3700],
            'L': [1300, 2400],
            'T': [700, 1300],
            'Y': [300, 700]
        }
    },
    ui: {
        statusUpdateInterval: 1000,
        tooltipOffset: 20,
        tooltipMaxWidth: 220,
        tooltipMaxHeight: 120
    }
};
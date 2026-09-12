export const CONFIG = {
    map: {
        minZoom: 0.001,        // было 0.02 — теперь можно отдалить до «всей галактики»
        maxZoom: 10,
        zoomStep: 1.2,
        wheelSensitivity: 0.9,
        minDistForClick: 30,
        shipSize: 12,
        nameFontSize: 10,
        gridStep: 5,
        padding: 80,
        minRadius: 1.5,        // было 2 — чуть меньше, чтобы при отдалении не слипались
        baseRadius: 8,
        nameDisplayThreshold: 0.5,
        nameAlwaysShowLimit: 30,   // если одиночных звёзд на экране не больше — подписи не скрываются
        gridDisplayThreshold: 10,
        regionDisplayThreshold: 0.05,  // при зуме ниже — вместо шариков видны регионы
        regionFontSize: 13,
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
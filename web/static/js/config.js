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
        }
    },
    ui: {
        statusUpdateInterval: 1000,
        tooltipOffset: 20,
        tooltipMaxWidth: 220,
        tooltipMaxHeight: 120
    }
};
// web/static/js/map/config.js
import { CONFIG } from '../config.js';

export const state = {
    worlds: [],           // кэш отдельных миров (для tooltip/fly/nav)
    clusters: [],         // текущие кластеры для рендера (с сервера)
    regions: [],          // регионы галактики (для малого зума, с сервера)
    galaxyRadius: 0,      // радиус галактики в мировых координатах (из регионов)
    minZoom: 0,           // вычисляется: масштаб, при котором галактика помещается в экран
    currentWorldId: null,
    isFlying: false,
    flyFrom: null,
    flyTo: null,
    flyDuration: 0,
    flyStartTime: 0,
    flyStartX: null,   // стартовая точка сегмента (61a): при редиректе — точка P
    flyStartY: null,
    // Ускоритель в панели полёта (ЧК3, спека ускорителя §3.4/§4.4).
    // accel — блок accelerator из ответа /travel (свежий, оба пути 202);
    // accelMe — блок accelerator из /me (нужен вне свежего /travel: возврат
    // с карты/после мини-игры); shipCatalog — /me.ship_catalog (параметры
    // текущего модуля: min_remaining_offer_s и т.п.). *Until — якоря локального
    // досчёта отката: момент, вычисленный из cooldown_remaining_s на клиенте.
    accel: null,
    accelMe: null,
    shipCatalog: [],
    accelCooldownUntil: 0,
    accelMeCooldownUntil: 0,
    selectedWorldId: null,
    hoveredWorldId: null,
    isDragging: false,
    dragStartX: 0,
    dragStartY: 0,
    dragStartOffsetX: 0,
    dragStartOffsetY: 0,
    offsetX: 0,
    offsetY: 0,
    scale: 1,
    canvasWidth: 0,
    canvasHeight: 0,
    followShip: false,   // активно следить за кораблём во время полёта
    focusWorldId: null,  // мир, найденный поиском (рисуется кольцом)
    highlightedNpcId: null, // подсвеченный агент поиска (ореол, спека 26a.1 §6.2)
    npcPositions: [],    // NPC-агенты для карты (спека 20a.1 §7): {id,name,x,y,status,...}
    userId: null,        // id игрока (для дефолтной схемы корабля, спека 99.2.15 §8)
    userShipIcon: '',    // PNG-имя спрайта корабля из /me (спека 61b §6.1)
    userShipColor: null, // цвет перекраски спрайта из /me (NULL = «Оригинал», спека 61b §6.1)
    userRole: '',        // роль пользователя ('admin'/'skycomposer' → FPS-счётчик на карте)
    radarRadius: 0,      // радиус радара игрока из /me (спека 77a §4.2; 0 = не загружен)
    hasEngine: true,     // установлен ли двигатель из /me (спека 91a §6.1; true = не загружено —
                         // сервер валидирует сам, UI не блокирует до данных)
    playerPositions: [], // чужие игроки в радиусе радара (спека 77a §5.3): {id,username,ship_icon,ship_color,status,x,y}
    starSingles: 0,      // одиночных звёзд в кадре (порог деградации вида звёзд, идея 2026-09-22 §3)
    npcAnimActive: false, // идёт цикл интерполяции NPC (один хозяин кадра: star_render пропускает draw, идея 2026-09-22 §4)
};

export const elements = {
    canvas: document.getElementById('mapCanvas'),
    ctx: document.getElementById('mapCanvas').getContext('2d'),
    tooltip: document.getElementById('tooltip'),
    tooltipName: document.getElementById('tooltipName'),
    tooltipType: document.getElementById('tooltipType'),
    tooltipLevel: document.getElementById('tooltipLevel'),
    tooltipFlyBtn: document.getElementById('tooltipFlyBtn'),
    statusBar: document.getElementById('status-bar'),
    zoomInfo: document.getElementById('zoom-info'),
    loading: document.getElementById('loading'),
};
// web/static/js/map/config.js
import { CONFIG } from '../config.js';

export const state = {
    worlds: [],           // кэш отдельных миров (для tooltip/fly/nav)
    clusters: [],         // текущие кластеры для рендера (с сервера)
    currentWorldId: null,
    isFlying: false,
    flyFrom: null,
    flyTo: null,
    flyDuration: 0,
    flyStartTime: 0,
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
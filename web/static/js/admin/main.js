// web/static/js/admin/main.js
import { initTabs } from './tabs.js';
import { loadStats, loadPlanetStats } from './stats.js';
import { loadWorlds, deleteWorld, createWorld } from './worlds.js';
import { 
    generateUniverse, generatePlanets, generateFactions, generateResources,
    cancelGeneration, clearUniverse, applyPreset 
} from './generation.js';
import { setPassword } from './auth.js';
import { initProbe, runProbe, cancelProbe, loadResults, showProbeResult, compareProbe,
    setProbeMode, liveToggle, liveSpeed, liveStep, liveSeek } from './probe.js';

// Глобальные функции для onclick в HTML
window.setPassword = setPassword;
window.applyPreset = applyPreset;
window.loadStats = loadStats;
window.loadWorlds = loadWorlds;
window.deleteWorld = deleteWorld;
window.createWorld = createWorld;
window.generateUniverse = generateUniverse;
window.generatePlanets = generatePlanets;
window.generateFactions = generateFactions;
window.generateResources = generateResources;
window.cancelGeneration = cancelGeneration;
window.clearUniverse = clearUniverse;
window.loadPlanetStats = loadPlanetStats;
window.runProbe = runProbe;
window.cancelProbe = cancelProbe;
window.loadResults = loadResults;
window.showProbeResult = showProbeResult;
window.compareProbe = compareProbe;
window.setProbeMode = setProbeMode;
window.liveToggle = liveToggle;
window.liveSpeed = liveSpeed;
window.liveStep = liveStep;
window.liveSeek = liveSeek;

export function initAdmin() {
    initTabs();
    loadStats();
    loadWorlds(1);
    initProbe();
}
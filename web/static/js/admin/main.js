// web/static/js/admin/main.js
import { initTabs } from './tabs.js';
import { loadStats, loadPlanetStats } from './stats.js';
import { loadWorlds, deleteWorld, createWorld } from './worlds.js';
import { 
    generateUniverse, generatePlanets, generateFactions, generateResources,
    generateSettlements,
    cancelGeneration, clearUniverse, applyPreset 
} from './generation.js';
import { setPassword } from './auth.js';

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
window.generateSettlements = generateSettlements;
window.generateResources = generateResources;
window.cancelGeneration = cancelGeneration;
window.clearUniverse = clearUniverse;
window.loadPlanetStats = loadPlanetStats;

export function initAdmin() {
    initTabs();
    loadStats();
    loadWorlds(1);
}
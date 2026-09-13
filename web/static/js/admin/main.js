// web/static/js/admin/main.js
import { initTabs } from './tabs.js';
import { loadStats, loadPlanetStats } from './stats.js';
import { loadWorlds, deleteWorld, createWorld } from './worlds.js';
import { 
    generateUniverse, generatePlanets, generateFactions, generateResources,
    generateSettlements, loadSettlementFields, renderSettlementModel,
    addSettlementRule, setSettleMode, setSettlePopKind, applySettlementPreset,
    cancelGeneration, clearUniverse, clearSettlements, applyPreset 
} from './generation.js';
import { setPassword } from './auth.js';
import { populateHypothesisPresets, renderHypothesisForm, runHypothesis } from './hypothesis.js';

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
window.clearSettlements = clearSettlements;
window.loadPlanetStats = loadPlanetStats;
window.renderSettlementModel = renderSettlementModel;
window.addSettlementRule = addSettlementRule;
window.loadSettlementFields = loadSettlementFields;
window.setSettleMode = setSettleMode;
window.setSettlePopKind = setSettlePopKind;
window.applySettlementPreset = applySettlementPreset;
window.renderHypothesisForm = renderHypothesisForm;
window.runHypothesis = runHypothesis;

export function initAdmin() {
    initTabs();
    loadSettlementFields();
    populateHypothesisPresets();
    loadStats();
    loadWorlds(1);
}
// web/static/js/admin/main.js
import { initTabs } from './tabs.js';
import { loadStats, loadPlanetStats } from './stats.js';
import { loadWorlds, deleteWorld, createWorld, initWorldsSorting } from './worlds.js';
import {
    generateUniverse, generatePlanets, generateFactions, generateResources,
    generateRaceSettlements, loadSettlementFields,
    cancelGeneration, clearUniverse, clearSettlements, applyPreset,
    switchGenSubTab, showTab, loadGenConfig, saveGenConfig, recalcGenWeights,
    regeneratePlanets
} from './generation.js';
import { ensureAdminAuth, setAfterLogin, getAdminRole, adminLogin } from './auth.js';
import {
    loadUsers, createUser, toggleCreateUserForm, openUserProfile,
    changeUserRole, resetUserPassword, deleteUser
} from './users.js';
import { populateHypothesisPresets, renderHypothesisForm, runHypothesis } from './hypothesis.js';
import {
    initNPC, loadNPC, loadNPCSettings, saveNPCSettings,
    createAgent, toggleNotify, deleteAgent, generateBulk, loadMoreNPC, loadNPCMetrics,
    clearAllAgents
} from './npc.js';
import { saveSettlementSettings, updateSettlementNetto } from './settlementSettings.js';
import { loadResources } from './resources.js';

// Глобальные функции для onclick в HTML
window.adminLogin = adminLogin;
window.applyPreset = applyPreset;
window.loadStats = loadStats;
window.loadWorlds = loadWorlds;
window.deleteWorld = deleteWorld;
window.createWorld = createWorld;
window.generateUniverse = generateUniverse;
window.generatePlanets = generatePlanets;
window.generateFactions = generateFactions;
window.generateRaceSettlements = generateRaceSettlements;
window.generateResources = generateResources;
window.cancelGeneration = cancelGeneration;
window.clearUniverse = clearUniverse;
window.clearSettlements = clearSettlements;
window.loadPlanetStats = loadPlanetStats;
window.loadSettlementFields = loadSettlementFields;
window.renderHypothesisForm = renderHypothesisForm;
window.runHypothesis = runHypothesis;
window.loadUsers = loadUsers;
window.createUser = createUser;
window.toggleCreateUserForm = toggleCreateUserForm;
window.openUserProfile = openUserProfile;
window.changeUserRole = changeUserRole;
window.resetUserPassword = resetUserPassword;
window.deleteUser = deleteUser;
window.saveNPCSettings = saveNPCSettings;
window.createAgent = createAgent;
window.toggleNotify = toggleNotify;
window.deleteAgent = deleteAgent;
window.loadNPC = loadNPC;
window.loadMoreNPC = loadMoreNPC;
window.generateBulk = generateBulk;
window.loadNPCMetrics = loadNPCMetrics;
window.clearAllAgents = clearAllAgents;
window.saveSettlementSettings = saveSettlementSettings;
window.updateSettlementNetto = updateSettlementNetto;
window.switchGenSubTab = switchGenSubTab;
window.showTab = showTab;
window.loadGenConfig = loadGenConfig;
window.saveGenConfig = saveGenConfig;
window.recalcGenWeights = recalcGenWeights;
window.regeneratePlanets = regeneratePlanets;
window.loadResources = loadResources;

// Ключ в localStorage читает web/static/js/modal/panel.js (карточка планеты
// в игровых страницах) — держать строку синхронной при переименовании.
const AUTO_REFRESH_PLANET_KEY = 'debugAutoRefreshPlanet';

function initAutoRefreshToggle() {
    const checkbox = document.getElementById('autoRefreshPlanetToggle');
    if (!checkbox) return;
    checkbox.checked = localStorage.getItem(AUTO_REFRESH_PLANET_KEY) === '1';
    checkbox.addEventListener('change', () => {
        localStorage.setItem(AUTO_REFRESH_PLANET_KEY, checkbox.checked ? '1' : '0');
    });
}

// initAdmin — старт админки: сначала проверка токена и роли (спека 99.2.14 §4),
// данные грузим только после успешного входа. После логина через оверлей
// initAdminData вызывается колбэком из auth.js.
export async function initAdmin() {
    setAfterLogin(initAdminData);
    const ok = await ensureAdminAuth();
    if (!ok) return;
    initAdminData();
}

function initAdminData() {
    // Вкладка «Пользователи» видна только skycomposer (§8). Если админ вошёл,
    // а в localStorage осталась активная вкладка из сессии skycomposer'а — сброс.
    const isSkycomposer = getAdminRole() === 'skycomposer';
    const usersTabBtn = document.getElementById('tab-users-btn');
    if (usersTabBtn) {
        usersTabBtn.style.display = isSkycomposer ? '' : 'none';
    }
    if (!isSkycomposer && localStorage.getItem('adminActiveTab') === 'tab-users') {
        localStorage.removeItem('adminActiveTab');
    }
    initTabs();
    loadSettlementFields();
    populateHypothesisPresets();
    loadStats();
    loadWorlds(1);
    initWorldsSorting();
    initAutoRefreshToggle();
}
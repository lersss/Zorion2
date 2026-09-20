// web/static/js/admin/poll.js
import { fetchWithAuth } from './auth.js';
import { loadStats } from './stats.js';
import { loadWorlds } from './worlds.js';
import { loadNPC, loadNPCMetrics } from './npc.js';

export const pollIntervals = {};

// JOB_UI — реестр джобов генерации (идея 2026-09-20): jobType (значение
// /admin/generate-status) → ключ интервала pollIntervals (как в стартовых
// функциях generation.js/npc.js/hypothesis.js), id прогресс-контейнера,
// id результата, id кнопки «Остановить» в секции. У джобов без контейнера
// в админке (generate_resources) id пустые — для них работает только
// красная кнопка-стоп у заголовка «Генерация».
const JOB_UI = {
    generate_universe:         { key: 'universe',         progressId: 'genProgress',           resultId: 'genResult',           cancelBtnId: 'cancelUniverseBtn' },
    generate_planets:          { key: 'planets',          progressId: 'planetProgress',        resultId: 'planetResult',        cancelBtnId: 'cancelPlanetsBtn' },
    generate_factions:         { key: 'factions',         progressId: 'factionProgress',       resultId: 'factionResult',       cancelBtnId: 'cancelFactionsBtn' },
    generate_resources:        { key: 'resources',        progressId: 'resourceProgress',      resultId: 'resourceResult',      cancelBtnId: 'cancelResourcesBtn' },
    generate_race_settlements: { key: 'race_settlements', progressId: 'raceSettlementProgress', resultId: 'raceSettlementResult', cancelBtnId: 'cancelRaceSettlementsBtn' },
    regenerate_planets:        { key: 'regenerate',       progressId: 'regenerateProgress',    resultId: 'regenerateResult',    cancelBtnId: 'cancelRegenerateBtn' },
    generate_npc:              { key: 'generate_npc',     progressId: 'npcBulkProgress',       resultId: 'npcBulkResult',       cancelBtnId: '' },
    hypothesis:                { key: 'hypothesis',       progressId: 'hypothesisProgress',    resultId: 'hypothesisResult',    cancelBtnId: 'cancelHypothesisBtn' },
    pacman:                    { key: 'pacman',           progressId: 'pacmanProgress',        resultId: 'pacmanResult',        cancelBtnId: 'cancelPacmanBtn' },
};

// runningJobs — множество jobType, у которых последний опрос дал 'running'.
// Питается из pollJob (все интервалы) и restoreJobStates; на нём работает
// красная кнопка-стоп у заголовка «Генерация».
export const runningJobs = new Set();

// updateGenStopBtn — красная кнопка-стоп (идея 2026-09-20): видна, пока
// крутится хоть один джоб генерации; скрывается, когда джобов нет.
function updateGenStopBtn() {
    const btn = document.getElementById('genStopBtn');
    if (btn) btn.style.display = runningJobs.size > 0 ? 'inline-block' : 'none';
}

export async function pollJob(jobType, progressId, resultId, cancelBtnId) {
    try {
        const res = await fetchWithAuth(`/admin/generate-status?job=${jobType}`);
        const data = await res.json();
        const progress = data.processed || 0;
        const total = data.total || 0;
        const status = data.status || 'running';

        // Красная кнопка-стоп (идея 2026-09-20): running-джобы копятся в
        // runningJobs, кнопка показывается/скрывается по их наличию.
        if (status === 'running') {
            runningJobs.add(jobType);
        } else {
            runningJobs.delete(jobType);
        }
        updateGenStopBtn();

        const textEl = document.getElementById(progressId + 'Text');
        const barEl = document.getElementById(progressId + 'Bar');
        if (textEl) textEl.textContent = total > 0 ? `${progress} из ${total}` : 'Подготовка...';
        if (barEl) {
            const percent = total > 0 ? (progress / total * 100) : 0;
            barEl.value = percent;
        }

        const cancelBtn = document.getElementById(cancelBtnId);
        if (cancelBtn) {
            cancelBtn.style.display = (status === 'running') ? 'inline-block' : 'none';
        }

        // Пакман (спека 2026-09-20 §2.1): пока пакман ест, «Удалить все миры»
        // недоступна; кнопка пакмана дизейблится на любой идущий джоб
        // (клиентская защита; серверная — 409).
        const clearBtn = document.getElementById('clearUniverseBtn');
        if (clearBtn) clearBtn.disabled = (jobType === 'pacman' && status === 'running');
        const pacmanBtn = document.getElementById('pacmanStartBtn');
        if (pacmanBtn) pacmanBtn.disabled = (status === 'running');

        if (status === 'done' || status === 'error' || status === 'canceled') {
            // Ключ интервала — из реестра JOB_UI: стартовые функции хранят
            // интервалы под короткими ключами ('universe', 'planets', ...),
            // а не под jobType — иначе интервал не очищается и поллит вечно.
            const ui = JOB_UI[jobType];
            const intervalKey = ui ? ui.key : jobType;
            clearInterval(pollIntervals[intervalKey]);
            delete pollIntervals[intervalKey];
            const progressEl = document.getElementById(progressId);
            if (progressEl) progressEl.style.display = 'none';
            if (cancelBtn) cancelBtn.style.display = 'none';
            if (clearBtn) clearBtn.disabled = false;
            if (pacmanBtn) pacmanBtn.disabled = false;
            const resultEl = document.getElementById(resultId);
            if (resultEl && status === 'done') {
                resultEl.textContent = `✅ Готово! (${total} объектов)`;
                if (jobType === 'generate_universe') {
                    // Недобор миров — штатная ситуация: при плотном запросе
                    // кластеры и «добивка» упираются в лимит попыток.
                    // processed = реально создано миров, total = запрошено.
                    resultEl.textContent = progress < total
                        ? `⚠️ Готово, но НЕДОБОР! Миров создано: ${progress} из ${total}`
                        : `✅ Готово! Миров создано: ${progress}`;
                    loadStats();
                    loadWorlds(1);
                } else if (jobType === 'generate_planets') {
                    loadStats();
                } else if (jobType === 'hypothesis') {
                    resultEl.textContent = data.report || `✅ Эксперимент завершён: ${progress} планет`;
                    loadStats();
                } else if (jobType === 'generate_npc') {
                    // Массовая генерация NPC (спека 26a.1 §9): отчёт джоба +
                    // обновить таблицу (первая страница) и метрики.
                    resultEl.textContent = data.report || `✅ Создано агентов: ${progress}`;
                    const bulkBtn = document.getElementById('npcBulkBtn');
                    if (bulkBtn) bulkBtn.disabled = false;
                    loadNPC();
                    loadNPCMetrics();
                } else if (jobType === 'generate_race_settlements') {
                    // Поселения рас (спека 99.2.21 §7, идея 56a): отчёт —
                    // «Не заселились (N из 50): ...» (копилка для разбора
                    // причин); все 50 заселились — отчёта нет, generic.
                    resultEl.textContent = data.report || `✅ Поселения рас сгенерированы`;
                } else if (jobType === 'pacman') {
                    // Пакман (спека 2026-09-20 §2.1): отчёт «Пакман съел N
                    // миров за X сек» + обновить статистику.
                    resultEl.textContent = data.report || `✅ Пакман съел ${progress} миров`;
                    loadStats();
                }
            } else if (resultEl && status === 'canceled') {
                resultEl.textContent = `⏹️ Остановлено пользователем`;
            } else if (resultEl && status === 'error') {
                resultEl.textContent = `❌ Ошибка: ${data.error || 'неизвестная'}`;
            }
        }
    } catch (e) {
        console.error('Poll error:', e);
    }
}

// restoreJobStates — восстановление состояния джобов после перезагрузки
// (идея 2026-09-20): опрос статусов всех джобов генерации; для running —
// показать прогресс-контейнер, добавить в runningJobs (красная кнопка-стоп)
// и запустить интервал pollJob, если его ещё нет. Вызывается при загрузке
// админки (initAdminData) и при активации вкладки «Генерация».
let restoreInFlight = false;

export async function restoreJobStates() {
    if (restoreInFlight) return;
    restoreInFlight = true;
    try {
        for (const [jobType, ui] of Object.entries(JOB_UI)) {
            if (pollIntervals[ui.key]) continue; // интервал уже крутится
            let data;
            try {
                const res = await fetchWithAuth(`/admin/generate-status?job=${jobType}`);
                data = await res.json();
            } catch (e) {
                console.error('Restore status error:', e);
                continue;
            }
            if (data.status !== 'running') continue;
            runningJobs.add(jobType);
            updateGenStopBtn();
            const progressEl = document.getElementById(ui.progressId);
            if (progressEl) progressEl.style.display = 'block';
            pollIntervals[ui.key] = setInterval(
                () => pollJob(jobType, ui.progressId, ui.resultId, ui.cancelBtnId),
                jobType === 'pacman' ? 1000 : 1500);
        }
    } finally {
        restoreInFlight = false;
    }
}

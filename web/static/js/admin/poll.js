// web/static/js/admin/poll.js
import { fetchWithAuth } from './auth.js';
import { loadStats } from './stats.js';
import { loadWorlds } from './worlds.js';
import { loadNPC, loadNPCMetrics } from './npc.js';

export const pollIntervals = {};

export async function pollJob(jobType, progressId, resultId, cancelBtnId) {
    try {
        const res = await fetchWithAuth(`/admin/generate-status?job=${jobType}`);
        const data = await res.json();
        const progress = data.processed || 0;
        const total = data.total || 0;
        const status = data.status || 'running';

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

        if (status === 'done' || status === 'error' || status === 'canceled') {
            clearInterval(pollIntervals[jobType]);
            delete pollIntervals[jobType];
            document.getElementById(progressId).style.display = 'none';
            if (cancelBtn) cancelBtn.style.display = 'none';
            const resultEl = document.getElementById(resultId);
            if (status === 'done') {
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
                }
            } else if (status === 'canceled') {
                resultEl.textContent = `⏹️ Остановлено пользователем`;
            } else if (status === 'error') {
                resultEl.textContent = `❌ Ошибка: ${data.error || 'неизвестная'}`;
            }
        }
    } catch (e) {
        console.error('Poll error:', e);
    }
}
-- Индекс пагинации списка агентов (спека 26a.1 §5.3): keyset-фильтр
-- (created_at, id) < (...) + сортировка created_at DESC, id DESC —
-- O(страница) при сотнях тысяч строк (без индекса — sort всей таблицы
-- на каждый запрос страницы ~50–100 мс).
-- Индекс (status, id) из 000026 остаётся для планировщика (batch-тикание);
-- новый — аддитивен, на тик не влияет.
CREATE INDEX IF NOT EXISTS idx_npc_agents_created_id ON npc_agents (created_at DESC, id DESC);
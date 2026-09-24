-- 000083_settlement_rename.sql — ренейм-миграция канон-имён класса поселения
-- (спека 2026-09-24-каталог-экспорт-импорт-контента-на-прод §6, итерация И1).
--
-- Класс-корень «Поселение» → «Колония»; ступени «Аутпост» → «Форпост»,
-- «Посёлок» → «Поселение» (решение создателя 2026-09-24 «новые канон», идея
-- 2026-09-22-каталог-стабильный-код §8). Миграция нужна существующим БД с
-- маркером сидера (сид не запускается): без неё канон-имена не доедут, а
-- первичная накатка/бэкфилл сопоставляют записи по именам (§3.5, §6).
--
-- Резолв — ПО ДАННЫМ, не по литералам-двойникам (имя `поселение` до ренейма
-- носят ДВА разных узла: класс-корень `parent_id IS NULL` и вторая ступень
-- ладдеры). Корень опознаётся как родитель ступени ладдеры (`params ? 'stage'`)
-- либо как родитель типа-якоря `generation_config.default_settlement_type_id` —
-- в обоих случаях это именно корень, а ступени отличаются по `parent_id`.
--
-- Порядок обязателен: сначала корень (освобождает имя `поселение`), затем
-- ступени (вторая ступень занимает `поселение`) — иначе `UNIQUE(name_norm)`
-- конфликтует. Всё идемпотентно: уже переименованная БД / отсутствие старого
-- имени — no-op. `default_settlement_type_id` и `settlements.settlement_type_id`
-- — id-ключевые, НЕ трогаются.
--
-- Безопасность (п.6 ревьюера): если целевое имя (`колония`) уже занято иным
-- типом, шаг 1 даёт ЯВНЫЙ отказ (RAISE) — миграция останавливается, транзакция
-- откатывается, молчаливой порчи нет.
--
-- Номер 000083 зафиксирован менеджером (последняя в дереве/DB.md — 000082).
-- На dev-БД переименование уже сделано вручную — миграция там no-op.

DO $$
DECLARE
    v_root     bigint;
    v_conflict bigint;
BEGIN
    -- Корень класса: родитель ступени ладдеры (наличие params.stage).
    SELECT DISTINCT pt.parent_id INTO v_root
    FROM producer_types pt
    WHERE pt.params ? 'stage' AND pt.parent_id IS NOT NULL
    LIMIT 1;

    -- Фолбэк: родитель типа-якоря базового поселения.
    IF v_root IS NULL THEN
        SELECT self.parent_id INTO v_root
        FROM generation_config gc
        JOIN producer_types self ON self.id = (gc.payload #>> '{}')::bigint
        WHERE gc.key = 'default_settlement_type_id'
          AND self.parent_id IS NOT NULL;
    END IF;

    -- Класса ещё нет (свежая БД: сид идёт после миграций) — нечего делать.
    IF v_root IS NULL THEN
        RETURN;
    END IF;

    -- Шаг 1. Класс-корень «Поселение» → «Колония» (только строка-корень).
    IF EXISTS (SELECT 1 FROM producer_types WHERE id = v_root AND name_norm = 'поселение') THEN
        SELECT id INTO v_conflict FROM producer_types WHERE name_norm = 'колония' AND id <> v_root LIMIT 1;
        IF v_conflict IS NOT NULL THEN
            RAISE EXCEPTION '000083: name_norm ''колония'' уже занят типом % — класс-корень % не переименован (разреши конфликт вручную)', v_conflict, v_root;
        END IF;
        UPDATE producer_types SET name = 'Колония', name_norm = 'колония' WHERE id = v_root;
    END IF;

    -- Шаг 2. Ступень-пол «Аутпост» → «Форпост».
    UPDATE producer_types SET name = 'Форпост', name_norm = 'форпост'
    WHERE parent_id = v_root AND name_norm = 'аутпост';

    -- Шаг 3. Вторая ступень «Посёлок» → «Поселение» (строго после шага 1).
    UPDATE producer_types SET name = 'Поселение', name_norm = 'поселение'
    WHERE parent_id = v_root AND name_norm = 'посёлок';
END $$;

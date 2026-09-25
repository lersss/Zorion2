-- 000087_retire_branch_buffers.sql — переезд механики с буферов ветки на ячейки
-- внутреннего хранилища (спека 2026-09-25-внутреннее-хранилище-и-рождение-
-- заказов §4.1/§4.4, ЧК2а, подэтап 2б).
--
-- Что делает:
--   1) переносит остатки settlement_branch_buffers в settlement_storage_cells,
--      агрегируя по (поселение, товар) — сумма amount по веткам и обоим
--      направлениям (вход/выход) одного поселения;
--   2) проставляет cap_share (вес ячейки, F3): явная ручка
--      producer_types.params.storage.shares побеждает; иначе число вхождений
--      товара во входы рецептов (recipe_components.component_id); для товара с
--      привязкой эффекта (потребность населения) — не меньше 1 (max(occ,1));
--   3) выводит буферы из обращения: RENAME в settlement_branch_buffers_retired
--      (НЕ DROP — данные для отката; финальный DROP — отдельной миграцией),
--      снятие FK и индексов.
--
-- Выбор по cap_share: точный расчёт веса в SQL приблизителен (ручка shares
-- читается из типа ТЕКУЩЕЙ ступени поселения; привязка эффекта — по name_norm
-- товара в params.effects любого типа). Поэтому cap_share здесь — стартовое
-- значение; owner-проход при сверке набора нужд (§4.3) пересчитывает его по
-- settlement.GoodWeights (самолечение весов). Количества переносятся точно.
--
-- Идемпотентность: перенос/веса/ретайр — под проверкой to_regclass; повторный
-- прогон — no-op. Номер 000087 (000086 уже применена, не трогается).

DO $$
BEGIN
    IF to_regclass('settlement_branch_buffers') IS NULL THEN
        RETURN; -- уже ретайрена — повторный прогон no-op
    END IF;

    -- 1) Перенос остатков: агрегация по (поселение, товар), только живые
    --    поселения (JOIN settlements). ON CONFLICT DO NOTHING — ячейка уже
    --    могла быть создана (идемпотентность).
    INSERT INTO settlement_storage_cells (owner_type, owner_id, good_id, amount, cap_share)
    SELECT 'settlement', b.settlement_id, bb.good_id, SUM(bb.amount), 0
    FROM settlement_branch_buffers bb
    JOIN settlement_branches b ON b.id = bb.branch_id
    JOIN settlements s ON s.id = b.settlement_id
    GROUP BY b.settlement_id, bb.good_id
    ON CONFLICT (owner_type, owner_id, good_id) DO NOTHING;

    -- 2) cap_share (F3): ручка shares > вхождения во входы рецептов, для
    --    товара с эффектом — max(occ, 1).
    WITH occ AS (
        SELECT component_id AS good_id, COUNT(*)::double precision AS n
        FROM recipe_components
        WHERE component_id IS NOT NULL
        GROUP BY component_id
    ),
    eff_goods AS (
        SELECT DISTINCT g.id AS good_id
        FROM goods g
        WHERE g.name_norm IN (
            SELECT jsonb_object_keys(pt.params->'effects')
            FROM producer_types pt
            WHERE pt.params ? 'effects'
              AND jsonb_typeof(pt.params->'effects') = 'object'
        )
    ),
    explicit AS (
        SELECT s.id AS settlement_id,
               (kv.key)::bigint AS good_id,
               (kv.value)::double precision AS share
        FROM settlements s
        JOIN producer_types pt ON pt.id = s.settlement_type_id
        CROSS JOIN LATERAL jsonb_each_text(pt.params->'storage'->'shares') AS kv
        WHERE jsonb_typeof(pt.params->'storage'->'shares') = 'object'
    )
    UPDATE settlement_storage_cells c
    SET cap_share = COALESCE(
            (SELECT e.share FROM explicit e
             WHERE e.settlement_id = c.owner_id AND e.good_id = c.good_id),
            GREATEST(
                COALESCE((SELECT o.n FROM occ o WHERE o.good_id = c.good_id), 0),
                CASE WHEN EXISTS (SELECT 1 FROM eff_goods eg WHERE eg.good_id = c.good_id)
                     THEN 1 ELSE 0 END
            )
        )
    WHERE c.owner_type = 'settlement';

    -- 2.5) Размер хранилища (спека §4.1): база — ручка
    --      producer_types.params.storage.size типа ТЕКУЩЕЙ ступени поселения,
    --      иначе дефолт. Значение согласовано с settlement.StorageSizeDefault
    --      (internal/economy/settlement/storage.go) = 1000.0. Дальше размер
    --      пересчитывает owner-проход (рост со ступенью/населением — гипотеза).
    UPDATE settlements s
    SET storage_size = COALESCE(
            (SELECT (pt.params->'storage'->>'size')::double precision
             FROM producer_types pt
             WHERE pt.id = s.settlement_type_id
               AND jsonb_typeof(pt.params->'storage'->'size') = 'number'),
            1000.0)
    WHERE s.storage_size = 0;

    -- 3) Ретайр: снять FK/индексы, переименовать (данные для отката).
    ALTER TABLE settlement_branch_buffers
        DROP CONSTRAINT IF EXISTS settlement_branch_buffers_branch_id_fkey;
    ALTER TABLE settlement_branch_buffers
        DROP CONSTRAINT IF EXISTS settlement_branch_buffers_good_id_fkey;
    DROP INDEX IF EXISTS idx_settlement_branch_buffers_branch;
    DROP INDEX IF EXISTS idx_settlement_branch_buffers_good;
    ALTER TABLE settlement_branch_buffers RENAME TO settlement_branch_buffers_retired;
END $$;

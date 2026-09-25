-- 000088_contract_author_settlement.sql
-- Автор-поселение контракта + бесплатная публикация supply (спека
-- 2026-09-25-внутреннее-хранилище-и-рождение-заказов.md §1.5/§5.2/§6, ЧК2а).
-- Номер забронирован @manager (docs/COORDINATION.md).
--
-- 1) contracts.author_type допускает 'settlement' (заказчик-поселение;
--    плательщик — владелец поселения, §1.5). direct_target_type НЕ расширяется
--    (M6): поселение — только автор, не исполнитель/прямая цель.
-- 2) contracts_live_has_escrow ослабляется: supply с нулевой наградой
--    публикуется без залога (у владельца нет денег — снабжение не встаёт, §5.2).
--    Прочие типы (travel и пр.) сохраняют прежний запрет.
--
-- Имя CHECK из 000062 — авто-имя inline-констрейнта (fact check: contracts_author_type_check,
-- contracts_live_has_escrow). DROP IF EXISTS + ADD CONSTRAINT — идемпотентно
-- при повторном прогоне файла.

ALTER TABLE contracts DROP CONSTRAINT IF EXISTS contracts_author_type_check;
ALTER TABLE contracts ADD CONSTRAINT contracts_author_type_check
    CHECK (author_type IN ('player', 'faction', 'building', 'agent', 'settlement'));

ALTER TABLE contracts DROP CONSTRAINT IF EXISTS contracts_live_has_escrow;
ALTER TABLE contracts ADD CONSTRAINT contracts_live_has_escrow
    CHECK (status NOT IN ('open','taken') OR escrow_amount > 0
           OR (type = 'supply' AND reward = 0));

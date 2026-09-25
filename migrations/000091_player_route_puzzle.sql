-- 000091_player_route_puzzle.sql
-- Состояние сегментной задачи мини-игры «Прокладка маршрута» (модель v9
-- «Планшет»), спека 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md
-- §14.1: одна строка на пару (игрок, вид игры kind). Хранит привязку к
-- сегменту перелёта (segment_hash), детерминированное поле (secret/layout,
-- клиенту не отдаются), вскрытые секторы (revealed) и остаток импульсов
-- разведки (pings_left). Владелец — игрок; расширяемость — kind + объекты в
-- JSONB. Живёт при сегменте player_flights, переживает рестарт.
--
-- НЕ входит в truncateTables (состояние игрока, переживает очистку вселенной;
-- FK → users, как player_accelerator/player_cargo).
--
-- Идемпотентно (CREATE TABLE IF NOT EXISTS).
--
-- НОМЕР: 000091 свободен на момент написания (последний в дереве — 000090).

CREATE TABLE IF NOT EXISTS player_route_puzzle (
    user_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    segment_hash BYTEA NOT NULL,
    secret       BYTEA NOT NULL,
    layout       JSONB NOT NULL,
    revealed     JSONB NOT NULL DEFAULT '[]'::jsonb,
    pings_left   INT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, kind)
);

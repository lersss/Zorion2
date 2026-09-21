-- 000065_belt_flight.sql (номер переназначен менеджером 2026-09-22: 000063/000064
-- заняты линией «ветки поселений»; исходно решение О-1 давало 000063)
-- Этап 2 поясов: пояс — цель внутрисистемного полёта (object_type='belt').
-- users.current_position / users.pending_destination — JSONB без CHECK (не требуют
-- миграции); ограничение только у player_intrasystem_flights (000046).
ALTER TABLE player_intrasystem_flights
    DROP CONSTRAINT IF EXISTS player_intrasystem_flights_from_type_check,
    DROP CONSTRAINT IF EXISTS player_intrasystem_flights_to_type_check;
ALTER TABLE player_intrasystem_flights
    ADD CONSTRAINT player_intrasystem_flights_from_type_check
        CHECK (from_type IN ('star','planet','satellite','belt')),
    ADD CONSTRAINT player_intrasystem_flights_to_type_check
        CHECK (to_type   IN ('star','planet','satellite','belt'));

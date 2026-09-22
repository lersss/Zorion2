-- 000070_supply_effects.sql — эффекты снабжения: универсальная модель нагрузки +
-- первый эффект «Голод» (спека 2026-09-22-эффекты-снабжения-задержка-голод §3.1).
--
-- Номер: спека бронирует 000070. Проверено 2026-09-23 в рабочем дереве:
-- 000069_belt_mining.sql и 000071_contract_board.sql заняты — 000070 свободен.
--
-- Справочник типов эффектов. Сила (кривая) и скорость восстановления —
-- НЕ в БД, а в store «Балансировки» (§7): params.curve — ССЫЛКА на компоненту;
-- recovery — скаляр компоненты (заводское 0.25, §7.1/§7.4), решение создателя
-- 2026-09-23 (О5 → вариант B).
CREATE TABLE IF NOT EXISTS effect_types (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT   NOT NULL,
    name_norm  TEXT   NOT NULL UNIQUE,
    impact     TEXT   NOT NULL,             -- вид воздействия, ОТКРЫТЫЙ; сейчас 'population_rate'
    params     JSONB  NOT NULL DEFAULT '{}',-- {'curve': '<компонента Балансировки>'}
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- recovery больше НЕ в params (скаляр живёт в Балансировке, §7.1).
INSERT INTO effect_types (name, name_norm, impact, params)
VALUES ('Голод', 'голод', 'population_rate', '{"curve": "hunger"}')
ON CONFLICT (name_norm) DO NOTHING;

-- Пилотная привязка (иначе пилот — no-op, §7.5): тип поселения получает
-- params.effects и params.eat, ключ — ПОЗИЦИЯ корзины (name_norm категории
-- ЛЮБОГО kind, §3.3). Пилот — позиция «продовольствие». Нормы per-позиция.
-- ВАЖНО (finding 1): НЕ перезаписываем объект eat целиком — `params ||
-- '{"eat": {…}}'` затирал бы нормы реализованного пилота итерации 4 («вода»,
-- «пища»); слияние идёт поуровнево (`||` с объектом-обёрткой). Следствие: пока
-- позиция «вода» не привязана в params.effects, ветка «Вода» населением не
-- потребляется (пилот — одна позиция) — норма eat.вода СОХРАНЕНА и включится
-- при привязке.
-- ВАЖНО (PITFALLS «БД и шелл»: jsonb_set НЕ создаёт промежуточный объект):
-- прежний вариант `jsonb_set(…, '{effects,продовольствие}', …, true)` при
-- отсутствии объекта `effects` — МОЛЧАЛИВЫЙ no-op (проверено на PG 16.9), и
-- пилот голода не активировался. Поэтому вложенные привязки собираются слиянием
-- `||` с объектом-обёрткой jsonb_build_object: отсутствующий `effects` создаётся,
-- а чужие ключи внутри `effects`/`eat` сохраняются. Повторный прогон
-- идемпотентен: `effects.продовольствие` перезаписывается на 'голод' (привязка
-- пилотная, §7.5 — перезапись намеренна), `eat.продовольствие` — норма пилота.
UPDATE producer_types
SET params = COALESCE(params, '{}'::jsonb)
    || jsonb_build_object(
         'effects', COALESCE(params->'effects', '{}'::jsonb) || jsonb_build_object('продовольствие', 'голод'),
         'eat',     COALESCE(params->'eat', '{}'::jsonb)     || jsonb_build_object('продовольствие', 2.5e-08))
WHERE name_norm = 'обычное поселение';

-- Действующие эффекты. Состояние = f(load >= порог) — НЕ хранится (производное);
-- метки прежней модели упразднены — их роль выполняет load/load_at.
-- Источник эффекта — ПОЗИЦИЯ (не ветка и не товар-выход, §4.2): хранится
-- name_norm позиции (информационно, из params.effects).
CREATE TABLE IF NOT EXISTS active_effects (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    effect_type_id   BIGINT NOT NULL REFERENCES effect_types (id) ON DELETE RESTRICT,
    owner_type       TEXT   NOT NULL,                                    -- 'settlement' сейчас
    owner_id         UUID   NOT NULL,
    owner_settlement_id UUID NULL REFERENCES settlements (id) ON DELETE CASCADE, -- якорь очистки владельца
    source_position  TEXT   NULL,                                        -- name_norm позиции (информационно)
    load             DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (load >= 0),  -- НАГРУЗКА
    load_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),                     -- базис нагрузки
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_type, owner_id, effect_type_id),
    CHECK (owner_type <> 'settlement' OR owner_settlement_id = owner_id)
);
CREATE INDEX IF NOT EXISTS idx_active_effects_owner  ON active_effects (owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_active_effects_settle ON active_effects (owner_settlement_id);
CREATE INDEX IF NOT EXISTS idx_active_effects_type   ON active_effects (effect_type_id);

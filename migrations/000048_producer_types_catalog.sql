-- Каталог типов производителей и предметов (спека 2026-09-20-фабрики §3.1):
-- producer_types (справочник типов производителей), items (справочник ТИПОВ
-- предметов — «что бывает»; экземпляры живут в инвентаре, не здесь),
-- producer_items (связь «производитель предметов ↔ предметы»).
-- BIGSERIAL-ключи, единообразно с 000045 (categories/goods/goods_slots).
-- Каталог — контент, не данные вселенной: ClearUniverse его не трогает.
-- name_norm — обычная колонка (НЕ generated): значение пишет приложение
-- (graph.NormalizeName, Unicode ToLower), как в 000045 (BUG-1, @tester iterA).
--
-- Дельта goods (решение 3b.6.4): volume/weight — данные каталога (механика
-- грузов/трюма — будущая фича); approved-товар без веса/объёма не проходит
-- валидацию (NULL-каталог запрещён, проверка в студии).

CREATE TABLE producer_types (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    name_norm   TEXT NOT NULL,             -- graph.NormalizeName(name) из Go
    kind        TEXT NOT NULL CHECK (kind IN ('goods', 'items', 'energy')),
    category_id BIGINT NULL REFERENCES categories (id),  -- kind=goods: категория товаров/ресурсов
    race_family TEXT NULL,                 -- семейство рас (F1–F10) или NULL = универсальный
    output      JSONB NULL,                -- спецификация выхода (items/energy; goods — категория)
    input       JSONB NULL,                -- спецификация входа/снабжения (люди, энергия, расходники)
    params      JSONB NULL,                -- эффективность, ёмкость населения, содержание, цена
    status      TEXT NOT NULL DEFAULT 'draft'
                CHECK (status IN ('draft', 'approved', 'excluded', 'banned')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name_norm)
);

CREATE TABLE items (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    name_norm  TEXT NOT NULL,              -- graph.NormalizeName(name) из Go
    slot_type  TEXT NOT NULL,              -- чертёж/модуль/инструмент/сертификат
    status     TEXT NOT NULL DEFAULT 'draft'
               CHECK (status IN ('draft', 'approved', 'excluded', 'banned')),
    unlocks    JSONB NULL,                 -- предмет-рецепт: [{producer_type_id, category_id}]
    params     JSONB NULL,                 -- характеристики (модуль корабля — объект по чертежу)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name_norm)
);

CREATE TABLE producer_items (
    producer_type_id BIGINT NOT NULL REFERENCES producer_types (id) ON DELETE CASCADE,
    item_id          BIGINT NOT NULL REFERENCES items (id) ON DELETE CASCADE,
    requirements     JSONB NULL,           -- расходники/энергия/люди на производство предмета
    PRIMARY KEY (producer_type_id, item_id)
);

ALTER TABLE goods ADD COLUMN volume DOUBLE PRECISION NULL;
ALTER TABLE goods ADD COLUMN weight DOUBLE PRECISION NULL;
-- Рецепт как сущность (спека 2026-09-21-рецепт-сущность-и-граф-фабрики §3,
-- редакция 4; идея 2026-09-21_фабрика-центричный-граф-товары-под-фабрику §1).
-- Состав переезжает из goods_slots в recipes + recipe_components; связь
-- «фабрика ↔ рецепты» — producer_recipes (M:N); сложность — свойство рецепта
-- (goods.tier_override снимается). Каталог — контент: ClearUniverse таблицы
-- не трогает (в truncateTables admin_universe.go их нет).
--
-- Порядок: создание таблиц → перенос данных → снос старой структуры.

-- 1) Таблицы рецептов (§2.3). BIGSERIAL-ключи и name_norm-конвенция — как у
--    000045/000048/000052.
CREATE TABLE recipes (
    id         BIGSERIAL PRIMARY KEY,
    good_id    BIGINT NOT NULL REFERENCES goods (id) ON DELETE CASCADE,
    complexity INT NULL CHECK (complexity IS NULL OR complexity >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (good_id)                      -- один рецепт на товар (снятие — альтернативные рецепты)
);

CREATE TABLE recipe_components (
    id             BIGSERIAL PRIMARY KEY,
    recipe_id      BIGINT NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    pos            INT NOT NULL,          -- порядок компонента (0-based)
    component_id   BIGINT NULL REFERENCES goods (id) ON DELETE SET NULL,  -- NULL = пустой компонент
    quantity       INT NOT NULL DEFAULT 1 CHECK (quantity >= 1),
    reason         TEXT NULL,             -- ИИ-обоснование (тултип)
    allow_resource BOOLEAN NOT NULL DEFAULT false,  -- подсказка ИИ для пустого компонента (§2.5)
    UNIQUE (recipe_id, pos)
);
CREATE INDEX idx_recipe_components_component ON recipe_components (component_id);

CREATE TABLE producer_recipes (
    producer_type_id BIGINT NOT NULL REFERENCES producer_types (id) ON DELETE CASCADE,
    recipe_id        BIGINT NOT NULL REFERENCES recipes (id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (producer_type_id, recipe_id)   -- повторная привязка — 409
);
CREATE INDEX idx_producer_recipes_recipe ON producer_recipes (recipe_id);

-- 2) Рецепт — на каждый товар (kind='good'); сложность = старый tier_override.
INSERT INTO recipes (good_id, complexity)
SELECT id, tier_override FROM goods WHERE kind = 'good';

-- 3) Состав — из goods_slots.
INSERT INTO recipe_components (recipe_id, pos, component_id, quantity, reason, allow_resource)
SELECT r.id, s.pos, s.component_id, s.quantity, s.reason, s.allow_resource
FROM goods_slots s JOIN recipes r ON r.good_id = s.good_id;

-- 4) Привязка: ко ВСЕМ универсальным конкретным фабрикам категории товара
--    (kind=goods, parent_id NOT NULL, category_id = категория товара,
--    race_family IS NULL И race IS NULL). «Ничей» товар (нет универсальной
--    дочки категории) остаётся без привязок — правится через справочник.
INSERT INTO producer_recipes (producer_type_id, recipe_id)
SELECT pt.id, r.id
FROM recipes r
JOIN goods g ON g.id = r.good_id
JOIN producer_types pt
  ON pt.kind = 'goods' AND pt.parent_id IS NOT NULL AND pt.category_id = g.category_id
 AND pt.race_family IS NULL AND pt.race IS NULL;

-- 5) Снос старой структуры.
ALTER TABLE goods DROP COLUMN tier_override;
DROP TABLE goods_slots;

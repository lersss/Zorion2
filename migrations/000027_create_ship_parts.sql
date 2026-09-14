-- Каталог деталей кораблей (спека 99.2.15 §2): «теневой» генератор в админке
-- нагенерирует детали, из которых агенты/игроки собирают схемы кораблей.
-- id = {category}_{slug(params)} — детерминизм: та же генерация с тем же seed
-- даёт тот же id (INSERT ... ON CONFLICT DO UPDATE при перегенерации категории).
-- CHECK на категории — как users_role_check в 99.2.14 §2 (дешевле кода);
-- список категорий — константа в одном месте (категории открыты, FAQ В1).
CREATE TABLE IF NOT EXISTS ship_parts (
    id         TEXT PRIMARY KEY,          -- "hull_arrow", "nose_spike", ...
    category   TEXT NOT NULL CHECK (category IN ('hull','nose','wings','engine','tail')),
    name       TEXT NOT NULL,             -- человеческое имя ("Стрела", "Клин")
    svg        TEXT NOT NULL,             -- SVG-фрагмент (без <svg>-обёртки), viewBox 200×200
    params     JSONB NOT NULL DEFAULT '{}', -- параметры генерации (для перегенерации)
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ship_parts_category ON ship_parts (category);
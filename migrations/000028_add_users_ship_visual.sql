-- Схема корабля игрока (спека 99.2.15 §2): {"color": "#3b82f6",
-- "parts": {"hull": "hull_arrow", "nose": "nose_spike", ...}}.
-- NULL = «ещё не собирал»: рендер и /me отдают дефолтную сборку от
-- (id игрока, legacy ship_icon) — см. спека §8 (мост).
-- ship_icon не удаляется (мост, спека §8); новой логикой не пишется.
ALTER TABLE users ADD COLUMN IF NOT EXISTS ship_visual JSONB;
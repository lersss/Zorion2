-- Реестр конфигов генерации (спека 99.2.3 §3):
-- паттерн «JSON-дефолты в коде + override-таблица в БД» (как матрица совместимости).
-- Ключи: 'star_weights' (спектральные классы + типы систем/объектов),
--        'planet_means' (среднее число планет по типу звезды).
-- Расширяется новыми ключами без миграций схемы.

CREATE TABLE generation_config (
    key        TEXT PRIMARY KEY,
    payload    JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE generation_config IS 'Реестр конфигов генерации (99.2.3): star_weights, planet_means';
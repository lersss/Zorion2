-- 77a: модель корабля, оборудование и радар — видимость игрока (спека 77a §12).
-- Три сущности: справочник моделей кораблей (рамка слотов), справочник
-- оборудования (радар/сканер/двигатель), личный каталог знания о планетах
-- (на дату, протухание 7 дней — статус на чтении, без фоновых джобов).

-- Справочник моделей кораблей
CREATE TABLE ship_models (
    id         TEXT PRIMARY KEY,          -- 'starter'
    name       TEXT NOT NULL,
    slots      JSONB NOT NULL,            -- {"radar":1,"scanner":1,"engine":1}
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Справочник оборудования
CREATE TABLE equipment (
    id         TEXT PRIMARY KEY,          -- 'radar_1', 'scanner_1' (одна модель радара; уровни — будущее с рынками)
    type       TEXT NOT NULL CHECK (type IN ('radar','scanner','engine')),
    name       TEXT NOT NULL,
    params     JSONB NOT NULL,            -- радар: {"radius":400}; сканер: {"depth":"surface","settlements":true}
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Игрок → модель и установленное оборудование
ALTER TABLE users ADD COLUMN ship_model_id TEXT REFERENCES ship_models(id);
ALTER TABLE users ADD COLUMN equipment JSONB;  -- {"radar":"radar_1","scanner":"scanner_1","engine":null}

-- Личный каталог знания о планетах (на дату)
CREATE TABLE player_planet_knowledge (
    user_id    UUID NOT NULL REFERENCES users(id),
    planet_id  UUID NOT NULL REFERENCES planets(id),
    data       JSONB NOT NULL,            -- поверхность, наличие поселений, источник
    scanned_at TIMESTAMPTZ NOT NULL,
    source     TEXT NOT NULL DEFAULT 'scanner',  -- 'scanner' | 'report' (задел)
    PRIMARY KEY (user_id, planet_id)
);

-- Бутстрап: стартовая модель и стартовая комплектация (спека 77a §3.3).
INSERT INTO ship_models (id, name, slots) VALUES
    ('starter', 'Стартовый разведчик', '{"radar":1,"scanner":1,"engine":1}');

INSERT INTO equipment (id, type, name, params) VALUES
    ('radar_1',   'radar',   'Радар-1',   '{"radius":400}'),
    ('scanner_1', 'scanner', 'Сканер-1',  '{"depth":"surface","settlements":true}');

-- Бэкфилл существующих игроков: стартовая модель + радар-1 + сканер-1
-- (как дефолты 61b при регистрации).
UPDATE users SET
    ship_model_id = 'starter',
    equipment = '{"radar":"radar_1","scanner":"scanner_1","engine":null}'
WHERE ship_model_id IS NULL;
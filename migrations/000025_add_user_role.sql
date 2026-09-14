-- Роль пользователя (спека 99.2.14 §2). Существующие учётки получают
-- 'player' — до фичи ролей не было, никто не админ. CHECK защищает от
-- опечаток в значениях роли. Индекс по role не нужен: таблица users малая.
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'player';
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('player', 'admin', 'skycomposer'));
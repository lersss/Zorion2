-- Профиль региона (59a, спека 99.2.10 §10): колонки regions.profile
-- (ключ класса из config/region_profiles/) + regions.profile_intensity
-- (0/1/2: слабая/средняя/сильная). Пустой profile — фоновый регион
-- (~25%, спека §10). В финале профиль не публикуется (не ярлык, §11.7);
-- ОТЛАДОЧНО (59a) колонка profile выводится в /api/regions — убрать в финале.

ALTER TABLE regions ADD COLUMN profile TEXT;
ALTER TABLE regions ADD COLUMN profile_intensity SMALLINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN regions.profile IS 'Ключ класса профиля региона (59a); NULL/пусто — фоновый регион';
COMMENT ON COLUMN regions.profile_intensity IS 'Интенсивность профиля 0/1/2: слабая/средняя/сильная (59a)';
-- B20: код формы поверхности «скалы» переименован в «горы».
-- Данные уже сгенерированных планет содержат старый код — миграция
-- переписывает его на верхнем уровне planets.data и у спутников
-- внутри data.satellites: и ключ compose в surface_composition, и
-- значение surface_dominant. Прозаические описания не затрагиваются.

-- 1. Верхний уровень: ключ surface_composition.скалы → горы.
UPDATE planets
SET data = jsonb_set(
    data,
    '{surface_composition,горы}',
    data->'surface_composition'->'скалы',
    true
) #- '{surface_composition,скалы}'
WHERE data->'surface_composition' ? 'скалы';

-- 2. Верхний уровень: surface_dominant «скалы» → «горы».
UPDATE planets
SET data = jsonb_set(data, '{surface_dominant}', '"горы"', false)
WHERE data->>'surface_dominant' = 'скалы';

-- 3. Спутники: ключ surface_composition[i].скалы → горы.
UPDATE planets
SET data = jsonb_set(
    data,
    '{satellites}',
    (
        SELECT jsonb_agg(
            CASE WHEN s->'surface_composition' ? 'скалы' THEN
                jsonb_set(
                    s #- '{surface_composition,скалы}',
                    '{surface_composition,горы}',
                    s->'surface_composition'->'скалы',
                    true
                )
            ELSE s END
        )
        FROM jsonb_array_elements(data->'satellites') AS s
    ),
    false
)
WHERE EXISTS (
    SELECT 1 FROM jsonb_array_elements(data->'satellites') AS s
    WHERE s->'surface_composition' ? 'скалы'
);

-- 4. Спутники: surface_dominant[i] «скалы» → «горы».
UPDATE planets
SET data = jsonb_set(
    data,
    '{satellites}',
    (
        SELECT jsonb_agg(
            CASE WHEN s->>'surface_dominant' = 'скалы' THEN
                jsonb_set(s, '{surface_dominant}', '"горы"', false)
            ELSE s END
        )
        FROM jsonb_array_elements(data->'satellites') AS s
    ),
    false
)
WHERE EXISTS (
    SELECT 1 FROM jsonb_array_elements(data->'satellites') AS s
    WHERE s->>'surface_dominant' = 'скалы'
);
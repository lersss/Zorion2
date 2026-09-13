// web/static/js/admin/hypothesisPresets.js
//
// Пресеты гипотез для вкладки «Проверка гипотез» — «близнецы»: канонический
// шаблон планеты (base) клонируется N раз, в группе переопределяется ровно
// одна варьируемая ось (axis) + необходимые для консистентности поля
// (overrides группы). Всё остальное — побайтово одинаково, дельта
// атрибутируется одной оси. На группу — одна звезда; звезды двух групп стоят
// визуально рядом.
//
// Население по умолчанию — фиксированное (иначе планеты группы получают
// разный старт и перестают быть «близнецами»); шанс заселения 100% (все
// планеты заселены). В UI население можно переключить на случайное.
//
// axis.type: 'number' | 'string' | 'bool' | null (null — варьируется только
// стартовое население поселения, планеты групп идентичны).

// CANONICAL — землеподобный мир (умеренный, океаны, жизнь). База многих
// гипотез; аудитом не флагается.
const CANONICAL = {
    size: 1.0,
    mass: 1.0,
    density: 1.0,
    gravity: 1.0,
    temperature: 288,
    water_percent: 70,
    atmosphere: 'азотно-кислородная',
    hydrosphere: 'океаны',
    biosphere: 'растительная',
    life: true,
    political_system: 'демократия',
    moons: 1,
    development_level: 0.5,
    archetype: 'умеренный',
    system_age: 1,
    surface_composition: { океаны: 60, скалы: 25, леса: 15 },
    subterrain_composition: {
        пустые_породы: 30, осадочные_породы: 25, рудные_жилы: 20,
        грунтовые_воды: 15, магматические_породы: 10,
    },
    surface_dominant: 'океаны',
    type: 'землеподобная',
    core: {
        type: 'металлическое', mass_percent: 30, activity: 60,
        radioactivity: 20, age: 1, is_active: true, is_metallic: true,
    },
};

// BARREN — бесплодная каменная планета (нет воды/жизни): температура меняется
// без противоречий — основа для гипотез оси «температура».
const BARREN = {
    size: 0.9,
    mass: 0.8,
    density: 1.0,
    gravity: 0.99,
    temperature: 280,
    water_percent: 0,
    atmosphere: 'разреженная',
    hydrosphere: 'сухая',
    biosphere: 'стерильная',
    life: false,
    political_system: 'нет',
    moons: 1,
    development_level: 0.2,
    archetype: 'изменчивый',
    system_age: 1,
    surface_composition: { скалы: 80, пески_пустыни: 20 },
    subterrain_composition: { пустые_породы: 60, магматические_породы: 40 },
    surface_dominant: 'скалы',
    type: 'скалистая (базовая)',
    core: {
        type: 'силикатное', mass_percent: 25, activity: 20,
        radioactivity: 10, age: 1, is_active: false, is_metallic: false,
    },
};

// COLD — ледяной мир: основа гипотезы оси «гидросфера льда».
const COLD = {
    size: 0.8,
    mass: 0.5,
    density: 0.8,
    gravity: 0.98,
    temperature: 120,
    water_percent: 5,
    atmosphere: 'разреженная',
    hydrosphere: 'ледяной покров',
    biosphere: 'стерильная',
    life: false,
    political_system: 'нет',
    moons: 0,
    development_level: 0.2,
    archetype: 'холодный',
    system_age: 1,
    surface_composition: { ледники: 70, мёрзлые_газы: 30 },
    subterrain_composition: { подземные_льды: 60, пустые_породы: 40 },
    surface_dominant: 'ледники',
    type: 'ледяная',
    core: {
        type: 'ледяное', mass_percent: 20, activity: 5,
        radioactivity: 5, age: 1, is_active: false, is_metallic: false,
    },
};

// GIANT — газовый гигант: основа гипотезы «верхний потолок = 0».
const GIANT = {
    size: 11.2,
    mass: 317.8,
    density: 0.23,
    gravity: 2.53,
    temperature: 150,
    water_percent: 0,
    atmosphere: 'водородно-гелиевая',
    hydrosphere: 'сухая',
    biosphere: 'стерильная',
    life: false,
    political_system: 'нет',
    moons: 8,
    development_level: 0,
    archetype: 'жаркий',
    system_age: 1,
    is_gas_giant: true,
    surface_dominant: 'газовый_гигант',
    type: 'газовый гигант',
    core: {
        type: 'металлическое', mass_percent: 20, activity: 80,
        radioactivity: 10, age: 1, is_active: true, is_metallic: true,
    },
};

// fixedPop — население по умолчанию: фиксированное (близнецы идентичны).
function fixedPop(v) {
    return { kind: 'fixed', fixed: v };
}

export const HYPOTHESIS_PRESETS = [
    // Контроль: сад стабилен (вода/тепло/воздух → дельта ≥ 0).
    {
        id: 'greenbelt',
        name: 'Зелёный пояс',
        hypothesis: 'Контроль: нижний потолок высок (вода, тепло, чистый воздух) → дельта ≥ 0 без игрока. Сад стабилен.',
        axis: { key: 'water_percent', label: 'Вода', unit: '%', type: 'number' },
        base: { ...CANONICAL },
        groups: [
            {
                id: 'сад', name: 'Сад', axisValue: 70, overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(100000000),
            },
            {
                id: 'сухость', name: 'Сухость', axisValue: 2,
                overrides: {
                    surface_composition: { скалы: 70, пески_пустыни: 30 },
                    surface_dominant: 'скалы',
                    type: 'скалистая (базовая)',
                },
                planetsPerWorld: 1, chance: 100, population: fixedPop(100000000),
            },
        ],
    },
    // Ось гравитация (через массу): тяжесть модифицирует аграрную ёмкость.
    {
        id: 'dugard_milliard',
        name: 'Аккуратный миллиард',
        hypothesis: 'Ось гравитация: зафиксирована пригодная температура, варьируется g → тяжесть модифицирует аграрную ёмкость. Гравитация — производная массы, варьируем mass (размер и g пересчитаны консистентно).',
        axis: { key: 'mass', label: 'Масса', unit: 'M⊕', type: 'number' },
        base: { ...CANONICAL },
        groups: [
            {
                id: 'тяжёлые', name: 'Тяжёлые', axisValue: 8,
                overrides: { mass: 8, size: 2.0, gravity: 2.0 },
                planetsPerWorld: 1, chance: 100, population: fixedPop(200000000),
            },
            {
                id: 'лёгкие', name: 'Лёгкие', axisValue: 0.3,
                overrides: { mass: 0.3, size: 0.67, gravity: 0.67 },
                planetsPerWorld: 1, chance: 100, population: fixedPop(200000000),
            },
        ],
    },
    // Ось атмосфера: ядовитый воздух при уютной температуре.
    {
        id: 'toxin_front',
        name: 'Токсин-фронт',
        hypothesis: 'Ось атмосфера: ядовитый воздух при уютной температуре → корма нет от газа, не от жары; дельта < 0 без поставок.',
        axis: { key: 'atmosphere', label: 'Атмосфера', unit: '', type: 'string' },
        base: { ...CANONICAL },
        groups: [
            {
                id: 'ядовитая', name: 'Ядовитая', axisValue: 'ядовитая', overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(100000000),
            },
            {
                id: 'чистая', name: 'Чистая', axisValue: 'азотно-кислородная', overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(100000000),
            },
        ],
    },
    // Сходимость сверху: старт выше потолка. Варьируется стартовое население.
    {
        id: 'requiem',
        name: 'Реквием по пригодности',
        hypothesis: 'Сходимость сверху: старт выше потолка (плотные мегаколонии) → дельта < 0 до асимптоты = аграрная ёмкость. Планеты групп идентичны, различается старт населения.',
        axis: null,
        base: { ...CANONICAL },
        groups: [
            {
                id: 'мегаколонии', name: 'Мегаколонии', axisValue: null, overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(1000000000),
            },
            {
                id: 'обычные', name: 'Обычные', axisValue: null, overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(500000),
            },
        ],
    },
    // Рост форпоста: старт мал → проверка перетока. Варьируется старт.
    {
        id: 'seeder',
        name: 'Сеятель-сеть',
        hypothesis: 'Рост форпоста: старт мал → проверка перетока; пригодные миры растут медленно, фронтир — только поставкой. Планеты идентичны, различается старт населения.',
        axis: null,
        base: { ...CANONICAL },
        groups: [
            {
                id: 'форпост', name: 'Форпост', axisValue: null, overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(50000),
            },
            {
                id: 'город', name: 'Город', axisValue: null, overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(5000000),
            },
        ],
    },
    // Ось жара: на бесплодной планете меняется только температура.
    {
        id: 'forges',
        name: 'Перенаселённые кузни',
        hypothesis: 'Ось жара: temp ≥ 127 °C → корма нет от перегрева; население живёт только поставкой (верхний потолок). База — бесплодная (нет воды/жизни), меняется только температура.',
        axis: { key: 'temperature', label: 'Температура', unit: '°C', type: 'number' },
        base: { ...BARREN },
        groups: [
            {
                id: 'жара', name: 'Жара', axisValue: 500, overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(500000000),
            },
            {
                id: 'комфорт', name: 'Комфорт', axisValue: 280, overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(500000000),
            },
        ],
    },
    // Ось гидросфера льда: подлёдные океаны vs ледяной панцирь.
    {
        id: 'cold_will',
        name: 'Ледяная холодная воля',
        hypothesis: 'Ось гидросфера льда: холодный пояс (подлёдные океаны vs ледяной покров) → воду даёт среда, а не игрок.',
        axis: { key: 'hydrosphere', label: 'Гидросфера', unit: '', type: 'string' },
        base: { ...COLD },
        groups: [
            {
                id: 'подлёдная', name: 'Подлёдные океаны', axisValue: 'подлёдная',
                overrides: {
                    water_percent: 60,
                    surface_composition: { ледники: 70, мёрзлые_газы: 20, озёра_реки: 10 },
                },
                planetsPerWorld: 1, chance: 100, population: fixedPop(50000000),
            },
            {
                id: 'панцирь', name: 'Ледяной панцирь', axisValue: 'ледяной покров',
                overrides: { water_percent: 5 },
                planetsPerWorld: 1, chance: 100, population: fixedPop(50000000),
            },
        ],
    },
    // Ось биосфера: растительная жизнь поднимает нижний потолок.
    {
        id: 'only_green',
        name: 'Только зелёные',
        hypothesis: 'Ось биосфера: растительная жизнь поднимает нижний потолок; сравнение зелёных миров с контролем greenbelt.',
        axis: { key: 'biosphere', label: 'Биосфера', unit: '', type: 'string' },
        base: { ...CANONICAL },
        groups: [
            {
                id: 'растительная', name: 'Растительная', axisValue: 'растительная', overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(1000000000),
            },
            {
                id: 'стерильная', name: 'Стерильная', axisValue: 'стерильная',
                overrides: {
                    life: false,
                    surface_composition: { скалы: 70, пески_пустыни: 30 },
                    surface_dominant: 'скалы',
                    type: 'скалистая (базовая)',
                },
                planetsPerWorld: 1, chance: 100, population: fixedPop(1000000000),
            },
        ],
    },
    // Два безкормных края: лёд vs лава (температура + поверхность).
    {
        id: 'stone_belts',
        name: 'Каменные пояса',
        hypothesis: 'Сравнение двух безкормных краёв (лёд и лава): разные скорости гибели без поставок. Близнецы бесплодной базы с разной температурой и поверхностью.',
        axis: { key: 'temperature', label: 'Температура', unit: '°C', type: 'number' },
        base: { ...BARREN },
        groups: [
            {
                id: 'лёд', name: 'Ледяной пояс', axisValue: 100,
                overrides: {
                    surface_composition: { ледники: 80, мёрзлые_газы: 20 },
                    surface_dominant: 'ледники',
                    type: 'ледяная',
                    subterrain_composition: { подземные_льды: 60, пустые_породы: 40 },
                },
                planetsPerWorld: 1, chance: 100, population: fixedPop(30000000),
            },
            {
                id: 'лава', name: 'Лавовый пояс', axisValue: 700,
                overrides: {
                    surface_composition: { лавовые_поля: 70, вулканические_поля: 30 },
                    surface_dominant: 'лавовые_поля',
                    type: 'вулканическая',
                    subterrain_composition: { магматические_камеры: 60, магматические_породы: 40 },
                },
                planetsPerWorld: 1, chance: 100, population: fixedPop(30000000),
            },
        ],
    },
    // Чистый верхний потолок: газовый гигант vs твёрдый мир.
    {
        id: 'orbital_docks',
        name: 'Орбитальные доки',
        hypothesis: 'Чистый верхний потолок: у газовых гигантов нижний потолок = 0 → без поставок население сходится к нулю.',
        axis: { key: 'is_gas_giant', label: 'Газовый гигант', unit: '', type: 'bool' },
        base: { ...GIANT },
        groups: [
            {
                id: 'гиганты', name: 'Газовые гиганты', axisValue: true, overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(500000000),
            },
            {
                id: 'твёрдые', name: 'Твёрдые миры', axisValue: false,
                overrides: {
                    size: 1.0, mass: 1.0, density: 1.0, gravity: 1.0,
                    atmosphere: 'азотно-кислородная',
                    surface_composition: { скалы: 70, пески_пустыни: 30 },
                    surface_dominant: 'скалы',
                    type: 'скалистая (базовая)',
                },
                planetsPerWorld: 1, chance: 100, population: fixedPop(500000000),
            },
        ],
    },
    // Ось экзобиология: грибная/светящаяся биосфера.
    {
        id: 'biosphere_exotic',
        name: 'Споровый рукав',
        hypothesis: 'Ось экзобиология: грибная и светящаяся биосфера → корм есть, но другой состав; дельта живучести отличается от растительной.',
        axis: { key: 'biosphere', label: 'Биосфера', unit: '', type: 'string' },
        base: { ...CANONICAL },
        groups: [
            {
                id: 'экзотика', name: 'Грибная / светящаяся', axisValue: 'грибная', overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(50000000),
            },
            {
                id: 'растительная', name: 'Растительная', axisValue: 'растительная', overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(50000000),
            },
        ],
    },
    // Ось возраста: молодые системы держат дольше старых.
    {
        id: 'young_worlds',
        name: 'Горячие ядра',
        hypothesis: 'Гипотеза возраста: возраст системы — скрытый запас энергии (геотерма), должен продлевать жизнь поселения на любом, даже холодном, мире. Молодые миры обязаны держаться дольше старых при прочих равных.',
        axis: { key: 'system_age', label: 'Возраст системы', unit: 'млрд лет', type: 'number' },
        base: { ...CANONICAL },
        groups: [
            {
                id: 'young', name: 'Молодые', axisValue: 1, overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(100000000),
            },
            {
                id: 'old', name: 'Старые', axisValue: 10, overrides: {},
                planetsPerWorld: 1, chance: 100, population: fixedPop(100000000),
            },
        ],
    },
];
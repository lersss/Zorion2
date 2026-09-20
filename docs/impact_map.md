# Реестр каскадных влияний — индекс

> Компактный обзор docs/impact_map.json (52 сущности, 315 связей, ~90 КБ — целиком НЕ читать). Вопрос реестра: «меняем X → смотрим на Y». Ведёт @manager при сдаче фич (идея 83a); валидатор — scripts/check_impact_map.ps1 после каждого коммита.

**Как читать:** нужна сущность — ищи её id/path в docs/impact_map.json через grep (одна сущность + её impacts[], ~1–3 КБ), целиком json не читай. Связанные сущности — по on-ссылкам (id или путь).

| id | path | что это |
|---|---|---|
| race_lore.json | config/race_lore.json | лор рас (86a §5.1.1): старые поля character/how_live/why/coexistence/origin + новые поля карточки 99.2.26 (kind/niche/size_indi… |
| races.json | config/races.json | каталог 60 рас (50 био + 10 роботов): окна opt/surv, атрибуты, потребление 13 осей, forage, home, bulge |
| race_balancer.json | config/race_balancer.json | R-кривые рас: factory (снимок из карточки) + active (настроенная), card_hash; env RACE_BALANCER_FILE |
| settlements.race_id | db:settlements | колонка settlements.race_id (000039) + regions.race_id (000038); NULL = легаси/люди; две расы на планете = два ряда |
| 22_races.md | docs/gamedesign/22_races.md | GDD-справочник рас (лор); источник фактов — config/races.json и спеки 99.2.21–99.2.24 |
| 18b_settlement_log.md | docs/gamedesign/18b_settlement_log.md | GDD-док «Лог поселения» (записи «Вымерло», таблица settlement_log, миграция 000024); вынесен из 18a_population_death.md 2026-09… |
| 18a_population_death.md | docs/gamedesign/18a_population_death.md | GDD-док «Смертность населения от среды»: R-модель (99.2.13), температурный профиль (99.2.12), рождаемость (99.2.16); «Лог посел… |
| settlement_log | db:settlement_log | таблица лога поселения (000024): settlement_id, death_at, cause, population_exact; uq_settlement_log_extinct — анти-дубль; бэкф… |
| 99.2.21 | docs/specs/_archive/99.2.21-races-v1.md | спека «Расы» + строка роадмапа §99.2.21 (✅ Реализовано 2026-09-17) |
| 99_roadmap.md | docs/gamedesign/99_roadmap.md | горячий док: статусы фич §99.2; ведёт @dispatcher по дельтам менеджера (строку добавляет менеджер) |
| CHANGELOG.md | CHANGELOG.md | горячий док: история релизов/фич; ведёт @dispatcher по дельтам менеджера |
| 77a | docs/specs/77a-ship-equipment-radar.md | спека «Модель корабля, оборудование и радар — видимость игрока» (77a, 2026-09-17); три сущности: ship_models, equipment, player… |
| ship_models | db:ship_models | справочник моделей кораблей (id, name, slots JSONB); users.ship_model_id → ship_models.id (000040) |
| equipment | db:equipment | справочник оборудования (radar/scanner/engine, params JSONB); users.equipment → справочник (000040/000042) |
| player_planet_knowledge | db:player_planet_knowledge | личный каталог знания о планетах (user_id, planet_id, data, scanned_at, source; PK user+planet; протухание 7 дней — статус на ч… |
| 88a.1 | docs/specs/_archive/88a.1-art-studio-water-f1.md | спека арт-студии: семейство F1 «Водные» (расы 2–4), люди → F0 (88a, 2026-09-17) |
| 94a | docs/specs/_archive/94a-consumption-catalog.md | спека «Каталог ресурсов + потребление рас» (94a, 2026-09-18): универсальный слой 20 ресурсов, 13 шаблонов хемотипов, мост consu… |
| 91a | docs/specs/_archive/91a-ship-section-dashboard.md | speca 91a (2026-09-18): razdel «Korabl» v dashboarde (siluet so slotami), dvigatel engine_1 kak nastoyashchiy modul (migraciya … |
| 86a | docs/specs/_archive/86a-player-dashboard-encyclopedia.md | спека «Дашборд игрока + Энциклопедия» (86a, 2026-09-17): профиль, статистика, энциклопедия (расы/звёзды/планеты/ресурсы/корабли… |
| 61b | docs/specs/_archive/61b-static-ship-sprites.md | спека «Статичные спрайты кораблей вместо генератора» (61b): сетка 21 спрайта + палитра 9 цветов + «Оригинал» (NULL); селектор в… |
| web/index.html | web/index.html | дашборд игрока: профиль, раздел «Корабль» (91a), статистика, энциклопедия (86a), селектор внешнего вида (61b); вкладки .tab-btn… |
| real.go | internal/resource/real.go | витрина 111 реальных веществ (10 осей + T_melt/T_boil, 22 семейства) для оценки ёмкости модели осей; с итерации B переноса (202… |
| distance.go | internal/resource/distance.go | проверка различимости профилей: коллизия = все 10 осей в пределах <10; T-разрешение ≥10 K; живая проверка минимального расстоян… |
| семейство (хим. группа) | web/static/js/admin/resources.js | терминологическая коллизия: «семейство» занято семействами рас F1–F10 (22_races.md, 99.2.21 §16); поле Family ресурса витрины в… |
| player_flights | db:player_flights | активный полёт игрока (миграция 000044, идея 97a): одна запись на игрока (PK user_id), from/to/start_x/start_y/start_time/arriv… |
| config/art/families.json | config/art/families.json | семейства рас для арт-студии: forms/materials/glows, морф-наборы (F1–F10); с 98a у всех 59 рас появились поля appearance (внешн… |
| config/art/forms.json | config/art/forms.json | глобальные словари форм арт-студии (character/parts/shapes/struct/морф-списки, palette_accents); с 98a пулы фильтруются blocked… |
| 98a | docs/specs/_archive/98a-внешность-рас-арт-фильтр.md | спека «Внешность рас + фильтрация шума в генераторе картинок» (98a, пилот 6 рас → расширено на все 59, контент 2026-09-20) + 98… |
| goods | db:goods (categories/goods/goods_slots) | каталог товаров/ресурсов в PostgreSQL (миграция 000045, перенос iterA): categories (товарные + 6 ресурсных системных), goods (n… |
| internal/goodsstudio | internal/goodsstudio/ | доменный пакет каталога (перенос iterA, iterC): model (Category/Good/Slot/Status/Kind/Source), graph (tier/cycles/names), valid… |
| internal/goodsstudio/ai | internal/goodsstudio/ai/ | ИИ «заполнить комплектующие» (перенос cmd/goods-studio/ai, iterC 2026-09-20): client (opencode HTTP, env OPENCODE_URL/MODEL/TIM… |
| web/static/js/auth.js | web/static/js/auth.js | общий модуль авторизации (iterC 2026-09-20, «причеши»): ядро auth.js (getToken/setToken/validateToken/login/fetchWithAuth); тон… |
| web/studio.html | web/studio.html | UI студии товаров на игровом сервере (перенос iterA/iterB/iterC): перенесённый cmd/goods-studio/web/index.html; подключён к /st… |
| nav-admin-map | web/map.html | Переходы админка ↔ карта (2026-09-19): синхронизация JWT между ключами token/adminToken при навигации; клик по 🌍 в шапке админ… |
| pacman_job | internal/handlers/admin_pacman.go | Пакман-вайп (идея 2026-09-20, спека 2026-09-20-pacman-galaxy-wipe): админ-джоб ест миры целиком по траектории nearest (жадный б… |
| races/ships/*.md | docs/gamedesign/races/ships/*.md | каталог кораблей рас (60 файлов по id расы, включая humans): внешний вид корабля (форма/материал/свечение/детали/палитра/чего н… |
| связь-с-кораблём | docs/gamedesign/ideas/2026-09-20_связь-с-кораблём-аватары-рас-канди… | механика-кандидат (2026-09-20): игрок связывается с кораблём расы (контакт/радар, 77a) и получает аватар представителя расы; ка… |
| 67a.1 | docs/specs/67a.1-art-studio-go.md | арт-студия (Go, cmd/art-studio): генерация аватаров рас 200×200, композит на кабину cockpit_1024.png (техподложка — «кабина сту… |
| users.current_position | db:users.current_position | внутрисистемная позиция игрока (000046, 99.2.27): JSONB NULL = вне системы; orbit {object_type, object_id, level, biome} / in_f… |
| player_intrasystem_flights | db:player_intrasystem_flights | активный внутрисистемный полёт (000046, 99.2.27): PK user_id, world_id, from/to type+id TEXT (синтетические id компаньонов), st… |
| IntrasystemManager | internal/travel/intrasystem_manager.go | менеджер внутрисистемных полётов (99.2.27, паттерн 97a): RWMutex-карта, CancelChan, TOCTOU-гвард удаления строки, Restore |
| players_positions_90a | internal/handlers/players_positions.go | /api/players/positions — правило 90a ИЗМЕНЕНО (99.2.27, решение создателя): показываются не только летящие, но и стоящие на орб… |
| 99.2.27 | docs/specs/99.2.27-intrasystem-flight.md | спека «Внутрисистемный полёт» (99.2.27, 2026-09-20, сдана «доделаем потом»): позиция (users.current_position), player_intrasyst… |
| biome_catalog.json | config/biome_catalog.json | справочник биомов (99.2.28, 2026-09-20): 57 биомов поверхности, 17 типов недр, 9 правил типов планет, параметры токсичности; за… |
| biomes | db:planets.data.biomes | биомы планеты (99.2.28, слой 10 каскада после недр): объекты {form, share}, сумма 100%; физические веса от свойств планеты (T/в… |
| subterrain | db:planets.data.subterrain | недра планеты (99.2.28, слой 9): объекты {type, share}, сумма 100%; веса от физики + subterrain_bias + whitelist bands + мягкие… |
| surface_composition | db:planets.data.surface_composition | сводный состав поверхности — ПРОИЗВОДНАЯ от биомов (99.2.28): map[форма]доля, считается одной функцией; 8 потребителей; вне рее… |
| subterrain_composition | db:planets.data.subterrain_composition | сводный состав недр — ПРОИЗВОДНАЯ от объектов недр (99.2.28); потребители те же, что у surface_composition; вне реестра было на… |
| 99.2.28 | docs/specs/99.2.28-biomes-planet-surface.md | спека «Генерация биомов планеты» (99.2.28, 2026-09-20, ✅ реализована): справочник biome_catalog.json, слой 10 биомов после недр… |
| 2026-09-21-планета | docs/specs/2026-09-21-планета-форма-поверхности-блик-и-реальная-атмосфера.md | спека «Планета: форма поверхности, блик и реальная атмосфера» (2026-09-21, ✅ реализована): поле высот + регионы вместо пятен-кругов;… |
| planet_image | internal/generator/planet/planet_image_v2.go | честный генератор картинки планеты (planet_image_v2.go + postprocessing.go + rings.go; доработка 2026-09-21: поле высот + регионы, блик L(seed), реальная атмосфера full, кэш v3;… |
| atmosphere_data | internal/generator/planet/atmosphere.go | атмосфера-объект (99.2.20 §4.1, слой 6 каскада; в данных planets.data.atmosphere_data): состав газов %, давление, парниковый эф… |
| biome_icons | web/static/sprites | иконки биомов (идея 2026-09-20 «иконки биомов поверхности»): стиль B мини-пейзаж, исходник 48×48, на экране ~24 px; имена файло… |

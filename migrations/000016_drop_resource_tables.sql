-- Снос старых таблиц ресурсов (99_roadmap.md §99.2.5).
-- planet_resources пуста (в неё писал текущий генератор), resources — legacy
-- (2.6M строк от прежней генерации). Данные не переносятся: ресурсы
-- персистятся только в JSON-сводке data["resources"].
-- Замена им ещё не спроектирована: номенклатура выводится из потребностей
-- поселений (13_settlements.md), генератор веществ до этого не пишется.
DROP TABLE IF EXISTS planet_resources;
DROP TABLE IF EXISTS resources;
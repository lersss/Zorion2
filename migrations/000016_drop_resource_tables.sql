-- Переход на новую ресурсную модель (99_roadmap.md §99.2.5):
-- обе старые таблицы ресурсов заменяются на substances + deposits.
-- planet_resources пуста (в неё писал текущий генератор), resources — legacy
-- (2.6M строк от прежней генерации). Данные не переносятся: до прихода
-- новой модели ресурсы персистятся только в JSON-сводке data["resources"].
DROP TABLE IF EXISTS planet_resources;
DROP TABLE IF EXISTS resources;
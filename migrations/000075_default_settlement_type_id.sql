-- 000075_default_settlement_type_id.sql — базовый тип поселения по id
-- (решение создателя 2026-09-23: «чини, и делай по id, а не по имени»).
--
-- Почему не по имени: ResolveDefaultSettlementTypeID искал тип по
-- name_norm = 'обычное поселение'. Тип переименован в «Городок»
-- (id = 148, name_norm = 'городок') — поиск по имени перестал находить тип, и
-- новые поселения создавались с settlement_type_id = NULL (нет потребностей и
-- голода; механика стадий на тип опирается). Имя — контент и меняется в студии;
-- id связи — нет.
--
-- Существующие БД: бэкфилл ключа generation_config типом, который реально
-- используется поселениями (на dev-БД — 148). Свежая БД: поселений ещё нет →
-- строка не вставляется, ключ выставит Go-сид каталога
-- (internal/goodsstudio/seed_producers.go). Идемпотентно (ON CONFLICT DO NOTHING):
-- повторный прогон значение не переписывает.
--
-- Номер 000075: последний в дереве — 000074 (000072 отсутствует), проверено по
-- рабочему дереву migrations/ (PITFALLS «БД и шелл», ловушка занятого номера).

INSERT INTO generation_config (key, payload)
SELECT 'default_settlement_type_id', to_jsonb(settlement_type_id)
FROM settlements WHERE settlement_type_id IS NOT NULL
GROUP BY settlement_type_id ORDER BY count(*) DESC, settlement_type_id LIMIT 1
ON CONFLICT (key) DO NOTHING;

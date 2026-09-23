// internal/goodsstudio/seed_producers.go
// Сидер каталога типов производителей и предметов (спека
// 2026-09-20-фабрики §10.1 п.2 + 2026-09-21-студия-дерево-построек-канвас
// §1.4): при первом старте (маркер producer_catalog_seed в
// generation_config) в одной транзакции сеет 17 типов производителей
// (поселение, 7 подтипов-ступеней класса «Поселение» (Аутпост → Посёлок →
// Городок → Город → Мегаполис → Метрополия → Экуменополис), фабрика,
// автофабрика, добывающая платформа, энергостанция, лаборатория-родитель,
// 3 лаборатории-подтипа, фабрика продовольствия),
// 4 предмета (чертёж, сертификат анализа, модуль корабля, кирка) и связи
// лабораторий с предметами. Дерево построек: parent_id — тип-родитель
// (подтип → тип), категория — только у подтипов kind=goods (спека §1.2).
// Категории (categories) к этому моменту уже посеяны goodsstudio.Seed —
// сид вызывается после него (cmd/server/main.go). Повторные старты —
// пропуск (маркер): правки студии сидом не перезаписываются (С1-паттерн).
// INSERT с ON CONFLICT (name_norm) DO NOTHING: миграция 000051 на свежей БД
// уже создала тип «Лаборатория» (data-миграция §1.3) — не дублировать,
// взять существующий id.
package goodsstudio

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"zorion/internal/goodsstudio/graph"
	"zorion/internal/models"
)

// ProducerSeedMarkerKey — ключ маркера сидера производителей в
// generation_config: payload {"applied_at", "producers", "items", "links"}.
const ProducerSeedMarkerKey = "producer_catalog_seed"

// seedProducer — тип производителя сида (спека §2/§4.1 + дерево построек §1.4).
type seedProducer struct {
	Name       string
	Kind       string // goods/items/energy
	Category   string // name_norm категории (kind=goods, только у подтипов), "" = NULL
	Parent     string // имя типа-родителя (подтип), "" = тип (parent_id NULL)
	RaceFamily string // "" = NULL (универсальный)
	Output     string // JSONB
	Input      string // JSONB
	Params     string // JSONB
}

// seedItem — предмет сида (спека §2/§4.2).
type seedItem struct {
	Name     string
	SlotType string
	Unlocks  string // JSONB, "" = NULL
	Params   string // JSONB, "" = NULL
}

// seedProducerItem — связь «лаборатория → предмет» (producer_items).
type seedProducerItem struct {
	Producer string
	Item     string
}

// seedProducers — базовые типы производителей (спека §10.1 п.2 + дерево
// построек §1.4/§2.1). Автофабрика — корзина роботов: энергия + механика +
// электроника, НЕ еда (решение 3b.6.1). Добывающая платформа — чистый тип
// уровня 3 (без категории: категории живут в подтипах-платформах, §2.2).
// Лаборатория — тип-родитель (kind=items); три лаборатории — подтипы
// (Parent: "Лаборатория"). «Фабрика продовольствия» — подтип Фабрики
// (категория продовольствие). Порядок: родители раньше подтипов (parent_id
// резолвится по имени из уже вставленных).
var seedProducers = []seedProducer{
	{Name: "Поселение", Kind: "goods", Output: `{}`, Input: `{"people": {"capacity": 100}}`, Params: `{}`},
	// Ступени-подтипы класса «Поселение» без категории (спека стадий §4.3,
	// решения 32/37, спека итерации 4 §5.2): ладдера роста по населению (люди),
	// пороги — params.stage {enter, exit}, порядок читается по числам порогов.
	// Базовая ступень «Аутпост» — пол ладдеры (её exit не читается, не задан).
	// Прочие params «Аутпоста»: нормы еды — структура params.eat в единой единице
	// «ед/сутки/млрд» с признаком params.eat_units (спека 2026-09-23 §2.3/§2.5),
	// ключ — ПОЗИЦИЯ корзины (спека эффектов §7.5/§4.2); число 600 — то же, что
	// DefaultEatK и результат конверсии 000067/000070×2.4e10 (T18).
	// params.effects — пилотная привязка «позиция → тип эффекта» (§4.2): иначе
	// на свежей БД пилот — no-op (миграция 000070 на свежей БД строку «Обычное
	// поселение» ещё не видит — её создаёт этот сид).
	{Name: "Аутпост", Kind: "goods", Parent: "Поселение", Output: `{}`, Input: `{}`, Params: `{"eat": {"вода": 600, "пища": 600, "продовольствие": 600}, "eat_units": "per_day_per_billion", "effects": {"продовольствие": "голод"}, "stage": {"enter": 0}}`},
	{Name: "Посёлок", Kind: "goods", Parent: "Поселение", Output: `{}`, Input: `{}`, Params: `{"stage": {"enter": 1000, "exit": 750}}`},
	{Name: "Городок", Kind: "goods", Parent: "Поселение", Output: `{}`, Input: `{}`, Params: `{"stage": {"enter": 10000, "exit": 7500}}`},
	{Name: "Город", Kind: "goods", Parent: "Поселение", Output: `{}`, Input: `{}`, Params: `{"stage": {"enter": 100000, "exit": 75000}}`},
	{Name: "Мегаполис", Kind: "goods", Parent: "Поселение", Output: `{}`, Input: `{}`, Params: `{"stage": {"enter": 1000000, "exit": 750000}}`},
	{Name: "Метрополия", Kind: "goods", Parent: "Поселение", Output: `{}`, Input: `{}`, Params: `{"stage": {"enter": 10000000, "exit": 7500000}}`},
	{Name: "Экуменополис", Kind: "goods", Parent: "Поселение", Output: `{}`, Input: `{}`, Params: `{"stage": {"enter": 100000000, "exit": 75000000}}`},
	{Name: "Фабрика", Kind: "goods", Output: `{}`, Input: `{"people": {"capacity": 50}, "energy": true, "consumables": []}`, Params: `{"efficiency": 1.0}`},
	{Name: "Автофабрика", Kind: "goods", Output: `{}`, Input: `{"robots": true, "energy": true, "consumables": ["механика", "электроника"]}`, Params: `{"robot_cost": 100}`},
	{Name: "Добывающая платформа", Kind: "goods", Output: `{}`, Input: `{"energy": true, "consumables": []}`, Params: `{}`},
	{Name: "Энергостанция", Kind: "energy", Output: `{"energy": 100}`, Input: `{"people": {"capacity": 10}, "fuel": true}`, Params: `{}`},
	{Name: "Лаборатория", Kind: "items", Output: `{}`, Input: `{"people": {"capacity": 10}, "energy": true, "consumables": []}`, Params: `{}`},
	{Name: "Лаборатория космических технологий", Kind: "items", Parent: "Лаборатория", Output: `{"items": ["Модуль корабля"]}`, Input: `{"people": {"capacity": 10}, "energy": true, "consumables": []}`, Params: `{}`},
	{Name: "Лаборатория экипировки", Kind: "items", Parent: "Лаборатория", Output: `{"items": ["Кирка"]}`, Input: `{"people": {"capacity": 10}, "energy": true, "consumables": []}`, Params: `{}`},
	{Name: "Исследовательская лаборатория", Kind: "items", Parent: "Лаборатория", Output: `{"items": ["Чертёж", "Сертификат анализа"]}`, Input: `{"people": {"capacity": 10}, "energy": true, "consumables": []}`, Params: `{}`},
	{Name: "Фабрика продовольствия", Kind: "goods", Category: "продовольствие", Parent: "Фабрика", Output: `{}`, Input: `{"people": {"capacity": 50}, "energy": true, "consumables": []}`, Params: `{"efficiency": 1.0}`},
}

// seedItems — базовые предметы (спека §10.1 п.2): типы «что бывает»;
// экземпляры живут в инвентаре, не здесь (§1).
var seedItems = []seedItem{
	{Name: "Чертёж", SlotType: "чертёж", Unlocks: `[]`, Params: `{}`},
	{Name: "Сертификат анализа", SlotType: "сертификат", Params: `{}`},
	{Name: "Модуль корабля", SlotType: "модуль", Params: `{}`},
	{Name: "Кирка", SlotType: "инструмент", Params: `{}`},
}

// seedProducerItems — связи лабораторий с предметами (спека §2; имена —
// новые, решение создателя идея §2 п.4).
var seedProducerItems = []seedProducerItem{
	{Producer: "Исследовательская лаборатория", Item: "Чертёж"},
	{Producer: "Исследовательская лаборатория", Item: "Сертификат анализа"},
	{Producer: "Лаборатория космических технологий", Item: "Модуль корабля"},
	{Producer: "Лаборатория экипировки", Item: "Кирка"},
}

// SeedProducers — сидер каталога производителей (спека §10.1 п.2).
// Маркер producer_catalog_seed в generation_config; повторные старты —
// пропуск. Ошибка — возвращается; вызывающий (cmd/server/main.go) делает
// log.Fatal.
func SeedProducers(db *sql.DB) error {
	var exists bool
	if err := db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM generation_config WHERE key = $1)`, ProducerSeedMarkerKey,
	).Scan(&exists); err != nil {
		return fmt.Errorf("seed producers: маркер: %w", err)
	}
	if exists {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("seed producers: begin: %w", err)
	}
	defer tx.Rollback()

	// Категории по name_norm (посеяны goodsstudio.Seed до этого сида).
	catByName := make(map[string]int64)
	rows, err := tx.Query(`SELECT id, name_norm FROM categories`)
	if err != nil {
		return fmt.Errorf("seed producers: категории: %w", err)
	}
	for rows.Next() {
		var id int64
		var norm string
		if err := rows.Scan(&id, &norm); err != nil {
			rows.Close()
			return fmt.Errorf("seed producers: категории: %w", err)
		}
		catByName[norm] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("seed producers: категории: %w", err)
	}

	// Типы производителей. Родители идут раньше подтипов (см. seedProducers) —
	// parent_id резолвится по имени из уже вставленных. ON CONFLICT DO NOTHING:
	// миграция 000051 на свежей БД уже создала «Лабораторию» (data-миграция
	// §1.3) — не дублировать, взять существующий id.
	producerIDs := make(map[string]int64, len(seedProducers))
	for _, p := range seedProducers {
		var catID interface{}
		if p.Category != "" {
			id, ok := catByName[p.Category]
			if !ok {
				return fmt.Errorf("seed producers: категория %q не найдена", p.Category)
			}
			catID = id
		}
		var parentID interface{}
		if p.Parent != "" {
			id, ok := producerIDs[p.Parent]
			if !ok {
				return fmt.Errorf("seed producers: родитель %q не найден (порядок: родители раньше подтипов)", p.Parent)
			}
			parentID = id
		}
		var id int64
		err := tx.QueryRow(
			`INSERT INTO producer_types (name, name_norm, kind, category_id, race_family, parent_id, race, output, input, params)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 ON CONFLICT (name_norm) DO NOTHING RETURNING id`,
			p.Name, graph.NormalizeName(p.Name), p.Kind, catID, nullStr(p.RaceFamily),
			parentID, nil, p.Output, p.Input, p.Params,
		).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			// уже есть (миграция 000051 на свежей БД) — существующий id.
			if err := tx.QueryRow(
				`SELECT id FROM producer_types WHERE name_norm = $1`, graph.NormalizeName(p.Name),
			).Scan(&id); err != nil {
				return fmt.Errorf("seed producers: тип %s: %w", p.Name, err)
			}
		} else if err != nil {
			return fmt.Errorf("seed producers: тип %s: %w", p.Name, err)
		}
		producerIDs[p.Name] = id
	}

	// Базовый тип поселения — id в generation_config (решение создателя
	// 2026-09-23: связь по id, не по имени). Путь свежей БД: миграция 000075
	// видит пустые settlements и ключ не ставит — ставит сид. Читает
	// repository.ResolveDefaultSettlementTypeID. Upsert идемпотентен и живёт в
	// той же транзакции, что и остальной сид.
	if id, ok := producerIDs["Аутпост"]; ok {
		if _, err := tx.Exec(
			`INSERT INTO generation_config (key, payload) VALUES ($1, to_jsonb($2::bigint))
			 ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`,
			models.DefaultSettlementTypeIDKey, id,
		); err != nil {
			return fmt.Errorf("seed producers: default_settlement_type_id: %w", err)
		}
	}

	// Слоты родителя (спека 2026-09-21-скрытые §1.3, путь 2 — свежие БД):
	// базовый сид универсального уровня + покрытие сид-подтипов. Прямые INSERT
	// мимо валидаций репозитория — инвариант С4 (слот обязателен для подтипа
	// kind=goods) к сиду не применяется: слоты создаются тем же сидом/миграцией.
	if err := seedProducerSlots(tx, producerIDs); err != nil {
		return err
	}

	// Предметы.
	itemIDs := make(map[string]int64, len(seedItems))
	for _, it := range seedItems {
		var id int64
		if err := tx.QueryRow(
			`INSERT INTO items (name, name_norm, slot_type, unlocks, params)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			it.Name, graph.NormalizeName(it.Name), it.SlotType, nullStr(it.Unlocks), nullStr(it.Params),
		).Scan(&id); err != nil {
			return fmt.Errorf("seed producers: предмет %s: %w", it.Name, err)
		}
		itemIDs[it.Name] = id
	}

	// Связи лабораторий с предметами.
	for _, link := range seedProducerItems {
		pid, ok1 := producerIDs[link.Producer]
		iid, ok2 := itemIDs[link.Item]
		if !ok1 || !ok2 {
			return fmt.Errorf("seed producers: связь %s→%s: не найдены", link.Producer, link.Item)
		}
		if _, err := tx.Exec(
			`INSERT INTO producer_items (producer_type_id, item_id) VALUES ($1, $2)`, pid, iid,
		); err != nil {
			return fmt.Errorf("seed producers: связь %s→%s: %w", link.Producer, link.Item, err)
		}
	}

	// Маркер — в той же транзакции: сид атомарен.
	marker, err := json.Marshal(map[string]interface{}{
		"applied_at": time.Now().UTC().Format(time.RFC3339),
		"producers":  len(seedProducers),
		"items":      len(seedItems),
		"links":      len(seedProducerItems),
	})
	if err != nil {
		return fmt.Errorf("seed producers: маркер: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO generation_config (key, payload) VALUES ($1, $2)`, ProducerSeedMarkerKey, string(marker),
	); err != nil {
		return fmt.Errorf("seed producers: маркер: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("seed producers: commit: %w", err)
	}
	return nil
}

// nullStr — пустая строка → NULL (для nullable JSONB/TEXT).
func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// seedProducerSlots — базовые слоты универсального уровня (спека скрытых
// §1.3, путь 2): Фабрика/Автофабрика × все товарные категории (kind='good',
// 13), Добывающая платформа × ресурсные (kind='resource', 6). Сид-подтип
// «Фабрика продовольствия» покрыт базовыми 13 (продовольствие — товарная
// категория) — отдельный слот не нужен. Идемпотентно (ON CONFLICT DO NOTHING).
func seedProducerSlots(tx *sql.Tx, producerIDs map[string]int64) error {
	rows, err := tx.Query(`SELECT id, kind FROM categories`)
	if err != nil {
		return fmt.Errorf("seed producers: слоты: категории: %w", err)
	}
	var goodsCats, resCats []int64
	for rows.Next() {
		var id int64
		var kind string
		if err := rows.Scan(&id, &kind); err != nil {
			rows.Close()
			return fmt.Errorf("seed producers: слоты: категории: %w", err)
		}
		switch kind {
		case "good":
			goodsCats = append(goodsCats, id)
		case "resource":
			resCats = append(resCats, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("seed producers: слоты: категории: %w", err)
	}
	insert := func(parent string, cats []int64) error {
		pid, ok := producerIDs[parent]
		if !ok {
			return nil // тип не создан (переименован/удалён) — слоты не создаём
		}
		for _, cid := range cats {
			if _, err := tx.Exec(
				`INSERT INTO producer_slots (parent_id, category_id) VALUES ($1, $2)
				 ON CONFLICT DO NOTHING`, pid, cid,
			); err != nil {
				return fmt.Errorf("seed producers: слот %s×%d: %w", parent, cid, err)
			}
		}
		return nil
	}
	for _, name := range []string{"Фабрика", "Автофабрика"} {
		if err := insert(name, goodsCats); err != nil {
			return err
		}
	}
	if err := insert("Добывающая платформа", resCats); err != nil {
		return err
	}
	return nil
}
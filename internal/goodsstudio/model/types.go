// Package model — сущности студии товаров (спека 99a.1 §5): категории,
// товары, слоты рецептов. Состояние — один state.json.
package model

import "time"

// Kind — вид узла графа.
type Kind string

const (
	KindGood     Kind = "good"
	KindResource Kind = "resource"
)

// Source — откуда появился товар (спека 99a.1 §5.1).
type Source string

const (
	SourceManual  Source = "manual"  // создан вручную
	SourceAI      Source = "ai"      // вставлен ответом ИИ
	SourcePalette Source = "palette" // импортирован из каталога ресурсов
	SourceImport  Source = "import"  // импортирован из top-каталога (99a.2 §4.1)
)

// Category — категория студии (свободное множество, спека 99a.1 §5.1).
// Kind — good/resource (спека переноса-студии-товаров-iterC §5.4, C2:
// категория нового товара от ИИ должна существовать и быть kind=good).
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind Kind   `json:"kind,omitempty"`
}

// Slot — элемент рецепта: ссылка на составляющий товар (или пустой).
// Количество введено (99a.2 §6): quantity — единиц составляющей; 0 = не
// задано (старый слот) → трактуется как 1 (нормализация при загрузке).
type Slot struct {
	GoodID   string `json:"good_id"`
	Quantity int    `json:"quantity,omitempty"` // единиц составляющей; 0 = не задано (старый слот) → 1
	Reason   string `json:"reason,omitempty"`   // тултип «почему в составе» (от ИИ, §7.3)
	// AllowResource — галка «заполнять ресурсом» (99a Пакет 4, п.8, решение
	// создателя 2026-09-19): ресурсы в рецепт только если прямо указано.
	// По умолчанию выключена (bool zero = false — старые слоты автоматически
	// «без ресурса», миграции state.json не нужны). Влияет только на пустой
	// слот: с галкой ИИ может предложить ресурс, без галки — только товары.
	AllowResource bool `json:"allow_resource"`
}

// Good — узел графа рецептов (спека 99a.1 §5.1).
type Good struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Category    string       `json:"category"`
	Kind        Kind         `json:"kind"`
	Source      Source       `json:"source"`
	Recipe      []Slot       `json:"recipe"`
	// RecipeID — id рецепта товара (recipes.id, спека
	// 2026-09-21-рецепт-сущность §2.3); 0 у ресурса (рецепта нет).
	RecipeID    int64        `json:"recipe_id,omitempty"`
	CreatedAt   string       `json:"created_at"`
	// Complexity — сложность рецепта (recipes.complexity, спека
	// 2026-09-21-рецепт-сущность §2.3): null = вычисляется по графу; заданное —
	// эффективное значение (тир = complexity ?? вычисленный). У ресурса поля
	// нет (тир = 0): ручного тира/сложности у ресурса не существует.
	Complexity *int `json:"complexity,omitempty"`
	// Volume/Weight — данные каталога (спека 2026-09-20-фабрики §3.1,
	// решение 3b.6.4): значение есть всегда (Р2, 2026-09-21) — дефолт 1/1,
	// правится вручную. Механика грузов/трюма — будущая фича, поля — данные
	// каталога.
	Volume *float64 `json:"volume,omitempty"`
	Weight *float64 `json:"weight,omitempty"`
}

// RecipeBinding — привязка рецепта к конкретной фабрике (producer_recipes,
// спека 2026-09-21-рецепт-сущность §2.3): проекция для валидатора
// (`State.Bindings`) — производное «какие фабрики держат рецепт».
type RecipeBinding struct {
	RecipeID       int64 `json:"recipe_id"`
	ProducerTypeID int64 `json:"producer_type_id"`
	GoodID         int64 `json:"good_id"`
}

// State — полное состояние студии (спека 99a.1 §5; Bindings — спека
// 2026-09-21-рецепт-сущность §5).
type State struct {
	SchemaVersion int        `json:"schema_version"`
	Categories    []Category `json:"categories"`
	Goods         []Good     `json:"goods"`
	// Bindings — привязки рецептов к фабрикам (producer_recipes), заполняется
	// снимком каталога; нужен валидатору (unbound_recipe).
	Bindings []RecipeBinding `json:"bindings,omitempty"`
}

// NowISO — текущее время в ISO8601 (UTC).
func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
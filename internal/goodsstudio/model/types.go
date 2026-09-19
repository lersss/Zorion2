// Package model — сущности студии товаров (спека 99a.1 §5): категории,
// товары, слоты рецептов, статусы. Состояние — один state.json.
package model

import "time"

// Status — статус товара (ровно один, спека 99a.1 §5.2).
type Status string

const (
	StatusDraft    Status = "draft"    // черновик (по умолчанию для новых)
	StatusApproved Status = "approved" // согласовано (контент, идёт в экспорт)
	StatusExcluded Status = "excluded" // исключено (не контент, возвращаемо)
	StatusBanned   Status = "banned"   // бан (ИИ больше не предлагает; возврат в один клик)
	StatusResource Status = "resource" // импортированный ресурс (read-only)
)

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
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ResourceRef — ссылка импортированного ресурса на каталог сервера
// (спека 99a.1 §5.1): catalog — layer|real, catalog_id — id в каталоге.
type ResourceRef struct {
	Catalog   string `json:"catalog"`
	CatalogID string `json:"catalog_id"`
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
	Status      Status       `json:"status"`
	BannedAt    *string      `json:"banned_at"`
	Kind        Kind         `json:"kind"`
	Source      Source       `json:"source"`
	Recipe      []Slot       `json:"recipe"`
	CreatedAt   string       `json:"created_at"`
	ResourceRef *ResourceRef `json:"resource_ref,omitempty"`
	// TierOverride — ручной тир (99a.3 §4.1): null = вычисляется (текущее
	// поведение); заданное значение — полная свобода (любое целое ≥ 0, в т.ч.
	// ниже вычисленного). Эффективный тир = override ?? вычисленный; ресурс = 0.
	// Старые state.json без поля = null — миграции не нужны (zero value).
	TierOverride *int `json:"tier_override,omitempty"`
}

// State — полное состояние студии (один state.json, спека 99a.1 §5).
type State struct {
	SchemaVersion int        `json:"schema_version"`
	Categories    []Category `json:"categories"`
	Goods         []Good     `json:"goods"`
}

// NowISO — текущее время в ISO8601 (UTC).
func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
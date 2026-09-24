// Package contentio — формат файла-снимка контента каталога студии (спека
// 2026-09-24-каталог-экспорт-импорт-контента-на-прод §4/§5): секции
// справочников, ссылки по метке (`code`) и натуральному ключу, без `id`.
// Единый источник формата для обеих сторон переноса: экспорт (И2) сериализует
// снимок, импорт (И3) читает его же. Секции упорядочены топологически по FK
// (родители раньше детей) — порядок полей struct = порядок секций в JSON.
package contentio

import "encoding/json"

// SupportedSchemaVersion — версия формата снимка, которую умеет читать импорт
// (§5 п.1: schema_version не выше поддерживаемой).
const SupportedSchemaVersion = 1

// Snapshot — файл-снимок контента (§4): порядок полей = порядок секций в JSON.
type Snapshot struct {
	SchemaVersion    int               `json:"schema_version"`
	GeneratedAt      string            `json:"generated_at"`
	Source           string            `json:"source"`
	Counts           map[string]int    `json:"counts"`
	Categories       []Category        `json:"categories"`
	Goods            []Good            `json:"goods"`
	Recipes          []Recipe          `json:"recipes"`
	RecipeComponents []RecipeComponent `json:"recipe_components"`
	ProducerTypes    []ProducerType    `json:"producer_types"`
	Items            []Item            `json:"items"`
	ProducerSlots    []ProducerSlot    `json:"producer_slots"`
	ProducerRecipes  []ProducerRecipe  `json:"producer_recipes"`
	ProducerItems    []ProducerItem    `json:"producer_items"`
	EffectTypes      []EffectType      `json:"effect_types"`
	GenerationConfig GenerationConfig  `json:"generation_config"`
}

// Category — категория: идентичность (kind, name_norm); code — семантический
// (water/gas/…), не метка переноса (§3.4).
type Category struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Code     string `json:"code,omitempty"`
	IsSystem bool   `json:"is_system"`
}

// Good — товар/ресурс: category — «голое» имя (резолв по goods.kind, §4);
// props переносится дословно; code — обязательная метка (§3).
type Good struct {
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	Category    string          `json:"category"`
	Description string          `json:"description,omitempty"`
	Volume      *float64        `json:"volume"`
	Weight      *float64        `json:"weight"`
	Props       json.RawMessage `json:"props,omitempty"`
	Source      string          `json:"source"`
	Code        string          `json:"code"`
}

// Recipe — рецепт товара: good — метка товара-выхода.
type Recipe struct {
	Good       string `json:"good"`
	Complexity *int   `json:"complexity"`
}

// RecipeComponent — позиция состава: good — метка товара рецепта, component —
// метка составляющей (пусто = пустой слот).
type RecipeComponent struct {
	Good          string `json:"good"`
	Pos           int    `json:"pos"`
	Component     string `json:"component,omitempty"`
	Quantity      int    `json:"quantity"`
	Reason        string `json:"reason,omitempty"`
	AllowResource bool   `json:"allow_resource"`
}

// ProducerType — тип производителя: parent — метка родителя, category — имя
// категории; category_kind — её kind (good/resource) — имя категории
// неоднозначно между пространствами (напр. «топливо» есть в обоих), §3.4/§4;
// section — раздел вида постройки (значение-ключ, дословно, §4).
type ProducerType struct {
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	Category     string          `json:"category,omitempty"`
	CategoryKind string          `json:"category_kind,omitempty"`
	Parent       string          `json:"parent,omitempty"`
	RaceFamily   string          `json:"race_family,omitempty"`
	Race         string          `json:"race,omitempty"`
	Output       json.RawMessage `json:"output,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	Params       json.RawMessage `json:"params,omitempty"`
	Hidden       bool            `json:"hidden"`
	Section      string          `json:"section,omitempty"`
	Code         string          `json:"code"`
}

// UnlockCategory — натуральный ключ категории в ссылке предмета (§4):
// (kind, name) — как у прочих ссылок на categories (§3.4).
type UnlockCategory struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Unlock — предмет-рецепт в снимке (§4): producer — метка типа производителя
// (`producer_types.code`), category — натуральный ключ. Поля `producer_type_id`/
// `category_id` — id-форма БД, в снимке НЕДОПУСТИМА (их присутствие при импорте
// → отказ, §4/T15).
type Unlock struct {
	Producer       string         `json:"producer,omitempty"`
	Category       UnlockCategory `json:"category"`
	ProducerTypeID *int64         `json:"producer_type_id,omitempty"`
	CategoryID     *int64         `json:"category_id,omitempty"`
}

// Item — тип предмета (§4): unlocks несётся по метке/натуральному ключу
// (НЕ по id: в БД `items.unlocks` хранит внутренние id, они не переносимы,
// §4/§5 п.6/T15); params — дословно; code — обязательна.
type Item struct {
	Name     string          `json:"name"`
	SlotType string          `json:"slot_type"`
	Unlocks  []Unlock        `json:"unlocks,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Code     string          `json:"code"`
}

// ProducerSlot — слот родителя: parent — метка, category — имя категории;
// category_kind — её kind (good/resource), т.к. имя неоднозначно между
// пространствами (§3.4/§4).
type ProducerSlot struct {
	Parent       string `json:"parent"`
	Category     string `json:"category"`
	CategoryKind string `json:"category_kind,omitempty"`
	RaceFamily   string `json:"race_family,omitempty"`
	Race         string `json:"race,omitempty"`
	Hidden       bool   `json:"hidden"`
}

// ProducerRecipe — привязка «постройка × рецепт»: producer/good — метки.
type ProducerRecipe struct {
	Producer string   `json:"producer"`
	Good     string   `json:"good"`
	Rate     *float64 `json:"rate"`
}

// ProducerItem — связь «производитель предметов ↔ предмет»: метки.
type ProducerItem struct {
	Producer     string          `json:"producer"`
	Item         string          `json:"item"`
	Requirements json.RawMessage `json:"requirements,omitempty"`
}

// EffectType — тип эффекта: params дословно; code — обязательна.
type EffectType struct {
	Name   string          `json:"name"`
	Impact string          `json:"impact"`
	Params json.RawMessage `json:"params,omitempty"`
	Code   string          `json:"code"`
}

// GenerationConfig — ссылка-якорь (§1.3): метка записи базового типа
// поселения (не сырой id); пусто — ключ не задан.
type GenerationConfig struct {
	DefaultSettlementTypeID string `json:"default_settlement_type_id"`
}

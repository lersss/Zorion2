// internal/repository/content_import.go
// Импорт снимка контента каталога dev → прод (спека 2026-09-24-каталог-
// экспорт-импорт-контента-на-прод §5, итерация И3): одна транзакция с
// pg_advisory_xact_lock (beginMutation), режим сопоставления по метке (штатный)
// или по (kind, name_norm) (первичная накатка, §3.5), upsert без смены `id`
// существующих записей, полная замена отсутствующего в файле с предсчётом
// мировых ссылок (§5.4), сухой прогон (ROLLBACK) и отчёт (§5.5).
//
// Точка входа — ImportContent (content_import_entry.go); здесь — секции,
// типы результата/диффа, строки целевой БД и натуральные ключи.
package repository

import (
	"database/sql"

	"zorion/internal/goodsstudio/graph"
)

// Секции отчёта/диффа (совпадают с ключами `counts` снимка, §4).
const (
	secCategories       = "categories"
	secGoods            = "goods"
	secRecipes          = "recipes"
	secRecipeComponents = "recipe_components"
	secProducerTypes    = "producer_types"
	secItems            = "items"
	secProducerSlots    = "producer_slots"
	secProducerRecipes  = "producer_recipes"
	secProducerItems    = "producer_items"
	secEffectTypes      = "effect_types"
)

// ImportDiffEntry — запись в перечне диффа/расхождений (§5.2): секция, метка
// (у categories метки нет — пусто), имя.
type ImportDiffEntry struct {
	Section string `json:"section"`
	Code    string `json:"code,omitempty"`
	Name    string `json:"name"`
}

// ImportBlocked — удаление, упёршееся в мировую ссылку (§5.4): таблица-ссылка
// и число ссылок.
type ImportBlocked struct {
	Section  string `json:"section"`
	Code     string `json:"code,omitempty"`
	Name     string `json:"name"`
	RefTable string `json:"ref_table"`
	RefCount int    `json:"ref_count"`
}

// ImportRemapEntry — метка цели, которая будет переназначена по файлу при
// первичной накатке (§3.5/§5.2): та же запись по `(kind, name_norm)`, но другой
// (или пустой) `code`.
type ImportRemapEntry struct {
	Section string `json:"section"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	OldCode string `json:"old_code,omitempty"`
}

// ImportDiff — сухой прогон (§5.2): режим, числа создать/обновить, перечень
// удаляемых записей, заблокированные мировыми ссылками, расхождения тождества,
// переназначаемые метки (накатка).
type ImportDiff struct {
	Mode      string             `json:"mode"`
	Create    map[string]int     `json:"create"`
	Update    map[string]int     `json:"update"`
	Delete    []ImportDiffEntry  `json:"delete"`
	Blocked   []ImportBlocked    `json:"blocked"`
	Unmatched []ImportDiffEntry  `json:"unmatched"`
	Remap     []ImportRemapEntry `json:"remap"`
}

// ImportReport — отчёт успешного применения (§5.5): числа по секциям.
type ImportReport struct {
	Created  map[string]int `json:"created"`
	Updated  map[string]int `json:"updated"`
	Deleted  map[string]int `json:"deleted"`
	Warnings []string       `json:"warnings"`
}

// ImportResult — итог импорта: дифф (всегда) и отчёт (при применении).
type ImportResult struct {
	Mode   string        `json:"mode"`
	DryRun bool          `json:"dry_run"`
	Diff   *ImportDiff   `json:"diff,omitempty"`
	Report *ImportReport `json:"report,omitempty"`
}

// --- строки целевой БД (с name_norm — для сопоставления §3.5) ---

type impCategory struct {
	ID       int64
	Name     string
	NameNorm string
	Kind     string
	Code     sql.NullString
	IsSystem bool
}

type impGood struct {
	ID          int64
	Name        string
	NameNorm    string
	CategoryID  int64
	Kind        string
	Source      string
	Description sql.NullString
	Volume      float64
	Weight      float64
	Props       []byte
	Code        sql.NullString
}

type impRecipe struct {
	ID         int64
	GoodID     int64
	Complexity sql.NullInt64
}

type impComponent struct {
	RecipeID      int64
	Pos           int
	ComponentID   sql.NullInt64
	Quantity      int
	Reason        sql.NullString
	AllowResource bool
}

type impProducer struct {
	ID         int64
	Name       string
	NameNorm   string
	Kind       string
	CategoryID sql.NullInt64
	ParentID   sql.NullInt64
	RaceFamily sql.NullString
	Race       sql.NullString
	Output     []byte
	Input      []byte
	Params     []byte
	Hidden     bool
	Section    sql.NullString
	Code       sql.NullString
}

type impItem struct {
	ID       int64
	Name     string
	NameNorm string
	SlotType string
	Unlocks  []byte
	Params   []byte
	Code     sql.NullString
}

type impSlot struct {
	ParentID   int64
	CategoryID int64
	RaceFamily sql.NullString
	Race       sql.NullString
	Hidden     bool
}

type impBinding struct {
	ProducerTypeID int64
	RecipeID       int64
	GoodID         int64
	Rate           sql.NullFloat64
}

type impProducerItem struct {
	ProducerTypeID int64
	ItemID         int64
	Requirements   []byte
}

type impEffect struct {
	ID       int64
	Name     string
	NameNorm string
	Impact   string
	Params   []byte
	Code     sql.NullString
}

// catKey — натуральный ключ категории (§3.4): (kind, name_norm).
type catKey struct {
	Kind string
	Name string
}

func catKeyOf(kind, name string) catKey { return catKey{Kind: kind, Name: graph.NormalizeName(name)} }
func catKeyStr(k catKey) string         { return k.Kind + "\x00" + k.Name }

// catKindOrDefault — kind категории из снимка; пусто (старый файл) → «good»
// (обратная совместимость, §4).
func catKindOrDefault(kind string) string {
	if kind == "" {
		return "good"
	}
	return kind
}

// catExists — есть ли категория (kind, name_norm) в множестве снимка. Пустой
// kind (старый файл без `category_kind`) трактуется ТАК ЖЕ, как в apply
// (`catKindOrDefault` → «good») — иначе валидация пропускала бы resource-имя,
// а применение падало FK-ошибкой (рассинхрон фолбэка).
func catExists(set map[catKey]bool, kind, name string) bool {
	return set[catKeyOf(catKindOrDefault(kind), name)]
}

// goodKey/prodKey — ключи первичной накатки (§3.5).
type goodKey struct {
	Kind string
	Name string
}

func goodKeyOf(kind, name string) goodKey {
	return goodKey{Kind: kind, Name: graph.NormalizeName(name)}
}

type prodKey struct {
	Kind string
	Name string
}

func prodKeyOf(kind, name string) prodKey {
	return prodKey{Kind: kind, Name: graph.NormalizeName(name)}
}

// internal/audit/types.go
package audit

// ==================== УРОВНИ КРИТИЧНОСТИ ====================

const (
	SeverityLow    = "low"
	SeverityMedium = "medium"
	SeverityHigh   = "high"
)

// ==================== ПРОБЛЕМА ====================

// Issue — одна найденная проблема.
// Универсальна: используется и для планет, и для звёзд, и для фракций.
type Issue struct {
	// Идентификация сущности, у которой найдена проблема.
	EntityID   string `json:"entity_id"`   // ID планеты / звезды / фракции
	EntityType string `json:"entity_type"` // "planet" / "star" / "faction"
	EntityName string `json:"entity_name"` // читаемое имя

	// Контекст (для планет — world_id, для фракций — homeworld и т.п.)
	ContextID string `json:"context_id,omitempty"`

	// Проблема
	Code        string                 `json:"code"`        // "jungles_in_cold"
	Severity    string                 `json:"severity"`    // "low" / "medium" / "high"
	Description string                 `json:"description"` // человекочитаемое
	Details     map[string]interface{} `json:"details,omitempty"`
}

// ==================== РЕЗУЛЬТАТ ====================

// AuditResult — результат полного аудита.
// Один и тот же тип для всех сущностей: планет, звёзд, фракций.
type AuditResult struct {
	// Что проверялось
	EntityType string `json:"entity_type"` // "planet" / "star" / ...

	// Итоги
	TotalEntities          int   `json:"total_entities"`
	EntitiesWithIssue      int   `json:"entities_with_issues"`
	EntitiesWithHighIssue  int   `json:"entities_with_high_issues"`
	TotalIssues            int   `json:"total_issues"`
	DurationMs             int64 `json:"duration_ms"`

	// Агрегаты по коду проблемы
	IssuesByCode map[string]int `json:"issues_by_code"`

	// Агрегаты по уровню критичности
	IssuesBySeverity map[string]int `json:"issues_by_severity"`

	// Агрегаты по коду для High-проблем («невозможные» планеты).
	HighIssuesByCode map[string]int `json:"high_issues_by_code"`

	// Примеры проблем (до SampleLimit)
	SampleIssues []Issue `json:"sample_issues"`

	// Был ли результат усечён
	Truncated bool `json:"truncated"`
}

// SampleLimit — максимальное количество примеров в ответе.
const SampleLimit = 200

// ==================== ПРАВИЛО ====================

// Rule — одно правило проверки.
// Дженерик по типу сущности: для планет это Rule[planet.View],
// для звёзд Rule[star.View] и т.д.
//
// Правило принимает "представление" сущности и возвращает список проблем.
// Каждое правило само формирует Issue с нужными Code/Severity.
type Rule[T any] struct {
	Check func(item *T) []Issue
}

// ==================== КОНТЕКСТ ====================

// ContextKey — общий интерфейс для "вида" сущности.
// Позволяет движку достать ID/Name/ContextID из любого типа.
//
// Каждая сущность (View) реализует этот интерфейс.
type ContextKey interface {
	GetID() string
	GetName() string
	GetContextID() string
	GetEntityType() string
}
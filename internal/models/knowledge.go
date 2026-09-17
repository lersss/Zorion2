// internal/models/knowledge.go
// Личный каталог знания о планетах (спека 77a §8): одна запись на
// (игрок, планета), дата актуальности, протухание 7 дней — статус на
// чтении, без фоновых джобов (И8, И10).
package models

import "time"

// PlanetKnowledge — запись player_planet_knowledge.
type PlanetKnowledge struct {
	UserID    string                 `json:"user_id"`
	PlanetID  string                 `json:"planet_id"`
	Data      map[string]interface{} `json:"data"` // поверхность, наличие поселений, источник
	ScannedAt time.Time              `json:"scanned_at"`
	Source    string                 `json:"source"` // 'scanner' | 'report' (задел)
}

// IsFresh — актуально ли знание на момент now (спека 77a §8.2):
// scanned_at ≥ now − 7 дней. Устаревшее знание не считается «свежим»,
// но не удаляется (история игрока остаётся).
func (k *PlanetKnowledge) IsFresh(now time.Time) bool {
	return k != nil && now.Sub(k.ScannedAt) <= KnowledgeTTL
}
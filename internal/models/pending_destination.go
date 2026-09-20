// internal/models/pending_destination.go
// Намерение композитного маршрута (спека 99.2.30 §2.2): users.pending_destination
// JSONB, NULL = намерения нет. Серверное состояние игрока «после прибытия в
// систему X лететь к объекту P» — существует только во время межзвёздного
// сегмента композитного маршрута; переживает рефреш (БД) и рестарт сервера
// (Restore-обработка §4.5). Расширяемость — поле kind (задел, не пишется).
package models

// PendingDestination — намерение композитного маршрута.
type PendingDestination struct {
	WorldID    string `json:"world_id"`    // система назначения межзвёздного сегмента (== req.WorldID на старте /travel)
	ObjectType string `json:"object_type"` // planet|satellite|companion
	ObjectID   string `json:"object_id"`   // planets.id для planet; id спутника из planets.data.satellites для satellite; синтетический id компаньона companion:<world>/extra:<world>:<i> для companion
}

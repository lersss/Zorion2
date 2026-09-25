package models

import "time"

// RoutePuzzle — состояние сегментной задачи мини-игры «Прокладка маршрута»
// (таблица player_route_puzzle, спека ускорителя §14.1, модель v9 «Планшет»):
// одна строка на пару (игрок, kind). SegmentHash привязывает состояние к
// сегменту перелёта, Secret/Layout — детерминированное поле (клиенту не
// отдаются), Revealed — JSON-массив вскрытых секторов, PingsLeft — остаток
// импульсов разведки. Состояние игрока: переживает рестарт и очистку
// вселенной (НЕ входит в truncateTables).
type RoutePuzzle struct {
	UserID      string
	Kind        string
	SegmentHash []byte
	Secret      []byte
	Layout      []byte
	Revealed    []byte
	PingsLeft   int
	CreatedAt   time.Time
}

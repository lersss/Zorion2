package models

import "time"

// Region — регион галактики (сектор вокруг кластерного центра).
// Используется для отображения карты на малом зуме.
type Region struct {
	ID         string
	Name       string
	CenterX    float64
	CenterY    float64
	Radius     float64
	Color      string
	WorldCount int
	// Profile — ключ класса профиля региона (59a, спека 99.2.10 §10);
	// пусто — фоновый регион (~25%). Не публикуется (не ярлык, §11.7).
	Profile string
	// ProfileIntensity — интенсивность профиля 0/1/2: слабая/средняя/сильная.
	ProfileIntensity int
	// RaceID — доминантная раса территории (спека 99.2.21 §7.1); пусто =
	// не назначена (до раздачи, легаси-вселенные).
	RaceID string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
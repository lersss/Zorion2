// internal/models/deposit.go
package models

// SurfaceDeposit — залежь поверхности планеты (спека 2026-09-22-поселение-
// добыча-сырья-биома-ленивый-буфер §5.2): объект поверхности «планета +
// ресурс каталога + слой + богатство + конечный запас».
//
// ID/PlanetID — служебные для вставки (uuid залежи и планета-владелец);
// в API не сериализуются (в блоке deposits карточки §5.2 их нет).
// GoodName заполняется чтением (JOIN goods) — у сгенерированных залежей в
// памяти пусто (генератор знает только good_id из карты name_norm → id).
type SurfaceDeposit struct {
	ID       string  `json:"-"`
	PlanetID string  `json:"-"`
	GoodID   int64   `json:"good_id"`
	GoodName string  `json:"good_name"`
	Stratum  string  `json:"stratum"`
	Wealth   float64 `json:"wealth"`
	Amount   float64 `json:"amount"`
}

package handlers

import "zorion/internal/routegame"

// accelerator_grid_dto.go — DTO доски v9 «Планшет»: публичные контракты
// offer/scan (§3.4/§14.8). Вынесено из accelerator_grid_handlers.go без
// изменения полей и JSON-тегов.

// acceleratorOfferResponse — контракт offer (§3.4/§14.8): публичная доска v9 +
// импульсы и вскрытые секторы; прежние верхние поля сохранены. Без прогноза
// прибытия и секунд результата (решение 14). Скрытые слои не отдаются.
type acceleratorOfferResponse struct {
	Fingerprint        string                `json:"fingerprint"`
	Game               string                `json:"game"`
	Passport           routegame.Passport    `json:"passport"`
	Board              acceleratorBoard      `json:"board"`
	PingsLeft          int                   `json:"pings_left"`
	Revealed           []acceleratorRevealed `json:"revealed"`
	RemainingS         int                   `json:"remaining_s"`
	MinRemainingBoostS int                   `json:"min_remaining_boost_s"`
	CooldownRemainingS *int64                `json:"cooldown_remaining_s"`
}

// acceleratorBoard — публичный слой поля v9 (§14.8): геометрия, объекты и
// подписи секторов. Цены клеток — в Visible (n×n). Реализованный слой и
// содержимое секторов НЕ входят.
type acceleratorBoard struct {
	N          int                  `json:"n"`
	Start      int                  `json:"start"`
	Finish     int                  `json:"finish"`
	Beacons    []int                `json:"beacons"`
	Visible    []float64            `json:"visible"`
	Lane       []int                `json:"lane"`
	Wall       []int                `json:"wall"`
	Mud        []int                `json:"mud"`
	Gate       []int                `json:"gate"`
	Bridge     []int                `json:"bridge"`
	Current    []acceleratorCurrent `json:"current"`
	DeadEnd    []int                `json:"dead_end"`
	Bottleneck []int                `json:"bottleneck"`
	Sectors    []acceleratorSector  `json:"sectors"`
	Mode       string               `json:"mode"`
}

// acceleratorCurrent — одностороннее течение: клетка и направление (0=+i, 1=−i,
// 2=+j, 3=−j).
type acceleratorCurrent struct {
	Cell int `json:"cell"`
	Dir  int `json:"dir"`
}

// acceleratorSector — публичный вид сектора: клетки, подпись σ и видимое
// окружение. Скрытого содержимого нет.
type acceleratorSector struct {
	Cells        []int  `json:"cells"`
	Sig          int    `json:"sig"`           // 0 Тихий, 1 Ровный, 2 Гулкий
	SigName      string `json:"sig_name"`      // «Тихий»/«Ровный»/«Гулкий»
	Surround     int    `json:"surround"`      // 0 Ничего, 1 Кордон, 2 Обрыв, 3 Мгла, 4 Течение
	SurroundName string `json:"surround_name"` // «Ничего»/«Кордон»/«Обрыв»/«Мгла»/«Течение»
}

// acceleratorRevealed — вскрытый сектор (элемент revealed из БД).
type acceleratorRevealed struct {
	Sector  int    `json:"sector"`
	Content string `json:"content"`
}

// acceleratorScanRequest — тело POST /api/accelerator/scan: fingerprint
// текущего сегмента + индекс сектора.
type acceleratorScanRequest struct {
	Fingerprint string `json:"fingerprint"`
	Sector      int    `json:"sector"`
}

// acceleratorScanResponse — результат вскрытия: содержимое сектора, остаток
// импульсов и актуальный список вскрытых.
type acceleratorScanResponse struct {
	Fingerprint string                `json:"fingerprint"`
	Sector      int                   `json:"sector"`
	Content     string                `json:"content"`
	PingsLeft   int                   `json:"pings_left"`
	Revealed    []acceleratorRevealed `json:"revealed"`
}

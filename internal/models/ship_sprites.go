// internal/models/ship_sprites.go
// Реестр статичных спрайтов кораблей (спека 61b §3.2): 21 PNG из
// web/static/sprites/, порядок фиксирован — он же источник индексов для
// spriteForAgent(id) на клиенте (добавление нового спрайта — только в конец,
// И8). Маппинг legacy SVG-имён → PNG (§4) и палитра перекраски (§5.5).
package models

// ShipSprite — одна запись реестра: id (имя без .png), человеческое имя,
// file (имя файла в web/static/sprites/).
type ShipSprite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	File string `json:"file"`
}

// ShipSprites — реестр из 21 спрайта (спека §3.1/§3.2). Порядок фиксирован:
// индексы используются spriteForAgent(id) = FNV-1a(id) % 21; вставка в
// середину сдвигает индексы и меняет визуал всех агентов — запрещена.
var ShipSprites = []ShipSprite{
	{ID: "anchor", Name: "Якорь", File: "anchor.png"},
	{ID: "book", Name: "Книга", File: "book.png"},
	{ID: "boomerang", Name: "Бумеранг", File: "boomerang.png"},
	{ID: "crater", Name: "Кратер", File: "crater.png"},
	{ID: "crescent", Name: "Полумесяц", File: "crescent.png"},
	{ID: "crystal", Name: "Кристалл", File: "crystal.png"},
	{ID: "drop", Name: "Капля", File: "drop.png"},
	{ID: "egg", Name: "Яйцо", File: "egg.png"},
	{ID: "heart", Name: "Сердце", File: "heart.png"},
	{ID: "helmet", Name: "Шлем", File: "helmet.png"},
	{ID: "jellyfish", Name: "Медуза", File: "jellyfish.png"},
	{ID: "manta", Name: "Манта", File: "manta.png"},
	{ID: "mushroom", Name: "Гриб", File: "mushroom.png"},
	{ID: "octopus", Name: "Осьминог", File: "octopus.png"},
	{ID: "pyramid", Name: "Пирамида", File: "pyramid.png"},
	{ID: "shark", Name: "Акула", File: "shark.png"},
	{ID: "shell", Name: "Ракушка", File: "shell.png"},
	{ID: "spiral", Name: "Спираль", File: "spiral.png"},
	{ID: "star_celestial", Name: "Звезда", File: "star_celestial.png"},
	{ID: "trident", Name: "Трезубец", File: "trident.png"},
	{ID: "volcano", Name: "Вулкан", File: "volcano.png"},
}

// DefaultShipIcon — дефолтный спрайт (Полумесяц, спека §4.1).
const DefaultShipIcon = "crescent.png"

// LegacyShipIconMap — маппинг 21 старого SVG-имени → PNG-имя (спека §4.2,
// биекция). Ключи — полные имена файлов с расширением: именно их писали
// старые дефолты (bootstrap/admin_users/Register, "ship_strela.svg") и
// дашборд через PUT /me/ship-icon. Применяется только при чтении; бэкфилл
// в БД запрещён.
var LegacyShipIconMap = map[string]string{
	"ship_strela.svg":    "boomerang.png",
	"ship_akula.svg":     "shark.png",
	"ship_astra.svg":     "star_celestial.png",
	"ship_barbican.svg":  "egg.png",
	"ship_chas.svg":      "book.png",
	"ship_fregat.svg":    "anchor.png",
	"ship_gonchik.svg":   "drop.png",
	"ship_grail.svg":     "heart.png",
	"ship_klin.svg":      "pyramid.png",
	"ship_lavr.svg":      "mushroom.png",
	"ship_mantikora.svg": "octopus.png",
	"ship_matrica.svg":   "trident.png",
	"ship_mirage.svg":    "jellyfish.png",
	"ship_nosorog.svg":   "helmet.png",
	"ship_orel.svg":      "manta.png",
	"ship_shkval.svg":    "volcano.png",
	"ship_skol.svg":      "crystal.png",
	"ship_talon.svg":     "crescent.png",
	"ship_tytan.svg":     "crater.png",
	"ship_vikhr.svg":     "shell.png",
	"ship_zmei.svg":      "spiral.png",
}

// ShipColorPalette — 9 хроматических цветов перекраски (спека §5.5);
// NULL = «Оригинал» (без перекраски).
var ShipColorPalette = []string{
	"#ef4444", "#f97316", "#eab308", "#22c55e", "#14b8a6",
	"#0ea5e9", "#3b82f6", "#8b5cf6", "#ec4899",
}

// shipSpriteByFile — индекс реестра по имени файла (для ResolveShipIcon
// и валидации PUT /me/ship-icon).
var shipSpriteByFile = func() map[string]ShipSprite {
	m := make(map[string]ShipSprite, len(ShipSprites))
	for _, s := range ShipSprites {
		m[s.File] = s
	}
	return m
}()

// ResolveShipIcon — единое правило маппинга ship_icon при чтении (спека §4):
// legacy SVG-имя → PNG-имя; уже PNG-имя из реестра → как есть; любое другое
// (неизвестное, битое, пустое) → DefaultShipIcon.
func ResolveShipIcon(icon string) string {
	if png, ok := LegacyShipIconMap[icon]; ok {
		return png
	}
	if _, ok := shipSpriteByFile[icon]; ok {
		return icon
	}
	return DefaultShipIcon
}

// IsValidShipIcon — имя файла ∈ реестр (для PUT /me/ship-icon, спека §4:
// принимаются только 21 PNG-имя; legacy-имена и мусор → 400).
func IsValidShipIcon(file string) bool {
	_, ok := shipSpriteByFile[file]
	return ok
}

// IsValidShipColor — цвет ∈ палитры (для PUT /me/ship-color, спека §7:
// NULL или hex из ShipColorPalette, иначе 400).
func IsValidShipColor(color string) bool {
	for _, c := range ShipColorPalette {
		if c == color {
			return true
		}
	}
	return false
}
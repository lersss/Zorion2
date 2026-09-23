// internal/models/ship_sprites.go
// Реестр статичных спрайтов кораблей (спека 61b §3.2): 24 PNG из
// web/static/sprites/ (21 базовый + 3 расовых «люди», проба 2026-09-21),
// порядок фиксирован — он же источник индексов для spriteForAgent(id) на
// клиенте (добавление нового спрайта — только в конец, И8). Маппинг legacy
// SVG-имён → PNG (§4) и палитра перекраски (§5.5).
package models

// ShipSprite — одна запись реестра: id (имя без .png), человеческое имя,
// file (имя файла в web/static/sprites/), race (слаг расы; пусто — легаси/
// нейтральный), ориентация показа — пара (Angle, Flip).
// Пиксели спрайта не поворачиваются: пара применяется при отрисовке
// (спека 2026-09-21-угол-корабля-в-метаданных §6.1). Легаси (базовые 21 и
// расовые «люди») — нули, поля angle/flip в JSON отсутствуют (omitempty).
// race отдаётся ВСЕГДА (без omitempty): у нейтральной записи он пустой, но
// поле обязано присутствовать — клиент строит индекс «раса → файлы» по
// ключу "" (спека 2026-09-23 §6.2, N12).
type ShipSprite struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	File  string  `json:"file"`
	Race  string  `json:"race"`            // слаг расы (спека 2026-09-23 §6.2)
	Angle float64 `json:"angle,omitempty"` // градусы, по часовой, (−180,180]; 0 = нос вправо
	Flip  bool    `json:"flip,omitempty"`  // зеркало, применяется ДО поворота
}

// ShipSprites — реестр из 24 спрайтов (спека §3.1/§3.2). Порядок фиксирован:
// индексы используются spriteForAgent(id) = FNV-1a(id) % len(ShipSprites);
// вставка в середину сдвигает индексы и меняет визуал всех агентов —
// запрещена (новые — только в конец, И8).
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
	// Корабли расы «люди» (проба 2026-09-21): только в конец — индексы
	// существующих 21 не сдвигаются (И8).
	{ID: "race_humans_starship", Name: "Звёздный корабль (люди)", File: "race_humans_starship.png"},
	{ID: "race_humans_cruiser", Name: "Крейсер (люди)", File: "race_humans_cruiser.png"},
	{ID: "race_humans_carrier", Name: "Носитель (люди)", File: "race_humans_carrier.png"},
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

// ResolveShipIcon — единое правило маппинга ship_icon при чтении (спека
// 2026-09-23 §8.1, N5): имя ∈ расовый реестр (включая NeutralShip) → как есть;
// любое другое (легаси-имя, неизвестное, битое, пустое) → DefaultHumanShip.
// Индекс shipSpriteByFile — расовый (race_ship_sprites.go).
func ResolveShipIcon(icon string) string {
	if _, ok := shipSpriteByFile[icon]; ok {
		return icon
	}
	return DefaultHumanShip
}

// IsValidShipIcon — имя файла ∈ расовый реестр (для PUT /me/ship-icon, спека
// 2026-09-23 §8.1: принимаются только имена реестра RaceShipSprites + NeutralShip;
// legacy-имена и мусор → 400).
func IsValidShipIcon(file string) bool {
	_, ok := shipSpriteByFile[file]
	return ok
}

// ShipOrientByFile — пара показа (angle°, flip) спрайта по имени файла
// (ЧК-ship, идея 2026-09-23 §5): значение из расового реестра RaceShipSprites
// через индекс shipSpriteByFile (реестр не дублируется). Файл неизвестен/
// пустой → (0, false) — тот же фолбэк, что у клиентского shipOrientFor.
func ShipOrientByFile(file string) (float64, bool) {
	if s, ok := shipSpriteByFile[file]; ok {
		return s.Angle, s.Flip
	}
	return 0, false
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

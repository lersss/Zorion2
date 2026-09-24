// internal/models/ship_sprites.go
// Общие определения визуала кораблей (спека 61b §3.2/§5.5): тип записи
// реестра, палитра перекраски и резолверы ship_icon. Источник имён — расовый
// реестр RaceShipSprites (race_ship_sprites.go), дефолт — DefaultHumanShip
// (спека 2026-09-23 §8.1, подэтап П4: легаси-реестр из 21 PNG, маппинг
// SVG→PNG и дефолт-полумесяц выпилены).
package models

// ShipSprite — одна запись реестра: id (имя без .png), человеческое имя,
// file (имя файла в web/static/sprites/), race (слаг расы; пусто —
// нейтральный), ориентация показа — пара (Angle, Flip).
// Пиксели спрайта не поворачиваются: пара применяется при отрисовке
// (спека 2026-09-21-угол-корабля-в-метаданных §6.1).
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
	// ScaleHuman — размер корабля в ростах человека (решение создателя
	// 2026-09-25): высота отрисовки спрайта / рост игрока. Не задано/0 → дефолт
	// DefaultShipScaleHuman (см. ShipScaleHuman). Поле — для будущих
	// переопределений (напр. истребитель меньше носителя); у всех записей
	// реестра значение пустое, дефолт даёт аксессор.
	ScaleHuman float64 `json:"scale_human,omitempty"`
}

// DefaultShipScaleHuman — размер корабля «в натуральную величину» по умолчанию
// (решение создателя 2026-09-25): 12 ростов человека. Дефолт даёт аксессор
// ShipScaleHuman — заполнять записи реестра не нужно.
const DefaultShipScaleHuman = 12.0

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

// ShipRaceByFile — слаг расы корабля по имени файла (спека 2026-09-23 §6.5:
// PUT /me/ship-icon принимает только файл расы игрока). Второе значение —
// файл ∈ реестр; у нейтрального корабля раса пустая.
func ShipRaceByFile(file string) (string, bool) {
	s, ok := shipSpriteByFile[file]
	if !ok {
		return "", false
	}
	return s.Race, true
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

// ShipScaleHuman — размер корабля в ростах человека по имени файла (решение
// создателя 2026-09-25): значение записи реестра, если задано > 0; иначе
// дефолт DefaultShipScaleHuman (12). Неизвестный/пустой файл → тот же дефолт
// (фолбэк как у ResolveShipIcon/ShipOrientByFile).
func ShipScaleHuman(file string) float64 {
	if s, ok := shipSpriteByFile[file]; ok && s.ScaleHuman > 0 {
		return s.ScaleHuman
	}
	return DefaultShipScaleHuman
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

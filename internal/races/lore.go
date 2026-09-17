// internal/races/lore.go — лор рас (спека 86a §5.1.1): config/race_lore.json,
// машиночитаемая проекция 22_races.md §3/§4. Загружается при старте тем же
// пакетом, что и каталог; валидация формата как у каталога рас.
package races

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RaceLore — лорная запись расы: family (F1–F9 / "robotic"), character
// («характер» словами), how_live («как живут»), why («зачем»/ниша),
// coexistence («сосуществование»); origin — только у роботов (происхождение:
// люди/самозародившийся/другие био/другие ИИ, 22_races.md §4.2).
type RaceLore struct {
	ID          string `json:"id"`
	Family      string `json:"family"`
	Character   string `json:"character"`
	HowLive     string `json:"how_live"`
	Why         string `json:"why"`
	Coexistence string `json:"coexistence"`
	Origin      string `json:"origin,omitempty"`
}

// loreCatalog — загруженный лор (read-only после LoadLore; загрузка при
// старте до горутин — блокировка не нужна, паттерн catalog).
var loreCatalog []*RaceLore

// loreFamilies — допустимые семейства (22_races.md §2.2/§4).
var loreFamilies = map[string]bool{
	"F1": true, "F2": true, "F3": true, "F4": true, "F5": true,
	"F6": true, "F7": true, "F8": true, "F9": true, "robotic": true,
}

// LoadLore — загружает лор рас из файла и валидирует каждую запись:
// id есть в каталоге, family из набора, текстовые поля непусты; у роботов
// (family = robotic) обязателен origin, у био-рас origin пуст. При ошибке
// текущий лор не трогается.
func LoadLore(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("race lore: %w", err)
	}
	var file struct {
		Races []*RaceLore `json:"races"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("race lore %s: %w", absPath, err)
	}
	for i, l := range file.Races {
		if err := l.Validate(); err != nil {
			return fmt.Errorf("race lore %s: запись %d (%s): %w", absPath, i+1, l.ID, err)
		}
	}
	loreCatalog = file.Races
	return nil
}

// LoreCatalog — текущий лор рас (read-only; пуст, если не загружен).
func LoreCatalog() []*RaceLore {
	return loreCatalog
}

// LoreByID — лор расы по ключу (nil, если нет).
func LoreByID(id string) *RaceLore {
	for _, l := range loreCatalog {
		if l.ID == id {
			return l
		}
	}
	return nil
}

// Validate — инварианты лора (спека 86a §5.1.1): id есть в каталоге рас,
// family из набора F1–F9/robotic, character/how_live/why/coexistence непусты;
// у роботов origin обязателен, у био-рас — пуст (происхождение био-рас не
// описано, 22_races.md §3).
func (l *RaceLore) Validate() error {
	if l.ID == "" {
		return fmt.Errorf("id пуст")
	}
	if ByID(l.ID) == nil {
		return fmt.Errorf("id %q нет в каталоге рас", l.ID)
	}
	if !loreFamilies[l.Family] {
		return fmt.Errorf("family = %q, ожидается F1–F9|robotic", l.Family)
	}
	for name, v := range map[string]string{
		"character": l.Character, "how_live": l.HowLive,
		"why": l.Why, "coexistence": l.Coexistence,
	} {
		if v == "" {
			return fmt.Errorf("поле %s пусто", name)
		}
	}
	if l.Family == "robotic" && l.Origin == "" {
		return fmt.Errorf("робот: origin обязателен (происхождение, 22_races.md §4.2)")
	}
	if l.Family != "robotic" && l.Origin != "" {
		return fmt.Errorf("био-раса: origin должен быть пуст (происхождение био-рас не описано, §3)")
	}
	return nil
}
// internal/audit/planet/view.go
package planet

import (
	"encoding/json"
	"log"

	"zorion/internal/audit"
)

// ==================== СТРУКТУРА СТРОКИ ИЗ БД ====================

// Row — строка таблицы planets из БД.
// Минимум полей, которые нужны аудитору.
type Row struct {
	ID      string
	WorldID string
	Name    string
	Data    []byte
}

// ==================== ПРЕДСТАВЛЕНИЕ ПЛАНЕТЫ ====================

// View — представление планеты для аудита.
// Заполняется один раз из JSON, чтобы правила не парсили map заново.
type View struct {
	ID      string
	Name    string
	WorldID string

	// Физика
	Size         float64
	Mass         float64
	Density      float64
	Temperature  float64
	WaterPercent float64

	// Типы и флаги
	Type            string
	SurfaceDominant string
	Climate         string
	Atmosphere      string
	Hydrosphere     string
	Biosphere       string

	IsGasGiant    bool
	IsRadioactive bool
	Life          bool

	// Композиции
	Surface    map[string]float64
	Subterrain map[string]float64

	// Ядро (nil если нет)
	Core *CoreView

	// Спутники
	Satellites []SatelliteView

	// Сырой JSON (на случай, если правило хочет свои поля)
	Raw map[string]interface{}
}

// CoreView — ядро планеты для проверок.
type CoreView struct {
	Type          string
	MassPercent   float64
	Activity      float64
	Radioactivity float64
	Age           float64
	IsActive      bool
	IsMetallic    bool
}

// SatelliteView — спутник газового гиганта для проверок.
type SatelliteView struct {
	ID           string
	Name         string
	Mass         float64
	Size         float64
	Temperature  float64
	WaterPercent float64
	Atmosphere   string
	Habitable    bool
	Life         bool
}

// ==================== ИНТЕРФЕЙС ДЛЯ ДВИЖКА ====================

// GetID — реализация интерфейса audit.ContextKey.
func (v *View) GetID() string { return v.ID }

// GetName — реализация интерфейса audit.ContextKey.
func (v *View) GetName() string { return v.Name }

// GetContextID — реализация интерфейса audit.ContextKey.
func (v *View) GetContextID() string { return v.WorldID }

// GetEntityType — реализация интерфейса audit.ContextKey.
func (v *View) GetEntityType() string { return "planet" }

// ==================== ПАРСИНГ ====================

// ParseAll — превращает строки из БД в список View.
// Строки с битым JSON пропускаются (с логом).
func ParseAll(rows []Row) []*View {
	views := make([]*View, 0, len(rows))
	for _, row := range rows {
		var data map[string]interface{}
		if err := json.Unmarshal(row.Data, &data); err != nil {
			log.Printf("⚠️ audit/planet: JSON error for %s: %v", row.ID, err)
			continue
		}
		v := Parse(data)
		// Обогащаем ID/Name/WorldID, если их нет в JSON
		if v.ID == "" {
			v.ID = row.ID
		}
		if v.Name == "" {
			v.Name = row.Name
		}
		if v.WorldID == "" {
			v.WorldID = row.WorldID
		}
		views = append(views, v)
	}
	return views
}

// Parse — превращает JSON-объект планеты в View.
func Parse(data map[string]interface{}) *View {
	v := &View{
		Raw:        data,
		Surface:    map[string]float64{},
		Subterrain: map[string]float64{},
	}

	// Идентификация
	v.ID = aStr(data, "id")
	v.Name = aStr(data, "name")
	v.WorldID = aStr(data, "world_id")

	// Физика
	v.Size = aFloat(data, "size")
	v.Mass = aFloat(data, "mass")
	v.Density = aFloat(data, "density")
	v.Temperature = aFloat(data, "temperature")
	v.WaterPercent = aFloat(data, "water_percent")

	// Типы и флаги
	v.Type = aStr(data, "type")
	v.SurfaceDominant = aStr(data, "surface_dominant")
	v.Climate = aStr(data, "climate")
	v.Atmosphere = aStr(data, "atmosphere")
	v.Hydrosphere = aStr(data, "hydrosphere")
	v.Biosphere = aStr(data, "biosphere")

	v.IsGasGiant = aBool(data, "is_gas_giant")
	v.IsRadioactive = aBool(data, "radioactive")
	v.Life = aBool(data, "life")

	// Композиции
	v.Surface = aFloatMap(data, "surface_composition")
	v.Subterrain = aFloatMap(data, "subterrain_composition")

	// Ядро
	v.Core = parseCore(data)

	// Спутники
	v.Satellites = parseSatellites(data)

	return v
}

func parseCore(data map[string]interface{}) *CoreView {
	raw, ok := data["core"].(map[string]interface{})
	if !ok {
		return nil
	}
	return &CoreView{
		Type:          aStr(raw, "type"),
		MassPercent:   aFloat(raw, "mass_percent"),
		Activity:      aFloat(raw, "activity"),
		Radioactivity: aFloat(raw, "radioactivity"),
		Age:           aFloat(raw, "age"),
		IsActive:      aBool(raw, "is_active"),
		IsMetallic:    aBool(raw, "is_metallic"),
	}
}

func parseSatellites(data map[string]interface{}) []SatelliteView {
	raw, ok := data["satellites"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]SatelliteView, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, SatelliteView{
			ID:           aStr(m, "id"),
			Name:         aStr(m, "name"),
			Mass:         aFloat(m, "mass"),
			Size:         aFloat(m, "size"),
			Temperature:  aFloat(m, "temperature"),
			WaterPercent: aFloat(m, "water_percent"),
			Atmosphere:   aStr(m, "atmosphere"),
			Habitable:    aBool(m, "habitable"),
			Life:         aBool(m, "life"),
		})
	}
	return result
}

// ==================== ХЕЛПЕРЫ СОЗДАНИЯ ISSUE ====================

// newIssue — создаёт Issue без details.
func newIssue(v *View, code, severity, description string) audit.Issue {
	return audit.Issue{
		EntityID:    v.ID,
		EntityType:  "planet",
		EntityName:  v.Name,
		ContextID:   v.WorldID,
		Code:        code,
		Severity:    severity,
		Description: description,
	}
}

// newIssueWithDetails — создаёт Issue с details.
func newIssueWithDetails(
	v *View,
	code, severity, description string,
	details map[string]interface{},
) audit.Issue {
	return audit.Issue{
		EntityID:    v.ID,
		EntityType:  "planet",
		EntityName:  v.Name,
		ContextID:   v.WorldID,
		Code:        code,
		Severity:    severity,
		Description: description,
		Details:     details,
	}
}

// ==================== ХЕЛПЕРЫ ДОСТУПА К JSON ====================

func aStr(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

func aFloat(data map[string]interface{}, key string) float64 {
	if v, ok := data[key].(float64); ok {
		return v
	}
	return 0
}

func aBool(data map[string]interface{}, key string) bool {
	if v, ok := data[key].(bool); ok {
		return v
	}
	return false
}

func aFloatMap(data map[string]interface{}, key string) map[string]float64 {
	result := map[string]float64{}
	raw, ok := data[key].(map[string]interface{})
	if !ok {
		return result
	}
	for k, v := range raw {
		if f, ok := v.(float64); ok {
			result[k] = f
		}
	}
	return result
}
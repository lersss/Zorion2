// internal/models/ship_visual.go
// Схема корабля: ровно одна деталь каждой категории + один основной цвет
// (спека 99.2.15 §2, инварианты И1/И2). Акценты — часть деталей, в схему
// не входят (И3).
package models

import "encoding/json"

// ShipVisual — схема корабля игрока (users.ship_visual) и результат
// детерминированной сборки агента (assemblyFromSeed, спека §7.1).
type ShipVisual struct {
	Color string            `json:"color"` // один цвет из палитры (§4)
	Parts map[string]string `json:"parts"` // category → id детали (всегда 5, И2)
}

// ShipVisualFromJSON — парсинг JSONB-колонки users.ship_visual.
func ShipVisualFromJSON(data []byte) (*ShipVisual, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var v ShipVisual
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
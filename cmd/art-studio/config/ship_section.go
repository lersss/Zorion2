package config

import (
	"errors"
	"strings"
)

// ShipLore — машинная проекция лор-файла корабля (docs/gamedesign/races/ships/
// <slug>.md): базовые texture/silhouette/blocked + семейство из шапки + типы
// корабля (пусто у обычных рас).
type ShipLore struct {
	Family     string
	Texture    string
	Silhouette string
	Blocked    []string
	Types      []ShipType
}

// ParseShipSection извлекает из лор-файла корабля машинную проекцию раздела
// «## Корабль (внешний вид)» (спека 2026-09-20-ships-races-generator §4.1/§6.1
// п.3): базовые маркеры «**Для генератора (texture):**» /
// «(silhouette)» / «(blocked)» (split по запятой, trim) + семейство из шапки
// («**Семейство:** F1 Водные» → «F1»). Типы корабля — маркеры
// «**Для генератора (type <имя> texture):**» / «(type <имя> silhouette)» /
// «(type <имя> blocked)»: типы собираются в порядке первого появления, поля
// типа наследуют базовые. Маркеры ищутся только внутри раздела «## Корабль
// (внешний вид)». Ошибка, если раздела/маркеров нет (нет ни базовой texture,
// ни одного типа с texture; для расы без типов дополнительно требуются
// silhouette и blocked — как было).
func ParseShipSection(md string) (ShipLore, error) {
	var lore ShipLore
	lines := strings.Split(md, "\n")
	inSection := false
	typeIdx := map[string]int{}
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "**Семейство:**") && lore.Family == "" {
			rest := strings.TrimSpace(strings.TrimPrefix(t, "**Семейство:**"))
			if i := strings.IndexAny(rest, " \t"); i > 0 {
				lore.Family = rest[:i]
			} else if rest != "" {
				lore.Family = rest
			}
			continue
		}
		if strings.HasPrefix(t, "## ") {
			inSection = strings.TrimSpace(strings.TrimPrefix(t, "## ")) == "Корабль (внешний вид)"
			continue
		}
		if !inSection {
			continue
		}
		const marker = "**Для генератора ("
		if !strings.HasPrefix(t, marker) {
			continue
		}
		rest := strings.TrimPrefix(t, marker)
		end := strings.Index(rest, "):**")
		if end < 0 {
			continue
		}
		inside := strings.TrimSpace(rest[:end])
		val := strings.TrimSpace(rest[end+len("):**"):])
		switch {
		case inside == "texture":
			lore.Texture = val
		case inside == "silhouette":
			lore.Silhouette = val
		case inside == "blocked":
			lore.Blocked = splitShipTokens(val)
		case strings.HasPrefix(inside, "type "):
			parts := strings.Fields(inside)
			if len(parts) != 3 {
				continue
			}
			name, field := parts[1], parts[2]
			idx, ok := typeIdx[name]
			if !ok {
				lore.Types = append(lore.Types, ShipType{Type: name})
				idx = len(lore.Types) - 1
				typeIdx[name] = idx
			}
			switch field {
			case "texture":
				lore.Types[idx].Texture = val
			case "silhouette":
				lore.Types[idx].Silhouette = val
			case "blocked":
				lore.Types[idx].Blocked = splitShipTokens(val)
			}
		}
	}
	hasTexture := strings.TrimSpace(lore.Texture) != ""
	for _, ty := range lore.Types {
		if strings.TrimSpace(ty.Texture) != "" {
			hasTexture = true
		}
	}
	if !hasTexture {
		return ShipLore{}, errors.New(`у корабля нет раздела "Корабль (внешний вид)" с маркерами texture/silhouette/blocked`)
	}
	if len(lore.Types) == 0 && (lore.Silhouette == "" || len(lore.Blocked) == 0) {
		return ShipLore{}, errors.New(`у корабля нет раздела "Корабль (внешний вид)" с маркерами texture/silhouette/blocked`)
	}
	return lore, nil
}

// splitShipTokens — токены через запятую (trim, пустые пропускаются).
func splitShipTokens(s string) []string {
	var out []string
	for _, tok := range strings.Split(s, ",") {
		if tok = strings.TrimSpace(tok); tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

package config

import (
	"errors"
	"strings"
)

// ParseShipSection извлекает из лор-файла корабля (docs/gamedesign/races/ships/
// <slug>.md) машинную проекцию раздела «## Корабль (внешний вид)» (спека
// 2026-09-20-ships-races-generator §4.1/§6.1 п.3): маркеры
// «**Для генератора (texture):**» / «**Для генератора (silhouette):**» /
// «**Для генератора (blocked):**» (split по запятой, trim) + семейство из
// шапки («**Семейство:** F1 Водные» → «F1»). Маркеры ищутся только внутри
// раздела «## Корабль (внешний вид)». Ошибка, если раздела или любого из
// маркеров нет.
func ParseShipSection(md string) (texture, silhouette string, blocked []string, family string, err error) {
	lines := strings.Split(md, "\n")
	inSection := false
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "**Семейство:**") && family == "" {
			rest := strings.TrimSpace(strings.TrimPrefix(t, "**Семейство:**"))
			if i := strings.IndexAny(rest, " \t"); i > 0 {
				family = rest[:i]
			} else if rest != "" {
				family = rest
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
		if strings.HasPrefix(t, "**Для генератора (texture):**") {
			texture = strings.TrimSpace(strings.TrimPrefix(t, "**Для генератора (texture):**"))
			continue
		}
		if strings.HasPrefix(t, "**Для генератора (silhouette):**") {
			silhouette = strings.TrimSpace(strings.TrimPrefix(t, "**Для генератора (silhouette):**"))
			continue
		}
		if strings.HasPrefix(t, "**Для генератора (blocked):**") {
			rest := strings.TrimSpace(strings.TrimPrefix(t, "**Для генератора (blocked):**"))
			for _, tok := range strings.Split(rest, ",") {
				if tok = strings.TrimSpace(tok); tok != "" {
					blocked = append(blocked, tok)
				}
			}
		}
	}
	if texture == "" || silhouette == "" || len(blocked) == 0 {
		return "", "", nil, "", errors.New(`у корабля нет раздела "Корабль (внешний вид)" с маркерами texture/silhouette/blocked`)
	}
	return texture, silhouette, blocked, family, nil
}
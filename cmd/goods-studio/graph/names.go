package graph

import (
	"strings"

	"zorion/cmd/goods-studio/model"
)

// NormalizeName — нормализация имени: lowercase, trim, схлопывание пробелов
// (спека 99a.1 §6.3). Ресурсы и товары — одно пространство имён.
func NormalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Join(strings.Fields(s), " ")
}

// NameIndex — карта нормализованное имя → индекс в goods.
func NameIndex(goods []model.Good) map[string]int {
	m := make(map[string]int, len(goods))
	for i := range goods {
		m[NormalizeName(goods[i].Name)] = i
	}
	return m
}

// FindByName — товар по нормализованному имени (nil, если нет).
func FindByName(name string, goods []model.Good) *model.Good {
	norm := NormalizeName(name)
	for i := range goods {
		if NormalizeName(goods[i].Name) == norm {
			return &goods[i]
		}
	}
	return nil
}
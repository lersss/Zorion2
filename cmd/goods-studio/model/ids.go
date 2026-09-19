package model

import "fmt"

// NextGoodID — следующий свободный id товара (g1, g2, ...).
func NextGoodID(goods []Good) string {
	max := 0
	for _, g := range goods {
		var n int
		if _, err := fmt.Sscanf(g.ID, "g%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("g%d", max+1)
}

// NextCategoryID — следующий свободный id категории (c1, c2, ...).
func NextCategoryID(cats []Category) string {
	max := 0
	for _, c := range cats {
		var n int
		if _, err := fmt.Sscanf(c.ID, "c%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("c%d", max+1)
}
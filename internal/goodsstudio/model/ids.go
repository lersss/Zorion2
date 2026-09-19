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
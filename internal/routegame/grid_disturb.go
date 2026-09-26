// internal/routegame/grid_disturb.go
// Помеха зонда: неудачная разведка активирует неустойчивость сектора
// (спека 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md, §14.4 R2,
// §14.7 ветка `ping_destabilize`): клетки вскрытого `unstable`-сектора и подход
// к ним (4-соседи) становятся дорогими (TrapCost) в реализованном слое. Копия
// механизма эталонного харнесса v9 (`destabilizedV9`); поле не мутируется.
package routegame

// DestabilizeGridField возвращает копию поля, где клетки перечисленных секторов
// (обычно — вскрытые `unstable`) и их 4-соседи стоят TrapCost в реализованном
// слое. Старт/финиш/маяки/шлюзы/мосты как соседи не дорожают (как в харнессе).
// Неизвестный индекс игнорируется; пустой список — поле без изменений.
func DestabilizeGridField(field GridField, unstableSectors []int) GridField {
	if len(unstableSectors) == 0 {
		return field
	}
	g := field
	r := append([]float64(nil), field.realized...)
	for _, idx := range unstableSectors {
		if idx < 0 || idx >= len(field.Sectors) {
			continue
		}
		for _, c := range field.Sectors[idx].Cells {
			r[c] = field.TrapCost
			// помеха на подходе: соседние клетки тоже дорогие.
			for _, st := range field.neighbors4(c) {
				n := st.cell
				if n == field.Start || n == field.Finish || isBeaconGrid(&field, n) || field.Gate[n] || field.Bridge[n] {
					continue
				}
				if r[n] < field.TrapCost {
					r[n] = field.TrapCost
				}
			}
		}
	}
	g.realized = r
	return g
}

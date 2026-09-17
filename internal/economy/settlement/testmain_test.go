package settlement

import (
	"os"
	"testing"

	"zorion/internal/races"
)

// TestMain — загружает каталог рас: генерация расовых R-кривых
// (deriveRaceCurves, 99.2.23 §3.3) и авто-инициализация (LoadRaceBalancer)
// требуют карточки из config/races.json. Без каталога расы не выводятся.
func TestMain(m *testing.M) {
	if err := races.LoadCatalog("../../../config/races.json"); err != nil {
		panic("settlement tests: каталог рас не загружен: " + err.Error())
	}
	os.Exit(m.Run())
}
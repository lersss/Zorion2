package planet

import (
	"os"
	"testing"

	"zorion/internal/races"
)

// TestMain — загружает каталог рас: пригодность для людей (Settleable,
// тег inhabited) считается через races.HumansSuitable (65a) и требует
// карточку humans из config/races.json. Без каталога HumansSuitable
// возвращает false — тесты пригодности молча «прошли» бы неверно.
func TestMain(m *testing.M) {
	if err := races.LoadCatalog("../../../config/races.json"); err != nil {
		panic("planet tests: каталог рас не загружен: " + err.Error())
	}
	os.Exit(m.Run())
}
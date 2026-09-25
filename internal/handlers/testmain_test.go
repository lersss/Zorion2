// internal/handlers/testmain_test.go
// Реестр кораблей игры — файл данных (спека
// 2026-09-25-арт-студия-удаление-кораблей-из-игры.md §3.5): тесты пакета
// читают непустой реестр (резолверы ship_icon, ship_options).
package handlers

import (
	"os"
	"testing"

	"zorion/internal/models"
)

func TestMain(m *testing.M) {
	if err := models.LoadShipRegistry("../../config/ships_registry.json"); err != nil {
		panic("LoadShipRegistry: " + err.Error())
	}
	os.Exit(m.Run())
}

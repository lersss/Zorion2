// Арт-студия генерации аватаров рас (спека 67a.1).
// Отдельный бинарь zorion-art-studio.exe, НЕ трогает игровой сервер.
// Сборка: go build -o zorion-art-studio.exe ./cmd/art-studio
package main

import (
	"embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"zorion/cmd/art-studio/comfy"
	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/generator"
	"zorion/cmd/art-studio/handlers"
)

//go:embed web/index.html
var uiFS embed.FS

func main() {
	cfg, err := config.LoadStudio("config/art/studio.json")
	if err != nil {
		fmt.Println("Ошибка конфига studio.json:", err)
		os.Exit(1)
	}
	forms, err := config.LoadForms("config/art/forms.json")
	if err != nil {
		fmt.Println("Ошибка конфига forms.json:", err)
		os.Exit(1)
	}
	families, err := config.LoadFamilies("config/art/families.json")
	if err != nil {
		fmt.Println("Ошибка конфига families.json:", err)
		os.Exit(1)
	}
	humans, err := config.LoadHumans("config/art/humans.json")
	if err != nil {
		fmt.Println("Ошибка конфига humans.json:", err)
		os.Exit(1)
	}
	ships, err := config.LoadShips("config/art/ships.json")
	if err != nil {
		fmt.Println("Ошибка конфига ships.json:", err)
		os.Exit(1)
	}
	shipDict, err := config.LoadShipDict("config/art/ship_dict.json")
	if err != nil {
		fmt.Println("Ошибка конфига ship_dict.json:", err)
		os.Exit(1)
	}

	// защита от дублей: bind порта при старте (спека 67a.1 §9.1)
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	ln, err := bindPort(addr)
	if err != nil {
		fmt.Printf("Арт-студия уже запущена (порт %d занят)\n", cfg.Port)
		os.Exit(1)
	}

	// сброс залипших статусов и stop.flag: рестарт убивает джобы, а status.json
	// мог остаться running:true — иначе генерация заблокирована навсегда
	for _, pool := range []string{"races_pool", "humans_pool", "ships_pool"} {
		poolDir := filepath.Join(cfg.PoolRoot, pool)
		generator.WriteStatus(poolDir, generator.Status{})
		os.Remove(filepath.Join(poolDir, "stop.flag"))
	}

	comfyClient := comfy.NewClient(cfg.ComfyURL, cfg.PollIntervalS, cfg.HistoryTimeoutS)
	runner := generator.NewRunner(cfg, forms, families, humans, comfyClient)
	runner.SetShips(ships, shipDict, "config/races.json")
	uiHTML, err := uiFS.ReadFile("web/index.html")
	if err != nil {
		fmt.Println("Ошибка чтения UI:", err)
		os.Exit(1)
	}
	srv := handlers.NewServer(cfg, forms, families, humans, runner, uiHTML, "config/art/families.json")
	srv.SetShips(ships, shipDict, "config/art/ships.json")

	fmt.Printf("Арт-студия: http://127.0.0.1:%d\n", cfg.Port)
	fmt.Printf("Пулы: %s\n", cfg.PoolRoot)
	if err := http.Serve(ln, srv.Handler()); err != nil {
		fmt.Println("Ошибка сервера:", err)
		os.Exit(1)
	}
}

// bindPort занимает порт для защиты от дублей: второй запуск падает
// (аналог socket.bind в прототипе, спека 67a.1 §9.1).
func bindPort(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}
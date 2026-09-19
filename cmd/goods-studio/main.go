// Студия товаров (спека 99a.1): отдельный бинарь zorion-goods-studio.exe,
// НЕ трогает игровой сервер (кроме импорта internal/resource на чтение).
// Сборка: go build -o zorion-goods-studio.exe ./cmd/goods-studio
package main

import (
	"embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"zorion/cmd/goods-studio/ai"
	"zorion/cmd/goods-studio/catalog"
	"zorion/cmd/goods-studio/config"
	"zorion/cmd/goods-studio/handlers"
	"zorion/cmd/goods-studio/model"
)

//go:embed web/index.html
var uiFS embed.FS

func main() {
	cfg, err := config.LoadStudio("config/goods/studio.json")
	if err != nil {
		fmt.Println("Ошибка конфига studio.json:", err)
		os.Exit(1)
	}

	// защита от дублей: bind порта при старте (спека 99a.1 §12.1)
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("Студия товаров уже запущена (порт %d занят)\n", cfg.Port)
		os.Exit(1)
	}

	statePath := filepath.Join(cfg.DataDir, "state.json")
	st, err := model.LoadState(statePath)
	if err != nil {
		fmt.Println("Ошибка state.json:", err)
		os.Exit(1)
	}
	// импорт каталогов ресурсов (спека 99a.1 §5.3): новые добавляются,
	// существующие обновляют имя/категорию, статусы/бан сохраняются
	resources := catalog.ImportResources()
	catalog.MergeResources(st, resources)
	if err := model.SaveState(statePath, st); err != nil {
		fmt.Println("Ошибка записи state.json:", err)
		os.Exit(1)
	}

	uiHTML, err := uiFS.ReadFile("web/index.html")
	if err != nil {
		fmt.Println("Ошибка чтения UI:", err)
		os.Exit(1)
	}
	aiClient := ai.NewClient(cfg.OpenCodeURL, cfg.Model, time.Duration(cfg.TimeoutS)*time.Second, cfg.MaxRetries)
	srv := handlers.NewServer(cfg, st, statePath, aiClient, uiHTML)

	fmt.Printf("Студия товаров: http://127.0.0.1:%d\n", cfg.Port)
	fmt.Printf("Состояние: %s\n", statePath)
	fmt.Printf("Модель ИИ: %s\n", cfg.Model)
	if err := http.Serve(ln, srv.Handler()); err != nil {
		fmt.Println("Ошибка сервера:", err)
		os.Exit(1)
	}
}
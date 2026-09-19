// Package handlers — HTTP-слой студии товаров (спека 99a.1 §9):
// роутинг, статика UI, no-cache заголовки, общие хелперы.
package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"zorion/cmd/goods-studio/ai"
	"zorion/cmd/goods-studio/config"
	"zorion/internal/goodsstudio/model"
)

// Server — HTTP-сервер студии товаров.
type Server struct {
	cfg       *config.StudioConfig
	state     *model.State
	statePath string
	ai        *ai.Client
	uiHTML    []byte

	mu         sync.RWMutex // общее состояние (AGENTS.md §0)
	generating bool         // одна активная ИИ-генерация (TryStart, §7.1)
	lastReport []string     // отчёт последнего «заполнить комплектующие»
}

// NewServer создаёт Server. uiHTML — содержимое web/index.html (embed в main).
func NewServer(cfg *config.StudioConfig, st *model.State, statePath string, aiClient *ai.Client, uiHTML []byte) *Server {
	return &Server{
		cfg:       cfg,
		state:     st,
		statePath: statePath,
		ai:        aiClient,
		uiHTML:    uiHTML,
	}
}

// Handler возвращает роутер со всеми эндпоинтами (спека 99a.1 §9).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/resources", s.handleResources)
	mux.HandleFunc("/api/categories", s.handleCategories)
	mux.HandleFunc("/api/categories/", s.handleCategoryByID)
	mux.HandleFunc("/api/goods", s.handleGoods)
	mux.HandleFunc("/api/goods/", s.handleGoodByID)
	mux.HandleFunc("/api/export", s.handleExport)
	mux.HandleFunc("/api/import", s.handleImport)
	mux.HandleFunc("/api/validate", s.handleValidate)
	return noCache(mux)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.uiHTML)
}

// --- общие хелперы ---

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// tryStartFill — атомарный старт генерации (TryStart, спека 99a.1 §7.1).
func (s *Server) tryStartFill() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generating {
		return false
	}
	s.generating = true
	s.lastReport = nil
	return true
}

// finishFill — завершение генерации (успех или ошибка).
func (s *Server) finishFill(report []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generating = false
	s.lastReport = report
}

// persist сохраняет state.json атомарно (под блокировкой вызывающего).
func (s *Server) persist() {
	model.SaveState(s.statePath, s.state)
}

// nowISO — текущее время ISO8601 (UTC).
func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
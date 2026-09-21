// Package handlers — HTTP-слой арт-студии (спека 67a.1 §6):
// роутинг, статика UI, no-cache заголовки, общие хелперы.
package handlers

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/generator"
)

// Server — HTTP-сервер арт-студии.
type Server struct {
	cfg          *config.StudioConfig
	forms        *config.FormsConfig
	families     config.FamiliesConfig
	familiesPath string            // config/art/families.json — запись+reload при пересборке промпта
	humans       *config.HumansConfig
	runner       *generator.Runner
	uiHTML       []byte
	raceSlug     map[string]string // name (races.json) → slug (id) для лора рас
	raceNameBySlug map[string]string // slug (id) → name (races.json) для вкладки кораблей
	loreDir      string            // каталог docs/gamedesign/races/
	famMu        sync.RWMutex      // защита families при reload (пересборка промпта)

	ships     config.ShipsConfig     // вкладка «Корабли рас» (nil — не подключена)
	shipDict  *config.ShipDictConfig // словари кораблей (nil — не подключены)
	shipsPath string                 // config/art/ships.json — запись+reload при пересборке
	shipsMu   sync.RWMutex           // защита ships при reload

	shipsDirPath string // каталог races/ships/ (в тестах — относительный путь)
	racesPath    string // config/races.json (валидация ключей ships.json; в тестах — относительный)
}

// NewServer создаёт Server. uiHTML — содержимое web/index.html (embed в main).
// familiesPath — путь к families.json (для /rebuild-prompt: запись + reload).
// При старте читает config/races.json (маппинг name→slug для /race-info).
func NewServer(cfg *config.StudioConfig, forms *config.FormsConfig, families config.FamiliesConfig, humans *config.HumansConfig, runner *generator.Runner, uiHTML []byte, familiesPath string) *Server {
	return &Server{
		cfg:          cfg,
		forms:        forms,
		families:     families,
		familiesPath: familiesPath,
		humans:       humans,
		runner:       runner,
		uiHTML:       uiHTML,
		raceSlug:     loadRaceSlug("config/races.json"),
		raceNameBySlug: loadRaceNames("config/races.json"),
		loreDir:      "docs/gamedesign/races",
	}
}

// SetShips подключает конфиги кораблей (вкладка «Корабли рас»).
func (s *Server) SetShips(ships config.ShipsConfig, dict *config.ShipDictConfig, shipsPath string) {
	s.shipsMu.Lock()
	defer s.shipsMu.Unlock()
	s.ships = ships
	s.shipDict = dict
	s.shipsPath = shipsPath
}

// shipsEntry — запись корабля расы (чтение под shipsMu: reloadShips может
// заменить конфиг в памяти).
func (s *Server) shipsEntry(slug string) (config.ShipEntry, bool) {
	s.shipsMu.RLock()
	defer s.shipsMu.RUnlock()
	if s.ships == nil {
		return config.ShipEntry{}, false
	}
	e, ok := s.ships[slug]
	return e, ok
}

// shipDictRef — словарь кораблей под shipsMu.
func (s *Server) shipDictRef() *config.ShipDictConfig {
	s.shipsMu.RLock()
	defer s.shipsMu.RUnlock()
	return s.shipDict
}

// reloadShips перечитывает ships.json с диска и заменяет конфиг в памяти
// (пересборка промпта: машинная проекция texture/silhouette/blocked обновилась).
func (s *Server) reloadShips(path string) error {
	ships, err := config.LoadShipsRaces(path, s.racesPathOr("config/races.json"))
	if err != nil {
		return err
	}
	s.shipsMu.Lock()
	s.ships = ships
	s.shipsMu.Unlock()
	return nil
}

// family возвращает семейство по id (чтение под famMu: reloadFamilies может
// заменить конфиг в памяти — без мьютекса concurrent map read/write).
func (s *Server) family(famID string) (config.Family, bool) {
	s.famMu.RLock()
	defer s.famMu.RUnlock()
	f, ok := s.families[famID]
	return f, ok
}

// reloadFamilies перечитывает families.json с диска и заменяет конфиг в памяти
// (пересборка промпта: машинная проекция appearance/blocked обновилась).
func (s *Server) reloadFamilies(path string) error {
	fam, err := config.LoadFamilies(path)
	if err != nil {
		return err
	}
	s.famMu.Lock()
	s.families = fam
	s.famMu.Unlock()
	return nil
}

// Handler возвращает роутер со всеми эндпоинтами (спека 67a.1 §6).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/list", s.handleList)
	mux.HandleFunc("/races", s.handleRaces)
	mux.HandleFunc("/refs", s.handleRefs)
	mux.HandleFunc("/ref", s.handleRef)
	mux.HandleFunc("/race-info", s.handleRaceInfo)
	mux.HandleFunc("/refimg", s.handleRefImg)
	mux.HandleFunc("/gallery", s.handleGallery)
	mux.HandleFunc("/gallery/img", s.handleGalleryImg)
	mux.HandleFunc("/refcand/", s.handleRefCand)
	mux.HandleFunc("/refset", s.handleRefSet)
	mux.HandleFunc("/refsetpool", s.handleRefSetPool)
	mux.HandleFunc("/refsetaccepted", s.handleRefSetAccepted)
	mux.HandleFunc("/refclear", s.handleRefClear)
	mux.HandleFunc("/genvar", s.handleGenVar)
	mux.HandleFunc("/genref", s.handleGenRef)
	mux.HandleFunc("/prompt", s.handlePrompt)
	mux.HandleFunc("/rebuild-prompt", s.handleRebuildPrompt)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/stop", s.handleStop)
	mux.HandleFunc("/clearpool", s.handleClearPool)
	mux.HandleFunc("/act", s.handleAct)
	mux.HandleFunc("/crop", s.handleCrop)
	mux.HandleFunc("/preview", s.handlePreview)
	mux.HandleFunc("/img/", s.handleImg)
	mux.HandleFunc("/humans/gen", s.handleHumansGen)
	mux.HandleFunc("/humans/list", s.handleHumansList)
	mux.HandleFunc("/humans/act", s.handleHumansAct)
	mux.HandleFunc("/humans/crop", s.handleHumansCrop)
	mux.HandleFunc("/humans/preview", s.handleHumansPreview)
	mux.HandleFunc("/humans/img/", s.handleHumansImg)
	mux.HandleFunc("/ships/races", s.handleShipsRaces)
	mux.HandleFunc("/ships/info", s.handleShipsInfo)
	mux.HandleFunc("/ships/prompt", s.handleShipsPrompt)
	mux.HandleFunc("/ships/rebuild", s.handleShipsRebuild)
	mux.HandleFunc("/ships/gen", s.handleShipsGen)
	mux.HandleFunc("/ships/genbatch", s.handleShipsGenBatch)
	mux.HandleFunc("/ships/list", s.handleShipsList)
	mux.HandleFunc("/ships/img/", s.handleShipsImg)
	mux.HandleFunc("/ships/act", s.handleShipsAct)
	mux.HandleFunc("/ships/vote", s.handleShipsVote)
	mux.HandleFunc("/ships/auto", s.handleShipsAuto)
	mux.HandleFunc("/ships/preview", s.handleShipsPreview)
	mux.HandleFunc("/ships/accepted", s.handleShipsAccepted)
	mux.HandleFunc("/ships/status", s.handleStatus)
	mux.HandleFunc("/ships/stop", s.handleStop)
	mux.HandleFunc("/checkpoint", s.handleCheckpoint)
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

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.runner.ReadStatusAny())
}

// GetCheckpoint — текущий чекпоинт SDXL (делегирует Runner).
func (s *Server) GetCheckpoint() string {
	return s.runner.GetCheckpoint()
}

// SetCheckpoint — смена чекпоинта SDXL на сессию (делегирует Runner).
func (s *Server) SetCheckpoint(name string) {
	s.runner.SetCheckpoint(name)
}

// handleCheckpoint — GET /checkpoint → {current, list} (текущий + доступные);
// POST /checkpoint?name= → установить чекпоинт на сессию (валидация:
// имя ∈ KnownCheckpoints; диск не переписывается, применяется к следующим
// генерациям). Ответ POST — {ok, current}.
func (s *Server) handleCheckpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		name := r.URL.Query().Get("name")
		if !config.IsKnownCheckpoint(name) {
			writeJSON(w, map[string]string{"error": "неизвестный чекпоинт: " + name})
			return
		}
		s.SetCheckpoint(name)
		writeJSON(w, map[string]interface{}{"ok": true, "current": name})
		return
	}
	writeJSON(w, map[string]interface{}{
		"current": s.GetCheckpoint(),
		"list":    config.KnownCheckpoints,
	})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	// стоп-флаг в пуле активной генерации (одна активная генерация, спека §6.2)
	races := generator.ReadStatus(filepath.Join(s.cfg.PoolRoot, "races_pool"))
	humans := generator.ReadStatus(filepath.Join(s.cfg.PoolRoot, "humans_pool"))
	ships := generator.ReadStatus(filepath.Join(s.cfg.PoolRoot, "ships_pool"))
	if !races.Running && !humans.Running && !ships.Running {
		writeJSON(w, map[string]string{"msg": "Нет активной генерации"})
		return
	}
	if races.Running {
		generator.CreateStopFlag(filepath.Join(s.cfg.PoolRoot, "races_pool"))
	}
	if humans.Running {
		generator.CreateStopFlag(filepath.Join(s.cfg.PoolRoot, "humans_pool"))
	}
	if ships.Running {
		generator.CreateStopFlag(filepath.Join(s.cfg.PoolRoot, "ships_pool"))
	}
	writeJSON(w, map[string]string{"msg": "Останавливаю генерацию..."})
}

// handleClearPool — «Очистить результаты»: удаляет r*.png и meta.json пула рас
// (ref_* эталоны и ref_cands/ не трогаем). Отклоняет, если генерация идёт.
func (s *Server) handleClearPool(w http.ResponseWriter, r *http.Request) {
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	if st := generator.ReadStatus(pool); st.Running {
		writeJSON(w, map[string]string{"msg": "Генерация идёт — останови её, потом очищай"})
		return
	}
	n := 0
	entries, err := os.ReadDir(pool)
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, "r") && !strings.HasPrefix(name, "ref_") && strings.HasSuffix(name, ".png") {
				os.Remove(filepath.Join(pool, name))
				n++
			}
		}
	}
	os.Remove(filepath.Join(pool, "meta.json"))
	writeJSON(w, map[string]string{"msg": fmt.Sprintf("Очищено вариантов: %d", n)})
}

// --- общие хелперы ---

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func servePNG(w http.ResponseWriter, fp string) {
	data, err := os.ReadFile(fp)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(data)
}

func writePNG(w http.ResponseWriter, img image.Image) {
	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, img)
}

func openImage(fp string) (image.Image, error) {
	f, err := os.Open(fp)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}

func savePNG(fp string, img image.Image) error {
	f, err := os.Create(fp)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func writeJSONFile(fp string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(fp, data, 0644)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func fileExists(fp string) bool {
	_, err := os.Stat(fp)
	return err == nil
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func atofDefault(s string, def float64) float64 {
	if s == "" {
		return def
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return f
}
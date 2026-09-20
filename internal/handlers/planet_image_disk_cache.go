// internal/handlers/planet_image_disk_cache.go
//
// Диск-кэш большой картинки (спека 2026-09-20 §5.2): PNG 512 в
// {PLANET_IMAGE_CACHE_DIR}/{sha256(planet_id|mode)}.png. Лимит 1000 файлов,
// вытеснение LRU по mtime; битый/частичный файл → перегенерация (§7).
// Каталог создаётся при старте (InitPlanetImageCacheDir); на проде задаётся
// env PLANET_IMAGE_CACHE_DIR в место с правом записи (DEPLOY.md §1).
package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"zorion/internal/generator/planet"
)

// diskCacheMaxFiles — лимит файлов диск-кэша (≈ 100–300 МБ при 100–300 КБ/PNG).
const diskCacheMaxFiles = 1000

// diskCacheMu — сериализация записи/вытеснения (несколько запросов могут
// писать одновременно; AGENTS.md §0).
var diskCacheMu sync.Mutex

// InitPlanetImageCacheDir — MkdirAll каталога диск-кэша при старте + стартовый
// клин (спека §5.2, S1/M1): удаляет файлы, чьё имя НЕ начинается с текущего
// префикса версии {ImageGenVersion}_ — включая легаси-файлы без префикса
// (старый путь писал {sha256}.png), иначе легаси остаются в LRU-лимите 1000
// и вытесняют новые.
func InitPlanetImageCacheDir(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	prefix := planet.ImageGenVersion + "_"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), prefix) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

// diskCachePath — путь файла кэша: {dir}/{imageGenVersion}_{sha256(planet_id|mode)}.png
// (префикс версии — явная сегрегация поколений, спека §5.2).
func diskCachePath(dir, planetID string, mode planet.ImageMode) string {
	sum := sha256.Sum256([]byte(planetID + "|" + string(mode)))
	return filepath.Join(dir, planet.ImageGenVersion+"_"+hex.EncodeToString(sum[:])+".png")
}

// diskCacheGet — чтение из диск-кэша. Битый/частичный файл → (nil, false)
// (перегенерация, спека §7). Хит обновляет mtime (LRU-порядок).
func diskCacheGet(dir, planetID string, mode planet.ImageMode) ([]byte, bool) {
	if dir == "" {
		return nil, false
	}
	path := diskCachePath(dir, planetID, mode)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		log.Printf("⚠️ Диск-кэш картинок: битый файл %s — перегенерация", path)
		return nil, false
	}
	_ = os.Chtimes(path, time.Now(), time.Now()) // touch для LRU
	return data, true
}

// diskCachePut — запись в диск-кэш (tmp + rename) + лимит 1000 файлов LRU
// по mtime (при превышении — удалить самые старые). Ошибка записи — лог
// (картинка уже отдана из памяти, сервер не падает).
func diskCachePut(dir, planetID string, mode planet.ImageMode, data []byte) {
	if dir == "" {
		return
	}
	diskCacheMu.Lock()
	defer diskCacheMu.Unlock()

	path := diskCachePath(dir, planetID, mode)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("⚠️ Диск-кэш картинок: запись %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		log.Printf("⚠️ Диск-кэш картинок: rename %s: %v", path, err)
		return
	}
	enforceDiskCacheLimit(dir)
}

// enforceDiskCacheLimit — LRU-лимит: при > 1000 файлов удалить самые старые
// по mtime. Вызывается под diskCacheMu.
func enforceDiskCacheLimit(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) <= diskCacheMaxFiles {
		return
	}
	type fileAge struct {
		path string
		mod  time.Time
	}
	var files []fileAge
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileAge{path: filepath.Join(dir, e.Name()), mod: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	excess := len(files) - diskCacheMaxFiles
	for i := 0; i < excess && i < len(files); i++ {
		_ = os.Remove(files[i].path)
	}
}
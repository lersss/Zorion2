// internal/handlers/planet_image_disk_cache_test.go
//
// Тесты диск-кэша большой картинки (спека 2026-09-20 §5.2/§8.2 №7):
// повторный запрос big не генерирует заново, лимит 1000 файлов (LRU),
// битый файл → перегенерация.
package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/generator/planet"
)

func TestDiskCacheBig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, InitPlanetImageCacheDir(dir))

	h, mock := planetImageHarness(t)
	h.cacheDir = dir

	req := httptest.NewRequest(http.MethodGet, "/api/planet-image?planet_id=p1&size=big", nil)
	req = withRole(req, "admin")
	req = withUserID(req, "u1")

	// Первый запрос: генерация + запись файла.
	expectPlanetImageFetch(mock, "p1", "w1",
		`{"temperature":288,"water_percent":60,"life":true,"biomes":[{"form":"горы","share":40},{"form":"океаны","share":35},{"form":"леса","share":25}]}`)
	rec1 := execJSON(h.ServeHTTP, req)
	require.Equal(t, http.StatusOK, rec1.Code)

	// Файл диск-кэша существует (админ → режим full, спека §3.3).
	path := diskCachePath(dir, "p1", planet.ImageModeFull)
	_, err := os.Stat(path)
	require.NoError(t, err, "файл диск-кэша создан")

	// Второй запрос — диск-кэш-хит: планета перечитывается (режим в ключе
	// кэша зависит от планеты/роли), но генерация не выполняется — файл отдан.
	expectPlanetImageFetch(mock, "p1", "w1",
		`{"temperature":288,"water_percent":60,"life":true,"biomes":[{"form":"горы","share":40},{"form":"океаны","share":35},{"form":"леса","share":25}]}`)
	rec2 := execJSON(h.ServeHTTP, req)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.True(t, bytes.Equal(rec1.Body.Bytes(), rec2.Body.Bytes()), "диск-кэш отдаёт тот же PNG")
	require.NoError(t, mock.ExpectationsWereMet())

	// Битый файл → перегенерация (запрос снова читает БД и пишет файл заново).
	require.NoError(t, os.WriteFile(path, []byte("битый-файл"), 0o644))
	expectPlanetImageFetch(mock, "p1", "w1",
		`{"temperature":288,"water_percent":60,"life":true,"biomes":[{"form":"горы","share":40},{"form":"океаны","share":35},{"form":"леса","share":25}]}`)
	rec3 := execJSON(h.ServeHTTP, req)
	require.Equal(t, http.StatusOK, rec3.Code)
	require.True(t, bytes.Equal(rec1.Body.Bytes(), rec3.Body.Bytes()), "битый файл → перегенерация того же PNG")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDiskCacheLimitLRU(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, InitPlanetImageCacheDir(dir))

	// Заполняем кэш сверх лимита: 1005 записей → остаётся ≤ 1000.
	for i := 0; i < diskCacheMaxFiles+5; i++ {
		diskCachePut(dir, "planet-"+string(rune('a'+i%26))+"-"+string(rune('0'+i/26)),
			planet.ImageModeHonest, []byte("png-data"))
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.LessOrEqual(t, len(entries), diskCacheMaxFiles, "лимит 1000 файлов (LRU по mtime)")
}

// ==================== №10: СТАРТОВЫЙ КЛИН ПО ВЕРСИИ (S1, M1) ====================

func TestDiskCacheVersionCleanup(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, InitPlanetImageCacheDir(dir))

	// Старые файлы (другой префикс версии) + легаси без префикса (M1).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "v2_abc.png"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "def.png"), []byte("x"), 0o644))
	// Текущий префикс версии.
	require.NoError(t, os.WriteFile(filepath.Join(dir, planet.ImageGenVersion+"_ghi.png"), []byte("x"), 0o644))

	// Повторный Init — стартовый клин (спека §5.2).
	require.NoError(t, InitPlanetImageCacheDir(dir))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.Contains(t, names, planet.ImageGenVersion+"_ghi.png", "текущий префикс остаётся")
	require.NotContains(t, names, "v2_abc.png", "старый префикс удаляется")
	require.NotContains(t, names, "def.png", "легаси без префикса удаляется (M1)")
}
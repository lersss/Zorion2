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

	// Файл диск-кэша существует.
	path := diskCachePath(dir, "p1", planet.ImageModeHonest)
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
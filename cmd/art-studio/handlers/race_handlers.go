package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"zorion/cmd/art-studio/generator"
	"zorion/cmd/art-studio/postproc"
)

// --- Расовая студия (перенос race_studio.py, спека 67a.1 §6.1) ---

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	fam := r.URL.Query().Get("fam")
	race := r.URL.Query().Get("race")
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	byFile := map[string]generator.MetaItem{}
	for _, m := range generator.ReadPoolMeta(pool) {
		byFile[m.File] = m
	}
	entries, _ := os.ReadDir(pool)
	var files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".png") && name != "cockpit_1024.png" {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	out := []map[string]string{}
	for _, name := range files {
		m := byFile[name]
		if fam != "" && m.Family != fam {
			continue
		}
		if race != "" && m.RaceID != race {
			continue
		}
		out = append(out, map[string]string{"file": name, "fam": m.Family, "race": m.Race})
	}
	writeJSON(w, out)
}

func (s *Server) handleRaces(w http.ResponseWriter, r *http.Request) {
	fam := r.URL.Query().Get("fam")
	if fam == "" {
		fam = "F2"
	}
	f, ok := s.families[fam]
	if !ok {
		writeJSON(w, map[string]interface{}{"races": []interface{}{}})
		return
	}
	var races []map[string]string
	for _, rc := range f.Races {
		races = append(races, map[string]string{"id": rc.ID, "name": rc.Name})
	}
	writeJSON(w, map[string]interface{}{"races": races})
}

func (s *Server) handleRefs(w http.ResponseWriter, r *http.Request) {
	fam := r.URL.Query().Get("fam")
	if fam == "" {
		fam = "F2"
	}
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	names := s.raceNames(fam)
	entries, _ := os.ReadDir(pool)
	prefix := "ref_" + fam + "_r"
	refs := []map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".png") {
			race := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".png")
			refs = append(refs, map[string]string{"race": race, "race_name": names[race]})
		}
	}
	writeJSON(w, map[string]interface{}{"refs": refs})
}

func (s *Server) handleRef(w http.ResponseWriter, r *http.Request) {
	fam := r.URL.Query().Get("fam")
	if fam == "" {
		fam = "F2"
	}
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	var raceStatus []map[string]interface{}
	raceOptions := ""
	if f, ok := s.families[fam]; ok {
		for _, rc := range f.Races {
			has := fileExists(filepath.Join(pool, RefFileName(fam, rc.ID)))
			raceStatus = append(raceStatus, map[string]interface{}{"id": rc.ID, "name": rc.Name, "has": has})
			mark := ""
			if has {
				mark = "✓ "
			}
			raceOptions += fmt.Sprintf(`<option value="%s">%s%s</option>`, rc.ID, mark, rc.Name)
		}
	}
	writeJSON(w, map[string]interface{}{"raceStatus": raceStatus, "raceOptions": raceOptions, "cands": s.readCands(pool)})
}

func (s *Server) handleRefImg(w http.ResponseWriter, r *http.Request) {
	fam := r.URL.Query().Get("fam")
	race := r.URL.Query().Get("race")
	fp := filepath.Join(s.cfg.PoolRoot, "races_pool", RefFileName(fam, race))
	servePNG(w, fp)
}

func (s *Server) handleRefCand(w http.ResponseWriter, r *http.Request) {
	fname := filepath.Base(r.URL.Path[len("/refcand/"):])
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	fp := filepath.Join(pool, "ref_cands", fname)
	img, err := openImage(fp)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	writePNG(w, postproc.CompositeOnCockpit(img, filepath.Join(pool, "cockpit_1024.png")))
}

func (s *Server) handleRefSet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	fam := q.Get("fam")
	race := q.Get("race")
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	src := filepath.Join(pool, "ref_cands", filepath.Base(file))
	msg := "нет файла кандидата"
	if _, err := os.Stat(src); err == nil {
		rp := filepath.Join(pool, RefFileName(fam, race))
		rm := strings.TrimSuffix(rp, ".png") + ".json"
		if err := copyFile(src, rp); err != nil {
			msg = "ошибка копирования: " + err.Error()
		} else {
			raceName := race
			if f, ok := s.families[fam]; ok {
				for _, rc := range f.Races {
					if rc.ID == race {
						raceName = rc.Name
						break
					}
				}
			}
			writeJSONFile(rm, map[string]string{"fam": fam, "race": raceName})
			msg = "Эталон расы " + raceName + " установлен"
		}
	}
	writeJSON(w, map[string]string{"msg": msg})
}

// handleRefSetPool — «Взять за новый эталон»: копирует сгенерированный вариант
// rNN.png из пула в ref_<FAM>_r<race>.png (вариант остаётся в пуле).
func (s *Server) handleRefSetPool(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	fam := q.Get("fam")
	race := q.Get("race")
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	src := filepath.Join(pool, filepath.Base(file))
	msg := "нет файла в пуле"
	if _, err := os.Stat(src); err == nil {
		rp := filepath.Join(pool, RefFileName(fam, race))
		rm := strings.TrimSuffix(rp, ".png") + ".json"
		if err := copyFile(src, rp); err != nil {
			msg = "ошибка копирования: " + err.Error()
		} else {
			raceName := race
			if f, ok := s.families[fam]; ok {
				for _, rc := range f.Races {
					if rc.ID == race {
						raceName = rc.Name
						break
					}
				}
			}
			writeJSONFile(rm, map[string]string{"fam": fam, "race": raceName})
			// вариант стал эталоном (калибровочным) — убираем его из пула
			// аватаров: в пул попадают только явно принятые кнопкой «Принять»
			// (решение создателя 2026-09-17)
			os.Remove(src)
			generator.RemovePoolMeta(pool, filepath.Base(file))
			msg = "Эталон расы " + raceName + " установлен (вариант убран из пула аватаров)"
		}
	}
	writeJSON(w, map[string]string{"msg": msg})
}

// handleRefSetAccepted — «Принять за эталон» из галереи: копирует одобренный
// вариант final_accepted/<FAM>/r<race_id>/<file> в ref_<FAM>_r<race>.png
// (одобренный остаётся в галерее).
func (s *Server) handleRefSetAccepted(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	fam := q.Get("fam")
	race := q.Get("race")
	accRoot := filepath.Join(s.cfg.PoolRoot, "final_accepted")
	src := filepath.Join(accRoot, fam, "r"+race, filepath.Base(file))
	msg := "нет файла в одобренных"
	if _, err := os.Stat(src); err == nil {
		pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
		rp := filepath.Join(pool, RefFileName(fam, race))
		rm := strings.TrimSuffix(rp, ".png") + ".json"
		if err := copyFile(src, rp); err != nil {
			msg = "ошибка копирования: " + err.Error()
		} else {
			raceName := race
			if f, ok := s.families[fam]; ok {
				for _, rc := range f.Races {
					if rc.ID == race {
						raceName = rc.Name
						break
					}
				}
			}
			writeJSONFile(rm, map[string]string{"fam": fam, "race": raceName})
			msg = "Эталон расы " + raceName + " установлен"
		}
	}
	writeJSON(w, map[string]string{"msg": msg})
}

func (s *Server) handleRefClear(w http.ResponseWriter, r *http.Request) {
	fam := r.URL.Query().Get("fam")
	race := r.URL.Query().Get("race")
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	rp := filepath.Join(pool, RefFileName(fam, race))
	rm := strings.TrimSuffix(rp, ".png") + ".json"
	removed := false
	for _, fp := range []string{rp, rm} {
		if err := os.Remove(fp); err == nil {
			removed = true
		}
	}
	msg := "Эталон расы " + race + " сброшен"
	if !removed {
		msg = "эталона и не было"
	}
	writeJSON(w, map[string]string{"msg": msg})
}

func (s *Server) handleGenVar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n := clampCount(atoiDefault(q.Get("n"), 8), s.cfg.MaxCount)
	denoise := atofDefault(q.Get("denoise"), s.cfg.DenoiseRef)
	cnStrength := atofDefault(q.Get("cn_strength"), s.cfg.CNStrength)
	cnEnd := atofDefault(q.Get("cn_end"), s.cfg.CNEnd)
	size := clampSize(atoiDefault(q.Get("size"), 512))
	palette := clampPalette(atoiDefault(q.Get("palette"), 0))
	msg, _ := s.runner.GenVar(q.Get("fam"), q.Get("race"), n, denoise, cnStrength, cnEnd, size, palette)
	writeJSON(w, map[string]string{"msg": msg})
}

func (s *Server) handleGenRef(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n := clampCount(atoiDefault(q.Get("n"), 12), s.cfg.MaxCount)
	anthro := q.Get("anthro") == "anthro"
	size := clampSize(atoiDefault(q.Get("size"), 512))
	msg, _ := s.runner.GenRef(q.Get("fam"), n, anthro, size)
	writeJSON(w, map[string]string{"msg": msg})
}

func (s *Server) handleAct(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	what := q.Get("what")
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	msg := "?"
	src := filepath.Join(pool, filepath.Base(file))
	switch what {
	case "accept":
		if _, err := os.Stat(src); err == nil {
			_, m := acceptRaceFile(pool, filepath.Join(s.cfg.PoolRoot, "final_accepted"), file)
			msg = m
		}
	case "reject":
		if _, err := os.Stat(src); err == nil {
			rej := filepath.Join(s.cfg.PoolRoot, "races_rejected")
			os.MkdirAll(rej, 0755)
			os.Rename(src, filepath.Join(rej, filepath.Base(file)))
			msg = "Удалено: " + file
		}
	}
	writeJSON(w, map[string]string{"msg": msg})
}

func (s *Server) handleCrop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	pct := atoiDefault(q.Get("pct"), 100)
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	writeJSON(w, map[string]string{"msg": cropFile(pool, file, pct)})
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	pct := atoiDefault(q.Get("pct"), 100)
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	img, err := openImage(filepath.Join(pool, filepath.Base(file)))
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	cropped := postproc.CropBottom(toNRGBA(img), pct)
	writePNG(w, postproc.CompositeOnCockpit(cropped, filepath.Join(pool, "cockpit_1024.png")))
}

func (s *Server) handleImg(w http.ResponseWriter, r *http.Request) {
	fname := filepath.Base(r.URL.Path[len("/img/"):])
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	fp := filepath.Join(pool, fname)
	if fname == "cockpit_1024.png" {
		servePNG(w, fp)
		return
	}
	img, err := openImage(fp)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	writePNG(w, postproc.CompositeOnCockpit(img, filepath.Join(pool, "cockpit_1024.png")))
}

// handleGallery — галерея одобренных вариантов расы: эталон (есть/нет) +
// принятые из final_accepted/<FAM>/r<race_id>/NN.png (отсортированы по имени).
func (s *Server) handleGallery(w http.ResponseWriter, r *http.Request) {
	fam := r.URL.Query().Get("fam")
	race := r.URL.Query().Get("race")
	pool := filepath.Join(s.cfg.PoolRoot, "races_pool")
	ref := fileExists(filepath.Join(pool, RefFileName(fam, race)))
	dir := filepath.Join(s.cfg.PoolRoot, "final_accepted", fam, "r"+race)
	var items []map[string]string
	entries, err := os.ReadDir(dir)
	if err == nil {
		var files []string
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".png") {
				files = append(files, e.Name())
			}
		}
		sort.Strings(files)
		for _, name := range files {
			items = append(items, map[string]string{"file": name, "kind": "accepted"})
		}
	}
	writeJSON(w, map[string]interface{}{"ref": ref, "items": items})
}

// handleGalleryImg — картинка принятого варианта из
// final_accepted/<FAM>/r<race_id>/<file> (filepath.Base — без произвольных путей).
func (s *Server) handleGalleryImg(w http.ResponseWriter, r *http.Request) {
	fam := r.URL.Query().Get("fam")
	race := r.URL.Query().Get("race")
	file := filepath.Base(r.URL.Query().Get("file"))
	fp := filepath.Join(s.cfg.PoolRoot, "final_accepted", fam, "r"+race, file)
	servePNG(w, fp)
}

// --- внутренние хелперы расовой студии ---

func (s *Server) raceNames(famID string) map[string]string {
	out := map[string]string{}
	if f, ok := s.families[famID]; ok {
		for _, rc := range f.Races {
			out[rc.ID] = rc.Name
		}
	}
	return out
}

func (s *Server) readCands(pool string) []map[string]string {
	refdir := filepath.Join(pool, "ref_cands")
	out := []map[string]string{}
	entries, err := os.ReadDir(refdir)
	if err != nil {
		return out
	}
	candMeta := generator.ReadCandMeta(refdir)
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".png") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, name := range files {
		rn, rid, info := "", "", ""
		if cm, ok := candMeta[name]; ok {
			rn = cm.Race
			rid = cm.RaceID
			info = CandInfo(cm.Prompt)
		}
		out = append(out, map[string]string{"file": name, "race": rn, "raceId": rid, "info": info})
	}
	return out
}

// CandInfo — выдержка из промпта для карточки: материал + форма
// (перенос _cand_info из race_studio.py).
func CandInfo(prompt string) string {
	if prompt == "" {
		return ""
	}
	mat := ""
	if i := strings.Index(prompt, "made of "); i != -1 {
		rest := prompt[i+len("made of "):]
		if j := strings.Index(rest, ","); j != -1 {
			mat = strings.TrimSpace(rest[:j])
		}
	}
	form := ""
	if i := strings.Index(prompt, "viewer, "); i != -1 {
		seg := prompt[i+len("viewer, "):]
		if j := strings.Index(seg, "made of"); j != -1 {
			form = strings.TrimSpace(seg[:j])
		}
	}
	if form == "" {
		k := strings.Index(prompt, "a ")
		l := strings.Index(prompt, " of material")
		if k != -1 && l != -1 && l > k {
			form = strings.TrimSpace(prompt[k+2 : l])
		}
	}
	var parts []string
	if mat != "" {
		parts = append(parts, mat)
	}
	if form != "" {
		parts = append(parts, form)
	}
	return strings.Join(parts, " · ")
}

// acceptRaceFile принимает файл из пула: ресайз 200×200 (центр/низ) →
// final_accepted/<FAM>/r<race_id>/NN.png. Раса — из meta.json файла
// (не из выбранного в UI, спека 67a.1 §8). Без меты — fallback race_NN.png.
func acceptRaceFile(pool, acceptRoot, file string) (string, string) {
	src := filepath.Join(pool, file)
	fam, raceID := "", ""
	for _, m := range generator.ReadPoolMeta(pool) {
		if m.File == file {
			fam = m.Family
			raceID = m.RaceID
			break
		}
	}
	if fam != "" && raceID != "" {
		d := filepath.Join(acceptRoot, fam, "r"+raceID)
		os.MkdirAll(d, 0755)
		dst := filepath.Join(d, fmt.Sprintf("%02d.png", NextAcceptNumber(d)))
		if err := resizeSave(src, dst); err != nil {
			return "", "Ошибка: " + err.Error()
		}
		os.Remove(src)
		rel, _ := filepath.Rel(acceptRoot, dst)
		return dst, "Принято: " + rel
	}
	dst := filepath.Join(acceptRoot, fmt.Sprintf("race_%02d.png", NextAcceptNumber(acceptRoot)))
	if err := resizeSave(src, dst); err != nil {
		return "", "Ошибка: " + err.Error()
	}
	os.Remove(src)
	return dst, "Принято: " + filepath.Base(dst)
}

func resizeSave(src, dst string) error {
	img, err := openImage(src)
	if err != nil {
		return err
	}
	return savePNG(dst, postproc.PadCenterBottom(img, 200, 200))
}

func cropFile(pool, file string, pct int) string {
	fp := filepath.Join(pool, filepath.Base(file))
	img, err := openImage(fp)
	if err != nil {
		return "нет файла"
	}
	cropped := postproc.CropBottom(toNRGBA(img), pct)
	if err := savePNG(fp, cropped); err != nil {
		return "ошибка сохранения"
	}
	return fmt.Sprintf("Обрезано %s: осталось %d%%", file, pct)
}

func clampCount(n, max int) int {
	if n < 1 {
		return 1
	}
	if n > max {
		return max
	}
	return n
}

// clampSize — допустимый размер генерации: 512 (быстро) или 1024.
func clampSize(n int) int {
	if n != 512 && n != 1024 {
		return 512
	}
	return n
}

func clampPalette(n int) int {
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}
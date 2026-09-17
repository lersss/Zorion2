package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"zorion/cmd/art-studio/postproc"
)

// --- Людская студия (перенос human_studio.py, спека 67a.1 §6.2) ---

func (s *Server) handleHumansGen(w http.ResponseWriter, r *http.Request) {
	n := clampCount(atoiDefault(r.URL.Query().Get("n"), 8), s.cfg.MaxCount)
	msg, _ := s.runner.GenHumans(n)
	writeJSON(w, map[string]string{"msg": msg})
}

func (s *Server) handleHumansList(w http.ResponseWriter, r *http.Request) {
	pool := filepath.Join(s.cfg.PoolRoot, "humans_pool")
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
		sex := ""
		if strings.Contains(name, "_f") {
			sex = "ж"
		} else if strings.Contains(name, "_m") {
			sex = "м"
		}
		out = append(out, map[string]string{"file": name, "sex": sex})
	}
	writeJSON(w, out)
}

func (s *Server) handleHumansAct(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	what := q.Get("what")
	pool := filepath.Join(s.cfg.PoolRoot, "humans_pool")
	msg := "?"
	src := filepath.Join(pool, filepath.Base(file))
	switch what {
	case "accept":
		if _, err := os.Stat(src); err == nil {
			acceptRoot := filepath.Join(s.cfg.PoolRoot, "final_accepted")
			dst := filepath.Join(acceptRoot, fmt.Sprintf("race_f1_humans_%02d.png", NextAcceptHumans(acceptRoot)))
			if err := resizeSave(src, dst); err != nil {
				msg = "Ошибка: " + err.Error()
			} else {
				os.Remove(src)
				msg = "Принято: " + filepath.Base(dst)
			}
		}
	case "reject":
		if _, err := os.Stat(src); err == nil {
			rej := filepath.Join(s.cfg.PoolRoot, "humans_rejected")
			os.MkdirAll(rej, 0755)
			os.Rename(src, filepath.Join(rej, filepath.Base(file)))
			msg = "Удалено: " + file
		}
	}
	writeJSON(w, map[string]string{"msg": msg})
}

func (s *Server) handleHumansCrop(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	pct := atoiDefault(q.Get("pct"), 100)
	pool := filepath.Join(s.cfg.PoolRoot, "humans_pool")
	writeJSON(w, map[string]string{"msg": cropFile(pool, file, pct)})
}

func (s *Server) handleHumansPreview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	pct := atoiDefault(q.Get("pct"), 100)
	pool := filepath.Join(s.cfg.PoolRoot, "humans_pool")
	img, err := openImage(filepath.Join(pool, filepath.Base(file)))
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	cropped := postproc.CropBottom(toNRGBA(img), pct)
	writePNG(w, postproc.CompositeOnCockpit(cropped, filepath.Join(pool, "cockpit_1024.png")))
}

func (s *Server) handleHumansImg(w http.ResponseWriter, r *http.Request) {
	fname := filepath.Base(r.URL.Path[len("/humans/img/"):])
	pool := filepath.Join(s.cfg.PoolRoot, "humans_pool")
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
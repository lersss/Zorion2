package routegame

// gridv9_show_test.go — ВРЕМЕННЫЙ показ поля модели v9 (§14). Untracked,
// ничего не пишет в БД, не коммитится. Печатает доску глазами игрока,
// доску с раскрытыми секторами, маршрут Safe и числа стратегий.
// Запуск: go test -run TestV9Show -v ./internal/routegame/ -timeout 5m

import (
	"fmt"
	"strings"
	"testing"
)

func TestV9Show(t *testing.T) {
	pairs := loadPairsV9(t)
	cfg := DefaultV9Config()
	for _, band := range []string{"short", "long"} {
		var (
			r     V9PairRecord
			field V9Field
			found bool
		)
		for _, cand := range pairs {
			if cand.Band != band {
				continue
			}
			ff, ok := GenerateFieldV9(HashSeed(cand.FromID, cand.ToID), cand.Dist, V9PairPassport(cand), cfg)
			if !ok {
				continue
			}
			r, field, found = cand, ff, true
			break
		}
		if !found {
			t.Errorf("band %s: нет подходящей пары", band)
			continue
		}
		V9ShowField(t, r, field, cfg)
	}
}

func V9ShowField(t *testing.T, r V9PairRecord, f V9Field, cfg V9Config) {
	m := AnalyzeV9(f, cfg)
	t.Logf("══════════════════════════════════════════════════════════════════")
	t.Logf("ПАРА %s→%s band=%s dist=%.2f | from=%s/%s/%s T=%d | to=%s/%s/%s T=%d belt=%v",
		shortID(r.FromID), shortID(r.ToID), r.Band, r.Dist,
		r.FromSpectral, r.FromStar, r.FromSystem, r.FromTemp,
		r.ToSpectral, r.ToStar, r.ToSystem, r.ToTemp, r.Belt)
	t.Logf("режим=%s k=%d h=%d N=%d L_naive=%.2f L_safe=%.2f L_risk=%.2f",
		f.Mode, m.K, m.H, f.N, f.LNaive, f.LSafe, f.LRisk)
	t.Logf("")
	t.Logf("--- 1) ДОСКА (как видит игрок; скрытые секторы = ?) ---")
	V9PrintBoard(t, f, false, nil)
	t.Logf("легенда: S старт | F финиш | * маяк | ~ топь | = русло | G шлюз | > < ^ v течение по направлению | B разовый мост | # узкий проход | W стена (дорого) | . обычная | ? скрытый сектор")
	t.Logf("")
	t.Logf("--- 2) ДОСКА С РАСКРЫТЫМИ СЕКТОРАМИ (показ) ---")
	V9PrintBoard(t, f, true, nil)
	t.Logf("легенда сектора: J jackpot | L lure | T trap | D decoy | U unstable | E empty")
	for j, s := range f.Sectors {
		cells := make([]string, 0, len(s.Cells))
		for _, c := range s.Cells {
			cells = append(cells, fmt.Sprintf("(%d,%d)", f.iOf(c), f.jOf(c)))
		}
		t.Logf("   сектор #%d σ=%-6s content=%-8s клеток=%d %s",
			j, s.Sig, s.Content, len(s.Cells), strings.Join(cells, " "))
	}
	t.Logf("")
	t.Logf("--- 3) ЛЕНИВЫЙ БЕЗОПАСНЫЙ МАРШРУТ (Safe) ---")
	safe := f.safePathV9()
	V9PrintBoard(t, f, false, safe)
	t.Logf("легенда: o — клетки маршрута Safe (обход всех тёмных секторов); len=%d", len(safe))
	t.Logf("")
	t.Logf("--- 4) ЧИСЛА ПО ПОЛЮ ---")
	t.Logf("dist=%.2f band=%s режим=%s | L_naive=%.2f L_safe=%.2f L_risk=%.2f",
		r.Dist, r.Band, f.Mode, f.LNaive, f.LSafe, f.LRisk)
	t.Logf("bonus: L0=%+.3f C=%+.3f S=%+.3f Safe=%+.3f X_σ=%+.3f (σ=%s/%s) I_σ=%+.3f B=%+.3f F=%+.3f",
		m.BonusL0, m.BonusC, m.BonusS, m.BonusSafe, m.BonusX, m.XSig, m.XContent, m.BonusI, m.BonusB, m.BonusF)
	t.Logf("cost:  L0=%.2f C=%.2f S=%.2f Safe=%.2f X=%.2f I=%.2f B=%.2f F=%.2f | I−Safe=%+.3f I−X=%+.3f",
		m.CostL0, m.CostC, m.CostS, m.CostSafe, m.CostX, m.CostI, m.CostB, m.CostF,
		m.BonusI-m.BonusSafe, m.BonusI-m.BonusX)
	t.Logf("")
}

func V9PrintBoard(t *testing.T, f V9Field, revealed bool, path []int) {
	secIndex := map[int]int{}
	for j, s := range f.Sectors {
		for _, c := range s.Cells {
			secIndex[c] = j
		}
	}
	onPath := map[int]bool{}
	for _, c := range path {
		onPath[c] = true
	}
	for j := 0; j < f.N; j++ {
		var b strings.Builder
		for i := 0; i < f.N; i++ {
			c := f.cell(i, j)
			sym := V9CellSym(f, c, secIndex, revealed)
			if onPath[c] && c != f.Start && c != f.Finish {
				sym = "o"
			}
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(sym)
		}
		t.Logf("   %s", b.String())
	}
}

func V9CellSym(f V9Field, c int, secIndex map[int]int, revealed bool) string {
	switch {
	case c == f.Start:
		return "S"
	case c == f.Finish:
		return "F"
	case isBeaconV9(&f, c):
		return "*"
	}
	if j, ok := secIndex[c]; ok {
		if !revealed {
			return "?"
		}
		switch f.Sectors[j].Content {
		case ContentJackpotV9:
			return "J"
		case ContentLureV9:
			return "L"
		case ContentTrapV9:
			return "T"
		case ContentDecoyV9:
			return "D"
		case ContentUnstableV9:
			return "U"
		default:
			return "E"
		}
	}
	switch {
	case f.Gate[c]:
		return "G"
	case f.Bridge[c]:
		return "B"
	}
	if d, ok := f.CurrentDir[c]; ok {
		switch d {
		case 0:
			return ">"
		case 1:
			return "<"
		case 2:
			return "v"
		default:
			return "^"
		}
	}
	switch {
	case f.Bottleneck[c]:
		return "#"
	case f.Wall[c]:
		return "W"
	case f.Mud[c] > 0:
		return "~"
	case f.Lane[c]:
		return "="
	}
	return "."
}

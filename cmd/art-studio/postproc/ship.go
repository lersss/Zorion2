package postproc

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// ShipFrame — результат автопроверки кадра (рецепт 2026-09-21,
// tools/ship_sprite_cut.py --frame-check): touch — стороны кадра, которых
// корабль коснулся ближе margin px; elong — вытянутость силуэта (λ1/λ2 главных
// осей); ok — кадр годен.
type ShipFrame struct {
	Touch []string `json:"touch"`
	Elong float64  `json:"elong"`
	OK    bool     `json:"ok"`
}

// ShipFrameCheck — автопроверка сырого кадра до выреза (рецепт 2026-09-21):
// корабль не ближе margin px к краю и вытянутость силуэта ≥ minElong
// (3/4-вид ≥ 1.3, фронтальный симметричный ≈ 1.0). Вызов — Python-подпроцесс
// (единственный источник детекции — tools/ship_sprite_cut.py). Негодный кадр
// джоб отбрасывает и берёт следующий seed.
func ShipFrameCheck(pythonCmd, inPath string, margin int, minElong float64) (ShipFrame, error) {
	var fc ShipFrame
	tmp, err := os.CreateTemp("", "ship_frame_*.json")
	if err != nil {
		return fc, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	args := []string{"tools/ship_sprite_cut.py", inPath, "--frame-check", "--report", tmpPath,
		"--margin", strconv.Itoa(margin), "--min-elong", strconv.FormatFloat(minElong, 'g', -1, 64)}
	out, err := exec.Command(pythonCmd, args...).CombinedOutput()
	if err != nil {
		return fc, fmt.Errorf("ship_sprite_cut (frame-check): python недоступен: %v: %s", err, string(out))
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return fc, fmt.Errorf("ship_sprite_cut (frame-check): нет отчёта: %v", err)
	}
	if err := json.Unmarshal(data, &fc); err != nil {
		return fc, fmt.Errorf("ship_sprite_cut (frame-check): отчёт: %v", err)
	}
	return fc, nil
}

// ShipOrient — подсказка авто-ориентации из отчёта выреза (profile_orientation
// в tools/ship_sprite_cut.py): angle — поворот главной оси (конвенция PIL,
// положительный — против часовой), nose_side — куда смотрит нос по авто
// (left/right), mirror — предлагаемое зеркало, mirrored — применено ли оно
// нормализацией, ambiguous/reason — уверенность авто. Решение о финальной
// стороне всё равно за человеком (приёмка /ships/act|auto), это лишь подсказка.
type ShipOrient struct {
	Angle     float64 `json:"angle"`
	NoseSide  string  `json:"nose_side"`
	Mirror    bool    `json:"mirror"`
	Mirrored  bool    `json:"mirrored"`
	Ambiguous bool    `json:"ambiguous"`
	Reason    string  `json:"reason"`
}

// ShipSpriteCut — вырез фона и нормализация кандидата. Рецепт 2026-09-22:
// magenta-фон + вырез «edge» с увеличенным tol. Живой прогон (diamond/coastal)
// показал, что SDXL по промпту magenta-фона кладёт фон ГРАДИЕНТОМ (яркая
// магента → тёмная маренго; у «бирюзовых» рас примешивается cyan), а не ровной
// заливкой. Из-за этого ключ по «магента-ности» (`chroma`) градиент не берёт:
// оставляет ореол фона и (при понижении tol) выедает тёмный магента-корпус (у
// Алмазных медиана m корпуса совпадает с фоном). Метод «edge» (diff от модели
// фона + барьер по кромкам) на magenta-фоне отделяет фон надёжно: амплитуда
// фон-градиента ≤ ~70, а корпус отличается от плоскости на 160–380. tol=100
// подобран на diamond (корпус цел) и coastal; chroma оставлен в скрипте для
// сравнения и регресс-тестов.
// tools/ship_sprite_cut.py --method edge --tol 100 --edge 20
// --fill-holes --no-orient → прозрачный PNG canvas×canvas.
// canvas — 200 (полный) или 100 (эскиз, 98c). --no-orient отключает пиксельный
// доворот (нос/зеркало — метаданные пары (A, F), применяются при показе, спека
// 2026-09-21-угол-корабля-в-метаданных §5); кроп/масштаб/центрирование
// остаются. Возвращает подсказку авто-ориентации из отчёта скрипта
// (best-effort: нет отчёта — нулевая подсказка без ошибки).
func ShipSpriteCut(pythonCmd, inPath, outPath string, canvas int) (ShipOrient, error) {
	var orient ShipOrient
	tmp, err := os.CreateTemp("", "ship_cut_*.json")
	if err != nil {
		return orient, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	args := []string{"tools/ship_sprite_cut.py", inPath, outPath, "--method", "edge",
		"--tol", "100", "--edge", "20", "--fill-holes", "--no-orient", "--report", tmpPath}
	if canvas > 0 && canvas != 200 {
		args = append(args, "--canvas", strconv.Itoa(canvas))
	}
	out, err := exec.Command(pythonCmd, args...).CombinedOutput()
	if err != nil {
		return orient, fmt.Errorf("ship_sprite_cut: python недоступен: %v: %s", err, string(out))
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return orient, nil // подсказка — best-effort
	}
	var rep struct {
		Orient ShipOrient `json:"orient"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		return orient, nil
	}
	return rep.Orient, nil
}

// ProcessShip вызывает tools/process_ship.py (спека 2026-09-20-ships-races-
// generator §3.5): вырез фона по цвету (BG_TOL 40), крупнейший связный
// компонент, сглаживание, кадрирование + паддинг 6 px, вписывание в
// canvas×canvas (200 — полный, 100 — эскиз, 98c), нормализация ориентации
// (нос вправо). Выход — прозрачный PNG в пул.
func ProcessShip(pythonCmd, inPath, outPath string, canvas int) error {
	args := []string{"tools/process_ship.py", inPath, outPath}
	if canvas > 0 && canvas != 200 {
		args = append(args, "--canvas", strconv.Itoa(canvas))
	}
	cmd := exec.Command(pythonCmd, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("process_ship: %v: %s", err, string(out))
	}
	return nil
}
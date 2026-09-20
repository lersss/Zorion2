package postproc

import (
	"fmt"
	"os/exec"
	"strconv"
)

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
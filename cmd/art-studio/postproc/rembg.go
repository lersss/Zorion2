package postproc

import (
	"fmt"
	"os/exec"
)

// RemoveBG вызывает Python-подпроцесс rembg (rembg_cli.py, isnet-general-use)
// одной командой на файл (решение 67a п.1, спека 67a.1 §7.1).
// Ошибка подпроцесса возвращается — вызывающий помечает картинку FAIL.
func RemoveBG(pythonCmd, cliPath, inPath, outPath string) error {
	cmd := exec.Command(pythonCmd, cliPath, inPath, outPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rembg: %v: %s", err, string(out))
	}
	return nil
}
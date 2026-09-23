//go:build !windows

// internal/goodsstudio/aiserve/proc_other.go
// Заглушка для не-Windows (прод Amvera): процессные пробы недоступны,
// managed=false, спавн/гашение невозможны (спека §2/§6).
package aiserve

import "errors"

var errNotWindows = errors.New("управление помощником доступно только на Windows")

func spawnHelper(port int, logPath string) (int, error) { return 0, errNotWindows }
func killTree(pid int) error                            { return errNotWindows }
func scanProcesses() []procInfo                         { return nil }
func portOwnerInfo(port int) (procInfo, bool)           { return procInfo{}, false }

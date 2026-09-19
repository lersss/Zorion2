package main

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPortProtection — защита от дублей (спека 99a.1 §12.1): второй bind
// того же порта падает (один процесс на порт).
func TestPortProtection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	addr := ln.Addr().String()
	second, err := net.Listen("tcp", addr)
	require.Error(t, err, "второй bind того же порта должен падать")
	require.Nil(t, second)
}
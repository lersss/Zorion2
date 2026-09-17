package main

import "testing"

// TestBindPortTwice — защита от дублей: второй bind на тот же порт падает
// (спека 67a.1 §9.1, DoD §10).
func TestBindPortTwice(t *testing.T) {
	ln, err := bindPort("127.0.0.1:0")
	if err != nil {
		t.Fatalf("первый bind: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	ln2, err := bindPort(addr)
	if err == nil {
		ln2.Close()
		t.Fatal("второй bind на тот же порт не упал")
	}
}

// TestBindPortFree — свободный порт занимается без ошибки.
func TestBindPortFree(t *testing.T) {
	ln, err := bindPort("127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind свободного порта: %v", err)
	}
	ln.Close()
}
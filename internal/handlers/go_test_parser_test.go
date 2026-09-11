package handlers

import (
	"strings"
	"testing"
)

func TestParseGoTestJSONAllPass(t *testing.T) {
	input := `{"Action":"run","Package":"zorion/internal/generator/planet","Test":"TestSatellitesHydroRare"}
{"Action":"output","Package":"zorion/internal/generator/planet","Test":"TestSatellitesHydroRare","Output":"--- PASS: TestSatellitesHydroRare (0.21s)\n"}
{"Action":"pass","Package":"zorion/internal/generator/planet","Test":"TestSatellitesHydroRare","Elapsed":0.21}
{"Action":"pass","Package":"zorion/internal/generator/planet","Elapsed":2.77}
{"Action":"skip","Package":"zorion/internal/handlers","Elapsed":0.02}`

	sum, err := parseGoTestJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sum.Status != "success" {
		t.Errorf("status = %q, want success", sum.Status)
	}
	if sum.PackagesTotal != 2 {
		t.Errorf("packages_total = %d, want 2", sum.PackagesTotal)
	}
	if sum.PackagesOK != 1 || sum.PackagesSkip != 1 {
		t.Errorf("packages ok/skip = %d/%d, want 1/1", sum.PackagesOK, sum.PackagesSkip)
	}
	if sum.TestsTotal != 1 || sum.TestsFailed != 0 {
		t.Errorf("tests total/failed = %d/%d, want 1/0", sum.TestsTotal, sum.TestsFailed)
	}
	if len(sum.Packages) != 2 {
		t.Fatalf("packages len = %d, want 2", len(sum.Packages))
	}
	if sum.Packages[0].Name != "planet" {
		t.Errorf("friendly name = %q, want planet", sum.Packages[0].Name)
	}
}

func TestParseGoTestJSONFailingTest(t *testing.T) {
	input := `{"Action":"run","Package":"zorion/internal/generator/planet","Test":"TestPlanetsSimpleRare"}
{"Action":"output","Package":"zorion/internal/generator/planet","Test":"TestPlanetsSimpleRare","Output":"--- FAIL: TestPlanetsSimpleRare (0.10s)\n"}
{"Action":"output","Package":"zorion/internal/generator/planet","Test":"TestPlanetsSimpleRare","Output":"    planet_data_simple_test.go:52: примитивных планет 9.00% — больше допустимых 3-4%\n"}
{"Action":"output","Package":"zorion/internal/generator/planet","Test":"TestPlanetsSimpleRare","Output":"FAIL\n"}
{"Action":"fail","Package":"zorion/internal/generator/planet","Test":"TestPlanetsSimpleRare","Elapsed":0.1}
{"Action":"fail","Package":"zorion/internal/generator/planet","Elapsed":1.2}`

	sum, err := parseGoTestJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sum.Status != "fail" {
		t.Errorf("status = %q, want fail", sum.Status)
	}
	if sum.TestsFailed != 1 {
		t.Errorf("tests_failed = %d, want 1", sum.TestsFailed)
	}
	if len(sum.Packages) != 1 {
		t.Fatalf("packages len = %d, want 1", len(sum.Packages))
	}
	p := sum.Packages[0]
	if p.Status != "fail" || p.Failed != 1 {
		t.Errorf("package status/failed = %q/%d, want fail/1", p.Status, p.Failed)
	}
	if len(p.Failures) != 1 {
		t.Fatalf("failures len = %d, want 1", len(p.Failures))
	}
	f := p.Failures[0]
	if f.Test != "TestPlanetsSimpleRare" {
		t.Errorf("failure test = %q", f.Test)
	}
	for _, bad := range []string{"--- FAIL", "FAIL\n"} {
		if strings.Contains(f.Message, bad) {
			t.Errorf("сообщение содержит служебную строку %q: %q", bad, f.Message)
		}
	}
	if !strings.Contains(f.Message, "больше допустимых 3-4%") {
		t.Errorf("сообщение не содержит суть ошибки: %q", f.Message)
	}
	if len(f.Message) > maxFailureLen {
		t.Errorf("сообщение слишком длинное: %d > %d", len(f.Message), maxFailureLen)
	}
}

func TestParseGoTestJSONBuildError(t *testing.T) {
	input := `{"Action":"output","Package":"zorion/internal/config","Output":"# zorion/internal/config\n"}
{"Action":"output","Package":"zorion/internal/config","Output":"./config.go:12:2: undefined: NotExisting\n"}
{"Action":"fail","Package":"zorion/internal/config","Elapsed":0.3}`

	sum, err := parseGoTestJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sum.Status != "fail" {
		t.Errorf("status = %q, want fail", sum.Status)
	}
	p := sum.Packages[0]
	if len(p.Failures) != 1 {
		t.Fatalf("failures len = %d, want 1", len(p.Failures))
	}
	if p.Failures[0].Test != "«пакет»" {
		t.Errorf("failure test = %q, want «пакет»", p.Failures[0].Test)
	}
	if !strings.Contains(p.Failures[0].Message, "undefined: NotExisting") {
		t.Errorf("сообщение об ошибке сборки потеряно: %q", p.Failures[0].Message)
	}
}

func TestParseGoTestJSONAnsiStripped(t *testing.T) {
	input := `{"Action":"output","Package":"p","Test":"TestX","Output":"    \u001b[31mboom\u001b[0m\n"}
{"Action":"fail","Package":"p","Test":"TestX","Elapsed":0.1}
{"Action":"fail","Package":"p","Elapsed":0.1}`

	sum, err := parseGoTestJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if strings.ContainsRune(sum.Packages[0].Failures[0].Message, '\x1b') {
		t.Errorf("ANSI-последовательности не вычищены: %q", sum.Packages[0].Failures[0].Message)
	}
}

func TestParseGoTestJSONEmpty(t *testing.T) {
	sum, err := parseGoTestJSON(strings.NewReader(""))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sum.Status != "success" || sum.PackagesTotal != 0 {
		t.Errorf("пустой ввод: status=%q packages=%d, want success/0", sum.Status, sum.PackagesTotal)
	}
}

func TestParseGoTestJSONGarbageLine(t *testing.T) {
	_, err := parseGoTestJSON(strings.NewReader("not a json line\n"))
	if err == nil {
		t.Fatal("ожидали ошибку на мусорной строке")
	}
}
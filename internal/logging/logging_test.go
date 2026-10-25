package logging_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/logging"
)

func logFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestLoggerWritesToTheLogFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	log, closeLog, err := logging.New(logging.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	log.Info("launcher opened", "ms", 12)
	log.Debug("hidden at the default level")
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "althea.log"))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(data); !strings.Contains(s, "launcher opened") || !strings.Contains(s, "ms=12") {
		t.Errorf("log = %q, want the info record", s)
	}
	if strings.Contains(string(data), "hidden") {
		t.Error("debug record written at the default level")
	}
}

func TestLoggerRotatesBySizeAndKeepsAFewFiles(t *testing.T) {
	dir := t.TempDir()
	log, closeLog, err := logging.New(logging.Options{Dir: dir, MaxSize: 200, Keep: 2})
	if err != nil {
		t.Fatal(err)
	}
	for range 40 {
		log.Info("a line long enough to fill the file quickly")
	}
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}

	want := []string{"althea.1.log", "althea.2.log", "althea.log"}
	if got := logFiles(t, dir); !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
	for _, name := range want {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > 200 {
			t.Errorf("%s is %d bytes, over the 200-byte limit", name, info.Size())
		}
	}
}

func TestLoggerAppendsAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	for _, msg := range []string{"first run", "second run"} {
		log, closeLog, err := logging.New(logging.Options{Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		log.Info(msg)
		if err := closeLog(); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, "althea.log"))
	if !strings.Contains(string(data), "first run") || !strings.Contains(string(data), "second run") {
		t.Errorf("log = %q, want both runs", data)
	}
}

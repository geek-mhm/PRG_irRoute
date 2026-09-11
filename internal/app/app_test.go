package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"irroute/internal/store"
)

type offlineRunner struct{}

func (offlineRunner) Run(string, ...string) (string, error) {
	return "", errors.New("command unavailable")
}
func (offlineRunner) RunInput(string, string, ...string) (string, error) {
	return "", errors.New("command unavailable")
}
func (offlineRunner) LookPath(string) error { return errors.New("command unavailable") }
func (offlineRunner) IsLinux() bool         { return false }
func (offlineRunner) IsRoot() bool          { return false }

func TestManualSetupAndForceRoute(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "iran.txt")
	if err := os.WriteFile(source, []byte("10.0.0.0/8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	answers := strings.Join([]string{
		"ens192",
		"198.51.100.10/24",
		"198.51.100.1",
		"ens224",
		"192.168.10.11/24",
		"192.168.10.1",
		source,
	}, "\n") + "\n"
	var output bytes.Buffer
	dataStore := store.New(store.NewPaths(root))
	application := New(strings.NewReader(answers), &output, &output, dataStore, offlineRunner{}, "test")
	if err := application.Run([]string{"setup"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := dataStore.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Local.Interface != "ens192" || cfg.International.Interface != "ens224" {
		t.Fatalf("unexpected interfaces: %+v", cfg)
	}
	if err := application.Run([]string{"force", "local", "add", "203.0.113.9", "--label", "monitor"}); err != nil {
		t.Fatal(err)
	}
	cfg, err = dataStore.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Routing.ForceLocal) != 1 || cfg.Routing.ForceLocal[0].CIDR != "203.0.113.9/32" {
		t.Fatalf("unexpected force routes: %+v", cfg.Routing.ForceLocal)
	}
}

package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"irroute/internal/model"
	"irroute/internal/store"
)

type fakeRunner struct {
	ruleOutput string
	batches    []string
	commands   []string
}

func (runner *fakeRunner) Run(name string, args ...string) (string, error) {
	command := name + " " + strings.Join(args, " ")
	runner.commands = append(runner.commands, command)
	if strings.Contains(command, "address show dev ens192") {
		return "2: ens192 inet 198.51.100.10/24", nil
	}
	if strings.Contains(command, "address show dev ens224") {
		return "3: ens224 inet 192.168.10.11/24", nil
	}
	if strings.Contains(command, "rule show priority") {
		return runner.ruleOutput, nil
	}
	if strings.Contains(command, "rule del priority") {
		return "", errors.New("rule not found")
	}
	if strings.Contains(command, "sysctl -n net.ipv4.conf.all.rp_filter") {
		return "2", nil
	}
	return "", nil
}

func (runner *fakeRunner) RunInput(input, name string, args ...string) (string, error) {
	runner.batches = append(runner.batches, input)
	return "", nil
}

func (*fakeRunner) LookPath(string) error { return nil }
func (*fakeRunner) IsLinux() bool         { return true }
func (*fakeRunner) IsRoot() bool          { return true }

func TestApplyAlternatesTablesAndDisableRemovesState(t *testing.T) {
	dataStore := configuredStore(t)
	runner := &fakeRunner{}
	routingEngine := New(dataStore, runner)

	first, err := routingEngine.Apply("test first apply")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Enabled || first.ActiveTable != 51820 || first.Generation != 1 {
		t.Fatalf("unexpected first state: %+v", first)
	}
	second, err := routingEngine.Apply("test second apply")
	if err != nil {
		t.Fatal(err)
	}
	if second.ActiveTable != 51821 || second.Generation != 2 {
		t.Fatalf("unexpected second state: %+v", second)
	}
	if len(runner.batches) != 4 {
		t.Fatalf("loaded %d batches, want 4", len(runner.batches))
	}
	commands := strings.Join(runner.commands, "\n")
	for _, expected := range []string{
		"rule add priority 10000 from all table main suppress_prefixlength 0",
		"rule add priority 10010 from 198.51.100.10/32 table 51810",
		"rule add priority 10020 from all table 51820",
		"resolvectl dns ens224 1.1.1.1 8.8.8.8",
		"resolvectl domain ens224 ~.",
		"resolvectl default-route ens192 no",
	} {
		if !strings.Contains(commands, expected) {
			t.Errorf("managed rule command %q was not executed\n%s", expected, commands)
		}
	}
	if !strings.Contains(runner.batches[0], "route add table 51820") || !strings.Contains(runner.batches[2], "route add table 51821") {
		t.Fatalf("A/B route loads were not performed in order: %v", runner.batches)
	}
	disabled, err := routingEngine.Disable("test disable")
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled || disabled.ActiveTable != 0 || disabled.Generation != 3 {
		t.Fatalf("unexpected disabled state: %+v", disabled)
	}
	commands = strings.Join(runner.commands, "\n")
	if !strings.Contains(commands, "resolvectl revert ens192") || !strings.Contains(commands, "resolvectl revert ens224") {
		t.Fatalf("disable did not restore per-link DNS settings:\n%s", commands)
	}
}

func TestApplyRejectsRuleCollisionBeforeLoadingTables(t *testing.T) {
	dataStore := configuredStore(t)
	runner := &fakeRunner{ruleOutput: "51800: from all lookup 99"}
	routingEngine := New(dataStore, runner)
	if _, err := routingEngine.Apply("collision test"); err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("Apply() error = %v, want collision error", err)
	}
	if len(runner.batches) != 0 {
		t.Fatalf("Apply() loaded tables before rejecting collision")
	}
}

func TestApplyRejectsTableCollisionBeforeLoadingTables(t *testing.T) {
	dataStore := configuredStore(t)
	runner := &tableCollisionRunner{fakeRunner: fakeRunner{}}
	routingEngine := New(dataStore, runner)
	if _, err := routingEngine.Apply("collision test"); err == nil || !strings.Contains(err.Error(), "table 51810 is already in use") {
		t.Fatalf("Apply() error = %v, want table collision error", err)
	}
	if len(runner.batches) != 0 {
		t.Fatalf("Apply() loaded tables before rejecting collision")
	}
}

type tableCollisionRunner struct {
	fakeRunner
}

func (runner *tableCollisionRunner) Run(name string, args ...string) (string, error) {
	command := name + " " + strings.Join(args, " ")
	if strings.Contains(command, "route show table 51810") {
		return "default via 192.0.2.1 dev eth9", nil
	}
	return runner.fakeRunner.Run(name, args...)
}

func configuredStore(t *testing.T) *store.Store {
	t.Helper()
	root := t.TempDir()
	dataStore := store.New(store.NewPaths(root))
	cfg := model.DefaultConfig()
	cfg.Local = model.Link{Interface: "ens192", Address: "198.51.100.10/24", Gateway: "198.51.100.1"}
	cfg.International = model.Link{Interface: "ens224", Address: "192.168.10.11/24", Gateway: "192.168.10.1"}
	if err := dataStore.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "iran.txt")
	if err := os.WriteFile(source, []byte("10.0.0.0/8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := dataStore.ImportIranCIDRs(source); err != nil {
		t.Fatal(err)
	}
	return dataStore
}

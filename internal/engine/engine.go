package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"irroute/internal/config"
	"irroute/internal/model"
	"irroute/internal/routing"
	"irroute/internal/store"
	sys "irroute/internal/system"
)

type Engine struct {
	Store  *store.Store
	Runner sys.Runner
}

type Check struct {
	Name    string
	OK      bool
	Message string
}

func New(dataStore *store.Store, runner sys.Runner) *Engine {
	return &Engine{Store: dataStore, Runner: runner}
}

func (e *Engine) Plan() (routing.Plan, error) {
	cfg, state, cidrs, err := e.load()
	if err != nil {
		return routing.Plan{}, err
	}
	return routing.Build(cfg, cidrs, state.ActiveTable)
}

func (e *Engine) Apply(reason string) (model.State, error) {
	lock, err := e.Store.AcquireLock()
	if err != nil {
		return model.State{}, err
	}
	defer lock.Release()
	return e.apply(reason)
}

func (e *Engine) apply(reason string) (model.State, error) {
	if err := e.requireHostAccess(); err != nil {
		return model.State{}, err
	}
	cfg, previous, cidrs, err := e.load()
	if err != nil {
		return model.State{}, err
	}
	if err := e.preflightNetwork(cfg); err != nil {
		return model.State{}, err
	}
	if !previous.Enabled {
		if err := e.rejectResourceCollisions(cfg); err != nil {
			return model.State{}, err
		}
	} else if err := e.validateManagedRules(cfg, previous); err != nil {
		return model.State{}, err
	}
	plan, err := routing.Build(cfg, cidrs, previous.ActiveTable)
	if err != nil {
		return model.State{}, err
	}
	snapshot, err := e.Store.WriteSnapshot(reason, cfg, previous, cidrs)
	if err != nil {
		return model.State{}, fmt.Errorf("create safety snapshot: %w", err)
	}
	if err := e.flushTable(plan.TargetTable); err != nil {
		return model.State{}, fmt.Errorf("flush target routing table: %w", err)
	}
	if _, err := e.Runner.RunInput(plan.TargetLoadBatch(), "ip", "-4", "-batch", "-"); err != nil {
		return model.State{}, fmt.Errorf("load target routing table: %w", err)
	}
	if _, err := e.Runner.RunInput(plan.LocalBatch(), "ip", "-4", "-batch", "-"); err != nil {
		return model.State{}, fmt.Errorf("load local source table: %w", err)
	}
	if err := e.switchRules(plan); err != nil {
		e.restoreRules(cfg, previous)
		return model.State{}, fmt.Errorf("activate routing rules: %w", err)
	}

	state := previous
	state.SchemaVersion = model.SchemaVersion
	state.Enabled = true
	state.ActiveTable = plan.TargetTable
	state.Generation++
	state.DataSHA256 = hashCIDRs(cidrs)
	state.LastAppliedAt = time.Now().UTC()
	state.LastSnapshot = snapshot
	if err := e.Store.SaveState(state); err != nil {
		e.restoreRules(cfg, previous)
		return model.State{}, fmt.Errorf("save state after routing change: %w", err)
	}
	return state, nil
}

func (e *Engine) Disable(reason string) (model.State, error) {
	lock, err := e.Store.AcquireLock()
	if err != nil {
		return model.State{}, err
	}
	defer lock.Release()
	return e.disable(reason)
}

func (e *Engine) disable(reason string) (model.State, error) {
	if err := e.requireHostAccess(); err != nil {
		return model.State{}, err
	}
	cfg, previous, cidrs, err := e.load()
	if err != nil {
		return model.State{}, err
	}
	snapshot, err := e.Store.WriteSnapshot(reason, cfg, previous, cidrs)
	if err != nil {
		return model.State{}, fmt.Errorf("create safety snapshot: %w", err)
	}
	if previous.Enabled {
		e.deletePriority(cfg.Routing.SourceRulePriority)
		e.deletePriority(cfg.Routing.MainRulePriority)
	}
	var cleanupProblems []string
	for _, table := range []int{cfg.Routing.LocalTable, cfg.Routing.TableA, cfg.Routing.TableB} {
		if err := e.flushTable(table); err != nil {
			cleanupProblems = append(cleanupProblems, fmt.Sprintf("flush routing table %d: %v", table, err))
		}
	}
	state := previous
	state.Enabled = false
	state.ActiveTable = 0
	state.Generation++
	state.LastAppliedAt = time.Now().UTC()
	state.LastSnapshot = snapshot
	if err := e.Store.SaveState(state); err != nil {
		return model.State{}, err
	}
	if len(cleanupProblems) > 0 {
		return state, fmt.Errorf("routing was disabled with cleanup errors: %s", strings.Join(cleanupProblems, "; "))
	}
	return state, nil
}

func (e *Engine) Rollback(snapshotPath string) (model.State, error) {
	lock, err := e.Store.AcquireLock()
	if err != nil {
		return model.State{}, err
	}
	defer lock.Release()
	return e.rollback(snapshotPath)
}

func (e *Engine) rollback(snapshotPath string) (model.State, error) {
	snapshot, err := e.Store.LoadSnapshot(snapshotPath)
	if err != nil {
		return model.State{}, err
	}
	if snapshot.SchemaVersion != model.SchemaVersion {
		return model.State{}, fmt.Errorf("snapshot schema version %d is not supported", snapshot.SchemaVersion)
	}
	if err := config.Validate(snapshot.Config); err != nil {
		return model.State{}, fmt.Errorf("snapshot configuration is invalid: %w", err)
	}
	currentState, err := e.Store.LoadState()
	if err != nil {
		return model.State{}, err
	}
	if currentState.Enabled {
		if _, err := e.disable("pre-rollback safety point"); err != nil {
			return model.State{}, fmt.Errorf("disable current generation before rollback: %w", err)
		}
	}
	if err := e.Store.SaveConfig(snapshot.Config); err != nil {
		return model.State{}, err
	}
	if err := e.Store.RestoreData(snapshot.IranCIDRs); err != nil {
		return model.State{}, err
	}
	restoredState := snapshot.State
	restoredState.Enabled = false
	restoredState.ActiveTable = 0
	if err := e.Store.SaveState(restoredState); err != nil {
		return model.State{}, err
	}
	if snapshot.State.Enabled {
		return e.apply("rollback activation")
	}
	return restoredState, nil
}

func (e *Engine) Doctor() []Check {
	checks := []Check{
		{Name: "operating system", OK: e.Runner.IsLinux(), Message: "Linux is required"},
		{Name: "privileges", OK: e.Runner.IsRoot(), Message: "root privileges are required for routing changes"},
	}
	if err := e.Runner.LookPath("ip"); err != nil {
		checks = append(checks, Check{Name: "iproute2", Message: "the ip command was not found"})
	} else {
		checks = append(checks, Check{Name: "iproute2", OK: true, Message: "ip command is available"})
	}
	cfg, err := e.Store.LoadConfig()
	if err != nil {
		checks = append(checks, Check{Name: "configuration", Message: err.Error()})
		return checks
	}
	if err := config.Validate(cfg); err != nil {
		checks = append(checks, Check{Name: "configuration", Message: err.Error()})
	} else {
		checks = append(checks, Check{Name: "configuration", OK: true, Message: "configuration is valid"})
	}
	cidrs, err := e.Store.LoadIranCIDRs()
	if err != nil {
		checks = append(checks, Check{Name: "Iran CIDR data", Message: err.Error()})
	} else {
		checks = append(checks, Check{Name: "Iran CIDR data", OK: true, Message: fmt.Sprintf("%d prefixes loaded", len(cidrs))})
	}
	if e.Runner.IsLinux() && e.Runner.LookPath("ip") == nil {
		state, stateErr := e.Store.LoadState()
		if stateErr != nil {
			checks = append(checks, Check{Name: "managed routing state", Message: stateErr.Error()})
		} else {
			resourceErr := e.rejectResourceCollisions(cfg)
			message := "managed priorities and tables are available"
			if state.Enabled {
				resourceErr = e.validateManagedRules(cfg, state)
				message = "live rules match saved state"
			}
			checks = append(checks, Check{Name: "managed routing state", OK: resourceErr == nil, Message: checkMessage(resourceErr, message)})
		}
		links := []struct {
			name string
			link model.Link
		}{{"local interface", cfg.Local}, {"international interface", cfg.International}}
		for _, item := range links {
			name, link := item.name, item.link
			err := sys.InterfaceExists(e.Runner, link.Interface)
			checks = append(checks, Check{Name: name, OK: err == nil, Message: checkMessage(err, "interface exists")})
			if err == nil {
				err = sys.InterfaceHasAddress(e.Runner, link.Interface, link.Address)
				checks = append(checks, Check{Name: name + " address", OK: err == nil, Message: checkMessage(err, "configured address is present")})
			}
			if err == nil && link.MAC != "" {
				actual, macErr := sys.InterfaceMAC(e.Runner, link.Interface)
				ok := macErr == nil && strings.EqualFold(actual, link.MAC)
				message := "pinned MAC address matches"
				if macErr != nil {
					message = macErr.Error()
				} else if !ok {
					message = fmt.Sprintf("expected %s, found %s", link.MAC, actual)
				}
				checks = append(checks, Check{Name: name + " identity", OK: ok, Message: message})
			}
		}
	}
	return checks
}

func (e *Engine) Status() (model.State, []string, error) {
	state, err := e.Store.LoadState()
	if err != nil {
		return state, nil, err
	}
	if !e.Runner.IsLinux() || e.Runner.LookPath("ip") != nil {
		return state, nil, nil
	}
	cfg, err := e.Store.LoadConfig()
	if err != nil {
		return state, nil, err
	}
	var rules []string
	for _, priority := range []int{cfg.Routing.SourceRulePriority, cfg.Routing.MainRulePriority} {
		output, _ := e.Runner.Run("ip", "-4", "-o", "rule", "show", "priority", fmt.Sprint(priority))
		if output != "" {
			rules = append(rules, strings.Split(output, "\n")...)
		}
	}
	return state, rules, nil
}

func (e *Engine) load() (model.Config, model.State, []string, error) {
	cfg, err := e.Store.LoadConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, model.State{}, nil, errors.New("configuration is missing; run irroute setup first")
		}
		return cfg, model.State{}, nil, err
	}
	if err := config.Validate(cfg); err != nil {
		return cfg, model.State{}, nil, fmt.Errorf("invalid configuration: %w", err)
	}
	state, err := e.Store.LoadState()
	if err != nil {
		return cfg, state, nil, err
	}
	cidrs, err := e.Store.LoadIranCIDRs()
	if err != nil {
		return cfg, state, nil, err
	}
	return cfg, state, cidrs, nil
}

func (e *Engine) requireHostAccess() error {
	if !e.Runner.IsLinux() {
		return errors.New("routing changes can only be applied on Linux")
	}
	if !e.Runner.IsRoot() {
		return errors.New("routing changes require root privileges")
	}
	if err := e.Runner.LookPath("ip"); err != nil {
		return errors.New("iproute2 is required but the ip command was not found")
	}
	return nil
}

func (e *Engine) preflightNetwork(cfg model.Config) error {
	links := []struct {
		name string
		link model.Link
	}{{"local", cfg.Local}, {"international", cfg.International}}
	for _, item := range links {
		name, link := item.name, item.link
		if err := sys.InterfaceExists(e.Runner, link.Interface); err != nil {
			return fmt.Errorf("%s interface: %w", name, err)
		}
		if err := sys.InterfaceHasAddress(e.Runner, link.Interface, link.Address); err != nil {
			return err
		}
		if link.MAC != "" {
			actual, err := sys.InterfaceMAC(e.Runner, link.Interface)
			if err != nil {
				return err
			}
			if !strings.EqualFold(actual, link.MAC) {
				return fmt.Errorf("%s interface MAC changed: expected %s, found %s", name, link.MAC, actual)
			}
		}
	}
	return nil
}

func (e *Engine) rejectResourceCollisions(cfg model.Config) error {
	for _, priority := range []int{cfg.Routing.SourceRulePriority, cfg.Routing.MainRulePriority} {
		output, err := e.Runner.Run("ip", "-4", "-o", "rule", "show", "priority", fmt.Sprint(priority))
		if err != nil {
			return err
		}
		if strings.TrimSpace(output) != "" {
			return fmt.Errorf("routing rule priority %d is already in use: %s", priority, output)
		}
	}
	for _, table := range []int{cfg.Routing.LocalTable, cfg.Routing.TableA, cfg.Routing.TableB} {
		output, err := e.Runner.Run("ip", "-4", "route", "show", "table", fmt.Sprint(table))
		if err != nil {
			if isMissingTable(err) {
				continue
			}
			return err
		}
		if strings.TrimSpace(output) != "" {
			return fmt.Errorf("routing table %d is already in use", table)
		}
	}
	return nil
}

func (e *Engine) flushTable(table int) error {
	_, err := e.Runner.Run("ip", "-4", "route", "flush", "table", fmt.Sprint(table))
	if isMissingTable(err) {
		return nil
	}
	return err
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "fib table does not exist") || strings.Contains(message, "table does not exist")
}

func (e *Engine) validateManagedRules(cfg model.Config, state model.State) error {
	expected := []struct {
		priority int
		parts    []string
	}{
		{cfg.Routing.SourceRulePriority, []string{strings.SplitN(cfg.Local.Address, "/", 2)[0], "lookup " + fmt.Sprint(cfg.Routing.LocalTable)}},
		{cfg.Routing.MainRulePriority, []string{"from all", "lookup " + fmt.Sprint(state.ActiveTable)}},
	}
	for _, item := range expected {
		output, err := e.ruleAtPriority(item.priority)
		if err != nil {
			return err
		}
		if output == "" {
			continue
		}
		for _, part := range item.parts {
			if !strings.Contains(output, part) {
				return fmt.Errorf("managed rule priority %d does not match saved state: %s", item.priority, output)
			}
		}
	}
	return nil
}

func (e *Engine) switchRules(plan routing.Plan) error {
	output, err := e.ruleAtPriority(plan.SourceRulePriority)
	if err != nil {
		return err
	}
	if output == "" {
		if _, err := e.Runner.Run("ip", "-4", "rule", "add", "priority", fmt.Sprint(plan.SourceRulePriority), "from", plan.PublicSource.String()+"/32", "table", fmt.Sprint(plan.LocalTable)); err != nil {
			return err
		}
	}
	e.deletePriority(plan.MainRulePriority)
	if _, err := e.Runner.Run("ip", "-4", "rule", "add", "priority", fmt.Sprint(plan.MainRulePriority), "from", "all", "table", fmt.Sprint(plan.TargetTable)); err != nil {
		return err
	}
	return nil
}

func (e *Engine) ruleAtPriority(priority int) (string, error) {
	output, err := e.Runner.Run("ip", "-4", "-o", "rule", "show", "priority", fmt.Sprint(priority))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (e *Engine) restoreRules(cfg model.Config, state model.State) {
	e.deletePriority(cfg.Routing.SourceRulePriority)
	e.deletePriority(cfg.Routing.MainRulePriority)
	if !state.Enabled || state.ActiveTable == 0 {
		return
	}
	localAddress := strings.SplitN(cfg.Local.Address, "/", 2)[0]
	e.Runner.Run("ip", "-4", "rule", "add", "priority", fmt.Sprint(cfg.Routing.SourceRulePriority), "from", localAddress+"/32", "table", fmt.Sprint(cfg.Routing.LocalTable))
	e.Runner.Run("ip", "-4", "rule", "add", "priority", fmt.Sprint(cfg.Routing.MainRulePriority), "from", "all", "table", fmt.Sprint(state.ActiveTable))
}

func (e *Engine) deletePriority(priority int) {
	for index := 0; index < 16; index++ {
		if _, err := e.Runner.Run("ip", "-4", "rule", "del", "priority", fmt.Sprint(priority)); err != nil {
			return
		}
	}
}

func hashCIDRs(cidrs []string) string {
	copyOfCIDRs := append([]string(nil), cidrs...)
	sort.Strings(copyOfCIDRs)
	sum := sha256.Sum256([]byte(strings.Join(copyOfCIDRs, "\n") + "\n"))
	return hex.EncodeToString(sum[:])
}

func checkMessage(err error, success string) string {
	if err != nil {
		return err.Error()
	}
	return success
}

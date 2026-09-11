package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"irroute/internal/config"
	"irroute/internal/engine"
	"irroute/internal/model"
	"irroute/internal/routing"
	"irroute/internal/store"
	sys "irroute/internal/system"
)

type App struct {
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Store   *store.Store
	Engine  *engine.Engine
	Runner  sys.Runner
	Version string
}

func New(in io.Reader, out, errOut io.Writer, dataStore *store.Store, runner sys.Runner, version string) *App {
	return &App{
		In:      in,
		Out:     out,
		Err:     errOut,
		Store:   dataStore,
		Engine:  engine.New(dataStore, runner),
		Runner:  runner,
		Version: version,
	}
}

func (a *App) Run(args []string) error {
	if len(args) == 0 {
		return a.menu()
	}
	switch args[0] {
	case "setup":
		return a.setup()
	case "status":
		return a.status()
	case "doctor":
		return a.doctor()
	case "plan":
		return a.plan(args[1:])
	case "enable":
		return a.apply(false, hasArgument(args[1:], "--no-confirm"), true)
	case "apply":
		return a.apply(hasArgument(args[1:], "--boot"), hasArgument(args[1:], "--no-confirm"), false)
	case "disable":
		return a.disable()
	case "force":
		return a.force(args[1:])
	case "dns":
		return a.dns(args[1:])
	case "data":
		return a.data(args[1:])
	case "rollback":
		path := "latest"
		if len(args) > 1 {
			path = args[1]
		}
		state, err := a.Engine.Rollback(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Rollback completed. Enabled: %t, generation: %d\n", state.Enabled, state.Generation)
		return nil
	case "version", "--version", "-v":
		fmt.Fprintf(a.Out, "irroute %s\n", a.Version)
		return nil
	case "help", "--help", "-h":
		a.usage()
		return nil
	default:
		a.usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (a *App) menu() error {
	reader := bufio.NewReader(a.In)
	for {
		fmt.Fprintln(a.Out, "\nirroute - Policy Routing Manager")
		fmt.Fprintln(a.Out, "1) Status")
		fmt.Fprintln(a.Out, "2) Setup wizard")
		fmt.Fprintln(a.Out, "3) Enable or apply")
		fmt.Fprintln(a.Out, "4) Disable")
		fmt.Fprintln(a.Out, "5) Show plan")
		fmt.Fprintln(a.Out, "6) Manage force routes")
		fmt.Fprintln(a.Out, "7) Manage DNS")
		fmt.Fprintln(a.Out, "8) Import Iran CIDR data")
		fmt.Fprintln(a.Out, "9) Run diagnostics")
		fmt.Fprintln(a.Out, "10) Roll back latest change")
		fmt.Fprintln(a.Out, "0) Exit")
		choice, err := prompt(reader, a.Out, "Select an option", "")
		if err != nil {
			return err
		}
		switch choice {
		case "1":
			if err := a.status(); err != nil {
				fmt.Fprintf(a.Err, "Error: %v\n", err)
			}
		case "2":
			if err := a.setupWithReader(reader); err != nil {
				fmt.Fprintf(a.Err, "Error: %v\n", err)
			}
		case "3":
			if confirm(reader, a.Out, "Apply routing now?", false) {
				if err := a.apply(false, false, true); err != nil {
					fmt.Fprintf(a.Err, "Error: %v\n", err)
				}
			}
		case "4":
			if confirm(reader, a.Out, "Disable irroute routing now?", false) {
				if err := a.disable(); err != nil {
					fmt.Fprintf(a.Err, "Error: %v\n", err)
				}
			}
		case "5":
			if err := a.plan(nil); err != nil {
				fmt.Fprintf(a.Err, "Error: %v\n", err)
			}
		case "6":
			if err := a.forceMenu(reader); err != nil {
				fmt.Fprintf(a.Err, "Error: %v\n", err)
			}
		case "7":
			if err := a.dnsMenu(reader); err != nil {
				fmt.Fprintf(a.Err, "Error: %v\n", err)
			}
		case "8":
			path, err := prompt(reader, a.Out, "CIDR file path", "")
			if err == nil && path != "" {
				err = a.importData(path)
			}
			if err != nil {
				fmt.Fprintf(a.Err, "Error: %v\n", err)
			}
		case "9":
			_ = a.doctor()
		case "10":
			if confirm(reader, a.Out, "Restore the latest snapshot?", false) {
				state, err := a.Engine.Rollback("latest")
				if err != nil {
					fmt.Fprintf(a.Err, "Error: %v\n", err)
				} else {
					fmt.Fprintf(a.Out, "Rollback completed. Enabled: %t\n", state.Enabled)
				}
			}
		case "0":
			return nil
		default:
			fmt.Fprintln(a.Err, "Invalid option.")
		}
	}
}

func (a *App) setup() error {
	return a.setupWithReader(bufio.NewReader(a.In))
}

func (a *App) setupWithReader(reader *bufio.Reader) error {
	cfg := model.DefaultConfig()
	if existing, err := a.Store.LoadConfig(); err == nil {
		cfg = existing
	}
	if state, err := a.Store.LoadState(); err == nil && state.Enabled {
		return errors.New("disable irroute before changing interface setup")
	}
	interfaces, _ := sys.DiscoverInterfaces(a.Runner)
	gateways, _ := sys.DiscoverDefaultGateways(a.Runner)

	fmt.Fprintln(a.Out, "\nSetup wizard")
	fmt.Fprintln(a.Out, "Local means the public interface used for Iran destinations.")
	fmt.Fprintln(a.Out, "International means the interface used for all other destinations.")

	localInterface, localDetected, err := selectInterface(reader, a.Out, "Select the local interface", cfg.Local.Interface, interfaces, "")
	if err != nil {
		return err
	}
	cfg.Local.Interface = localInterface
	cfg.Local.MAC = localDetected.MAC
	localAddressDefault := cfg.Local.Address
	if localAddressDefault == "" && len(localDetected.Addresses) > 0 {
		localAddressDefault = localDetected.Addresses[0]
	}
	if cfg.Local.Address, err = prompt(reader, a.Out, "Local IPv4 address in CIDR notation", localAddressDefault); err != nil {
		return err
	}
	localGatewayDefault := cfg.Local.Gateway
	if localGatewayDefault == "" {
		localGatewayDefault = gateways[localInterface]
	}
	if cfg.Local.Gateway, err = prompt(reader, a.Out, "Local gateway", localGatewayDefault); err != nil {
		return err
	}

	internationalInterface, internationalDetected, err := selectInterface(reader, a.Out, "Select the international interface", cfg.International.Interface, interfaces, localInterface)
	if err != nil {
		return err
	}
	cfg.International.Interface = internationalInterface
	cfg.International.MAC = internationalDetected.MAC
	internationalAddressDefault := cfg.International.Address
	if internationalAddressDefault == "" && len(internationalDetected.Addresses) > 0 {
		internationalAddressDefault = internationalDetected.Addresses[0]
	}
	if cfg.International.Address, err = prompt(reader, a.Out, "International IPv4 address in CIDR notation", internationalAddressDefault); err != nil {
		return err
	}
	internationalGatewayDefault := cfg.International.Gateway
	if internationalGatewayDefault == "" {
		internationalGatewayDefault = gateways[internationalInterface]
	}
	if cfg.International.Gateway, err = prompt(reader, a.Out, "International gateway", internationalGatewayDefault); err != nil {
		return err
	}

	cfg.Routing.FailurePolicy = "manual"
	fmt.Fprintln(a.Out, "Failure policy: manual (phase one)")
	dnsDefault := cfg.DNS.Mode
	if dnsDefault == "" {
		dnsDefault = "managed"
	}
	dnsMode, err := prompt(reader, a.Out, "DNS mode (managed or system)", dnsDefault)
	if err != nil {
		return err
	}
	cfg.DNS.Mode = strings.ToLower(strings.TrimSpace(dnsMode))
	if cfg.DNS.Mode == "managed" {
		serversDefault := strings.Join(cfg.DNS.International, ",")
		if serversDefault == "" {
			serversDefault = "1.1.1.1,8.8.8.8"
		}
		servers, err := prompt(reader, a.Out, "International DNS servers (comma-separated IPv4 addresses)", serversDefault)
		if err != nil {
			return err
		}
		cfg.DNS.International = splitCommaList(servers)
		cfg.DNS.Local = nil
	} else if cfg.DNS.Mode == "system" {
		cfg.DNS.Local = nil
		cfg.DNS.International = nil
	}
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("setup answers are invalid: %w", err)
	}

	dataDefault := ""
	if _, err := os.Stat(a.Store.Paths.IranFile); err == nil {
		dataDefault = a.Store.Paths.IranFile
	}
	dataPath, err := prompt(reader, a.Out, "Iran CIDR source file", dataDefault)
	if err != nil {
		return err
	}
	if dataPath == "" {
		return errors.New("an Iran CIDR source file is required")
	}
	if err := a.Store.SaveConfig(cfg); err != nil {
		return err
	}
	importedHash := ""
	if filepath.Clean(dataPath) != filepath.Clean(a.Store.Paths.IranFile) {
		count, hash, err := a.Store.ImportIranCIDRs(dataPath)
		if err != nil {
			return fmt.Errorf("import Iran CIDR data: %w", err)
		}
		importedHash = hash
		fmt.Fprintf(a.Out, "Imported %d prefixes. SHA-256: %s\n", count, hash)
	} else if _, err := a.Store.LoadIranCIDRs(); err != nil {
		return err
	}
	if importedHash != "" {
		state, err := a.Store.LoadState()
		if err != nil {
			return err
		}
		state.DataSHA256 = importedHash
		if err := a.Store.SaveState(state); err != nil {
			return err
		}
	}
	fmt.Fprintf(a.Out, "Configuration saved to %s\n", a.Store.Paths.ConfigFile)
	fmt.Fprintln(a.Out, "Run 'irroute doctor', review 'irroute plan', then run 'irroute enable'.")
	return nil
}

func (a *App) status() error {
	state, rules, err := a.Engine.Status()
	if err != nil {
		return err
	}
	status := "disabled"
	if state.Enabled {
		status = "enabled"
	}
	fmt.Fprintf(a.Out, "Status: %s\n", status)
	fmt.Fprintf(a.Out, "Active table: %d\n", state.ActiveTable)
	fmt.Fprintf(a.Out, "Generation: %d\n", state.Generation)
	fmt.Fprintf(a.Out, "Managed DNS active: %t\n", state.DNSManaged)
	if !state.LastAppliedAt.IsZero() {
		fmt.Fprintf(a.Out, "Last applied: %s\n", state.LastAppliedAt.Format("2006-01-02 15:04:05Z"))
	}
	if state.DataSHA256 != "" {
		fmt.Fprintf(a.Out, "Data SHA-256: %s\n", state.DataSHA256)
	}
	if len(rules) == 0 && state.Enabled && a.Runner.IsLinux() {
		fmt.Fprintln(a.Out, "Warning: state is enabled but managed rules were not found.")
	}
	for _, rule := range rules {
		fmt.Fprintf(a.Out, "Rule: %s\n", rule)
	}
	return nil
}

func (a *App) doctor() error {
	failed := false
	for _, check := range a.Engine.Doctor() {
		label := "PASS"
		if !check.OK {
			label = "FAIL"
			failed = true
		}
		fmt.Fprintf(a.Out, "[%s] %s: %s\n", label, check.Name, check.Message)
	}
	if failed {
		return errors.New("one or more diagnostic checks failed")
	}
	return nil
}

func (a *App) plan(args []string) error {
	plan, err := a.Engine.Plan()
	if err != nil {
		return err
	}
	fmt.Fprint(a.Out, plan.Summary())
	if hasArgument(args, "--commands") {
		fmt.Fprintln(a.Out, "\nLocal table batch:")
		fmt.Fprint(a.Out, plan.LocalBatch())
		fmt.Fprintln(a.Out, "\nTarget table batch:")
		fmt.Fprint(a.Out, plan.TargetBatch())
	}
	return nil
}

func (a *App) apply(boot, noConfirm, enablePersistence bool) error {
	reason := "manual enable"
	if boot {
		reason = "boot apply"
	}
	rollbackUnit := ""
	if !boot && !noConfirm {
		snapshot, err := a.captureSnapshot("before interactive apply")
		if err != nil {
			return fmt.Errorf("create pre-apply safety snapshot: %w", err)
		}
		rollbackUnit, err = a.scheduleConnectivityRollback(snapshot)
		if err != nil {
			return err
		}
	}
	state, err := a.Engine.Apply(reason)
	if err != nil {
		a.cancelConnectivityRollback(rollbackUnit)
		return err
	}
	fmt.Fprintf(a.Out, "Routing enabled on table %d. Generation: %d\n", state.ActiveTable, state.Generation)
	if !boot {
		if !noConfirm {
			if err := a.confirmActivation(rollbackUnit); err != nil {
				return err
			}
		} else {
			fmt.Fprintln(a.Err, "Warning: automatic connectivity rollback was explicitly disabled.")
		}
		if enablePersistence {
			if err := a.setServiceEnabled(true); err != nil {
				return fmt.Errorf("routing is active but boot persistence could not be enabled: %w", err)
			}
		}
	}
	return nil
}

func (a *App) disable() error {
	if err := a.setServiceEnabled(false); err != nil {
		return fmt.Errorf("disable boot persistence before removing routes: %w", err)
	}
	state, err := a.Engine.Disable("manual disable")
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "Routing disabled. Generation: %d\n", state.Generation)
	return nil
}

func (a *App) force(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: irroute force <local|international> <add|remove|list> [CIDR] [--label text]")
	}
	if args[0] != "local" && args[0] != "international" {
		return errors.New("force route target must be local or international")
	}
	if len(args) < 2 {
		return errors.New("force route action must be add, remove, or list")
	}
	cfg, err := a.Store.LoadConfig()
	if err != nil {
		return err
	}
	entries := &cfg.Routing.ForceLocal
	if args[0] == "international" {
		entries = &cfg.Routing.ForceInternational
	}
	switch args[1] {
	case "list":
		if len(*entries) == 0 {
			fmt.Fprintln(a.Out, "No force routes configured.")
			return nil
		}
		for _, entry := range *entries {
			fmt.Fprintf(a.Out, "%s\t%s\n", entry.CIDR, entry.Label)
		}
		return nil
	case "add", "remove":
		if len(args) < 3 {
			return errors.New("a CIDR or IPv4 address is required")
		}
		prefix, err := routing.ParseIPv4Prefix(args[2])
		if err != nil {
			return err
		}
		oldConfig := cloneConfig(cfg)
		if args[1] == "add" {
			label := argumentValue(args[3:], "--label")
			*entries = upsertEntry(*entries, model.RouteEntry{CIDR: prefix.String(), Label: label})
		} else {
			var removed bool
			*entries, removed = removeEntry(*entries, prefix.String())
			if !removed {
				return fmt.Errorf("force route %s was not found", prefix)
			}
		}
		if err := config.Validate(cfg); err != nil {
			return err
		}
		if err := a.saveAndMaybeApply(oldConfig, cfg, "force route change"); err != nil {
			return err
		}
		fmt.Fprintf(a.Out, "Force-%s route %s %s.\n", args[0], prefix, pastTense(args[1]))
		return nil
	default:
		return errors.New("force route action must be add, remove, or list")
	}
}

func (a *App) forceMenu(reader *bufio.Reader) error {
	target, err := prompt(reader, a.Out, "Target (local or international)", "local")
	if err != nil {
		return err
	}
	action, err := prompt(reader, a.Out, "Action (add, remove, or list)", "list")
	if err != nil {
		return err
	}
	args := []string{target, action}
	if action == "add" || action == "remove" {
		cidr, err := prompt(reader, a.Out, "IPv4 address or CIDR", "")
		if err != nil {
			return err
		}
		args = append(args, cidr)
		if action == "add" {
			label, err := prompt(reader, a.Out, "Optional label", "")
			if err != nil {
				return err
			}
			if label != "" {
				args = append(args, "--label", label)
			}
		}
	}
	return a.force(args)
}

func (a *App) dns(args []string) error {
	cfg, err := a.Store.LoadConfig()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "show" {
		fmt.Fprintf(a.Out, "DNS mode: %s\n", cfg.DNS.Mode)
		if len(cfg.DNS.International) > 0 {
			fmt.Fprintf(a.Out, "International DNS servers: %s\n", strings.Join(cfg.DNS.International, ", "))
		}
		return nil
	}
	oldConfig := cloneConfig(cfg)
	switch args[0] {
	case "set":
		if len(args) < 2 {
			return errors.New("usage: irroute dns set <IPv4> [IPv4 ...]")
		}
		cfg.DNS.Mode = "managed"
		cfg.DNS.Local = nil
		cfg.DNS.International = append([]string(nil), args[1:]...)
	case "system":
		cfg.DNS.Mode = "system"
		cfg.DNS.Local = nil
		cfg.DNS.International = nil
	default:
		return errors.New("usage: irroute dns <show|set|system>")
	}
	if err := config.Validate(cfg); err != nil {
		return err
	}
	if err := a.saveAndMaybeApply(oldConfig, cfg, "DNS configuration change"); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "DNS mode changed to %s.\n", cfg.DNS.Mode)
	return nil
}

func (a *App) dnsMenu(reader *bufio.Reader) error {
	action, err := prompt(reader, a.Out, "DNS action (show, set, or system)", "show")
	if err != nil {
		return err
	}
	args := []string{action}
	if action == "set" {
		servers, err := prompt(reader, a.Out, "International DNS servers (comma-separated IPv4 addresses)", "1.1.1.1,8.8.8.8")
		if err != nil {
			return err
		}
		args = append(args, splitCommaList(servers)...)
	}
	return a.dns(args)
}

func (a *App) data(args []string) error {
	if len(args) == 2 && args[0] == "import" {
		return a.importData(args[1])
	}
	return errors.New("usage: irroute data import <file>")
}

func (a *App) importData(path string) error {
	oldData, oldDataErr := a.Store.LoadIranCIDRs()
	cfg, cfgErr := a.Store.LoadConfig()
	stateBefore, stateErr := a.Store.LoadState()
	rollbackSnapshot := ""
	if oldDataErr == nil && cfgErr == nil && stateErr == nil {
		rollbackSnapshot, _ = a.Store.WriteSnapshot("before Iran CIDR data import", cfg, stateBefore, oldData)
	}
	count, hash, err := a.Store.ImportIranCIDRs(path)
	if err != nil {
		return err
	}
	state, err := a.Store.LoadState()
	if err != nil {
		return err
	}
	if state.Enabled {
		if rollbackSnapshot == "" {
			return errors.New("could not create a safe rollback snapshot for the data change")
		}
		rollbackUnit, err := a.scheduleConnectivityRollback(rollbackSnapshot)
		if err != nil {
			if oldDataErr == nil {
				_ = a.Store.RestoreData(oldData)
			}
			return err
		}
		_, err = a.Engine.Apply("Iran CIDR data import")
		if err != nil {
			a.cancelConnectivityRollback(rollbackUnit)
			if oldDataErr == nil {
				_ = a.Store.RestoreData(oldData)
			}
			return fmt.Errorf("new data was not activated and previous data was restored: %w", err)
		}
		if err := a.confirmActivation(rollbackUnit); err != nil {
			return err
		}
	} else {
		state.DataSHA256 = hash
		if err := a.Store.SaveState(state); err != nil {
			return err
		}
	}
	fmt.Fprintf(a.Out, "Imported %d prefixes. SHA-256: %s\n", count, hash)
	return nil
}

func (a *App) saveAndMaybeApply(oldConfig, newConfig model.Config, reason string) error {
	stateBefore, stateErr := a.Store.LoadState()
	dataBefore, dataErr := a.Store.LoadIranCIDRs()
	rollbackSnapshot := ""
	if stateErr == nil && dataErr == nil {
		rollbackSnapshot, _ = a.Store.WriteSnapshot("before "+reason, oldConfig, stateBefore, dataBefore)
	}
	if err := a.Store.SaveConfig(newConfig); err != nil {
		return err
	}
	state, err := a.Store.LoadState()
	if err != nil {
		return err
	}
	if !state.Enabled {
		return nil
	}
	if rollbackSnapshot == "" {
		_ = a.Store.SaveConfig(oldConfig)
		return errors.New("could not create a safe rollback snapshot for the configuration change")
	}
	rollbackUnit, err := a.scheduleConnectivityRollback(rollbackSnapshot)
	if err != nil {
		_ = a.Store.SaveConfig(oldConfig)
		return err
	}
	_, err = a.Engine.Apply(reason)
	if err != nil {
		a.cancelConnectivityRollback(rollbackUnit)
		_ = a.Store.SaveConfig(oldConfig)
		return fmt.Errorf("change was not activated and previous configuration was restored: %w", err)
	}
	if err := a.confirmActivation(rollbackUnit); err != nil {
		return err
	}
	return nil
}

func (a *App) captureSnapshot(reason string) (string, error) {
	cfg, err := a.Store.LoadConfig()
	if err != nil {
		return "", err
	}
	state, err := a.Store.LoadState()
	if err != nil {
		return "", err
	}
	cidrs, err := a.Store.LoadIranCIDRs()
	if err != nil {
		return "", err
	}
	return a.Store.WriteSnapshot(reason, cfg, state, cidrs)
}

func (a *App) scheduleConnectivityRollback(rollbackSnapshot string) (string, error) {
	if !a.Runner.IsLinux() || a.Store.Paths.Root != "/" {
		return "", nil
	}
	if a.Runner.LookPath("systemd-run") != nil || a.Runner.LookPath("systemctl") != nil {
		return "", errors.New("systemd is required for connectivity-confirmed routing changes; no changes were applied")
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find irroute executable: %w", err)
	}
	unit := fmt.Sprintf("irroute-confirm-rollback-%d", time.Now().UnixNano())
	_, err = a.Runner.Run(
		"systemd-run",
		"--unit="+unit,
		"--on-active=120s",
		"--timer-property=AccuracySec=1s",
		"--collect",
		executable,
		"rollback",
		rollbackSnapshot,
	)
	if err != nil {
		return "", fmt.Errorf("schedule connectivity rollback: %w; no changes were applied", err)
	}
	return unit, nil
}

func (a *App) confirmActivation(unit string) error {
	if unit == "" {
		return nil
	}
	fmt.Fprintln(a.Out, "A safety rollback is scheduled in 120 seconds.")
	fmt.Fprintln(a.Out, "Verify that this SSH session and both network paths still work.")
	fmt.Fprint(a.Out, "Type CONFIRM within 90 seconds to keep this configuration: ")
	response := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(a.In)
		value, _ := reader.ReadString('\n')
		response <- strings.TrimSpace(value)
	}()
	select {
	case value := <-response:
		if value != "CONFIRM" {
			return errors.New("confirmation was not received; the scheduled rollback remains active")
		}
	case <-time.After(90 * time.Second):
		return errors.New("confirmation timed out; the scheduled rollback remains active")
	}
	if _, err := a.Runner.Run("systemctl", "stop", unit+".timer"); err != nil {
		return fmt.Errorf("cancel scheduled rollback: %w", err)
	}
	fmt.Fprintln(a.Out, "Configuration confirmed. The safety rollback was cancelled.")
	return nil
}

func (a *App) cancelConnectivityRollback(unit string) {
	if unit == "" {
		return
	}
	_, _ = a.Runner.Run("systemctl", "stop", unit+".timer")
}

func (a *App) setServiceEnabled(enabled bool) error {
	if !a.Runner.IsLinux() || a.Store.Paths.Root != "/" || a.Runner.LookPath("systemctl") != nil {
		return nil
	}
	args := []string{"disable", "--now", "irroute.service"}
	if enabled {
		args = []string{"enable", "irroute.service"}
	}
	if _, err := a.Runner.Run("systemctl", args...); err != nil {
		return err
	}
	return nil
}

func (a *App) usage() {
	fmt.Fprintln(a.Out, `Usage: irroute [command]

Commands:
  setup                                      Run the interactive setup wizard
  status                                     Show saved and live status
  doctor                                     Run safety and configuration checks
  plan [--commands]                          Preview the next routing generation
  enable [--no-confirm]                      Apply routing and enable boot persistence
  apply [--boot] [--no-confirm]              Apply routing configuration
  disable                                    Remove managed rules and disable persistence
  force <local|international> add <CIDR>      Add or update a force route
  force <local|international> remove <CIDR>   Remove a force route
  force <local|international> list            List force routes
  dns show                                    Show DNS policy
  dns set <IPv4> [IPv4 ...]                   Route DNS through the international interface
  dns system                                  Return DNS control to the system network configuration
  data import <file>                          Import and validate Iran CIDR data
  rollback [latest|snapshot]                  Restore a safety snapshot
  version                                     Show the version
  help                                        Show this help

Run irroute without a command for the interactive menu.`)
}

func selectInterface(reader *bufio.Reader, out io.Writer, message, current string, interfaces []sys.Interface, excluded string) (string, sys.Interface, error) {
	var choices []sys.Interface
	for _, item := range interfaces {
		if item.Name != excluded {
			choices = append(choices, item)
		}
	}
	if len(choices) == 0 {
		value, err := prompt(reader, out, message, current)
		return value, sys.Interface{Name: value}, err
	}
	for index, item := range choices {
		addresses := strings.Join(item.Addresses, ", ")
		if addresses == "" {
			addresses = "no IPv4 address"
		}
		fmt.Fprintf(out, "%d) %s (%s)\n", index+1, item.Name, addresses)
	}
	defaultChoice := ""
	for index, item := range choices {
		if item.Name == current {
			defaultChoice = strconv.Itoa(index + 1)
		}
	}
	value, err := prompt(reader, out, message, defaultChoice)
	if err != nil {
		return "", sys.Interface{}, err
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 1 || index > len(choices) {
		return "", sys.Interface{}, errors.New("invalid interface selection")
	}
	return choices[index-1].Name, choices[index-1], nil
}

func prompt(reader *bufio.Reader, out io.Writer, message, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(out, "%s [%s]: ", message, defaultValue)
	} else {
		fmt.Fprintf(out, "%s: ", message)
	}
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = defaultValue
	}
	return value, nil
}

func confirm(reader *bufio.Reader, out io.Writer, message string, defaultValue bool) bool {
	defaultText := "y/N"
	if defaultValue {
		defaultText = "Y/n"
	}
	value, err := prompt(reader, out, message+" ("+defaultText+")", "")
	if err != nil || value == "" {
		return defaultValue
	}
	return strings.EqualFold(value, "y") || strings.EqualFold(value, "yes")
}

func upsertEntry(entries []model.RouteEntry, newEntry model.RouteEntry) []model.RouteEntry {
	found := false
	for index := range entries {
		prefix, err := routing.ParseIPv4Prefix(entries[index].CIDR)
		if err == nil && prefix.String() == newEntry.CIDR {
			entries[index] = newEntry
			found = true
		}
	}
	if !found {
		entries = append(entries, newEntry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CIDR < entries[j].CIDR })
	return entries
}

func removeEntry(entries []model.RouteEntry, cidr string) ([]model.RouteEntry, bool) {
	result := make([]model.RouteEntry, 0, len(entries))
	removed := false
	for _, entry := range entries {
		prefix, err := routing.ParseIPv4Prefix(entry.CIDR)
		if err == nil && prefix.String() == cidr {
			removed = true
			continue
		}
		result = append(result, entry)
	}
	return result, removed
}

func argumentValue(args []string, key string) string {
	for index := 0; index < len(args)-1; index++ {
		if args[index] == key {
			return args[index+1]
		}
	}
	return ""
}

func hasArgument(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func pastTense(action string) string {
	if action == "add" {
		return "added"
	}
	return "removed"
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func cloneConfig(cfg model.Config) model.Config {
	cloned := cfg
	cloned.Routing.ForceLocal = append([]model.RouteEntry(nil), cfg.Routing.ForceLocal...)
	cloned.Routing.ForceInternational = append([]model.RouteEntry(nil), cfg.Routing.ForceInternational...)
	cloned.DNS.Local = append([]string(nil), cfg.DNS.Local...)
	cloned.DNS.International = append([]string(nil), cfg.DNS.International...)
	return cloned
}

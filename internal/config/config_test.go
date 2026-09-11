package config

import (
	"strings"
	"testing"

	"irroute/internal/model"
)

func TestValidateAcceptsValidConfiguration(t *testing.T) {
	cfg := validConfig()
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() returned %v", err)
	}
}

func TestValidateReportsMultipleProblems(t *testing.T) {
	cfg := validConfig()
	cfg.International.Interface = cfg.Local.Interface
	cfg.Routing.MainRulePriority = cfg.Routing.SourceRulePriority
	err := Validate(cfg)
	if err == nil {
		t.Fatal("Validate() accepted invalid configuration")
	}
	for _, expected := range []string{"interfaces must be different", "rule priorities must be ordered"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q does not contain %q", err, expected)
		}
	}
}

func TestNormalizeConfigMigratesUnsafeLegacyPriorities(t *testing.T) {
	cfg := validConfig()
	cfg.Routing.MainRoutesPriority = 0
	cfg.Routing.SourceRulePriority = 51800
	cfg.Routing.MainRulePriority = 51810
	cfg = model.NormalizeConfig(cfg)
	if cfg.Routing.MainRoutesPriority != 10000 || cfg.Routing.SourceRulePriority != 10010 || cfg.Routing.MainRulePriority != 10020 {
		t.Fatalf("legacy priorities were not migrated: %+v", cfg.Routing)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("migrated configuration is invalid: %v", err)
	}
}

func TestValidateRejectsRulesAfterSystemMain(t *testing.T) {
	cfg := validConfig()
	cfg.Routing.MainRoutesPriority = 51790
	cfg.Routing.SourceRulePriority = 51800
	cfg.Routing.MainRulePriority = 51810
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "must precede the system main rule") {
		t.Fatalf("Validate() error = %v, want system main priority error", err)
	}
}

func TestValidateManagedDNS(t *testing.T) {
	cfg := validConfig()
	cfg.DNS.International = nil
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "at least one international DNS server") {
		t.Fatalf("Validate() error = %v, want missing DNS server error", err)
	}
	cfg.DNS.International = []string{"not-an-address"}
	if err := Validate(cfg); err == nil || !strings.Contains(err.Error(), "valid IPv4 address") {
		t.Fatalf("Validate() error = %v, want invalid DNS address error", err)
	}
}

func validConfig() model.Config {
	cfg := model.DefaultConfig()
	cfg.Local = model.Link{Interface: "ens192", Address: "198.51.100.10/24", Gateway: "198.51.100.1"}
	cfg.International = model.Link{Interface: "ens224", Address: "192.168.10.11/24", Gateway: "192.168.10.1"}
	return cfg
}

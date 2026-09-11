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
	for _, expected := range []string{"interfaces must be different", "source rule priority"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q does not contain %q", err, expected)
		}
	}
}

func validConfig() model.Config {
	cfg := model.DefaultConfig()
	cfg.Local = model.Link{Interface: "ens192", Address: "198.51.100.10/24", Gateway: "198.51.100.1"}
	cfg.International = model.Link{Interface: "ens224", Address: "192.168.10.11/24", Gateway: "192.168.10.1"}
	return cfg
}

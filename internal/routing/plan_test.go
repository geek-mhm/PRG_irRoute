package routing

import (
	"strings"
	"testing"

	"irroute/internal/model"
)

func TestBuildUsesInactiveTableAndForceOverrides(t *testing.T) {
	cfg := testConfig()
	cfg.Routing.ForceLocal = []model.RouteEntry{{CIDR: "203.0.113.9"}}
	cfg.Routing.ForceInternational = []model.RouteEntry{{CIDR: "10.0.0.64/26"}}
	plan, err := Build(cfg, []string{"10.0.0.0/24"}, cfg.Routing.TableA)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetTable != cfg.Routing.TableB {
		t.Fatalf("target table = %d, want %d", plan.TargetTable, cfg.Routing.TableB)
	}
	local := prefixStrings(plan.EffectiveLocal)
	wantLocal := []string{"10.0.0.0/26", "10.0.0.128/25", "203.0.113.9/32"}
	if strings.Join(local, ",") != strings.Join(wantLocal, ",") {
		t.Fatalf("effective local = %v, want %v", local, wantLocal)
	}
	batch := plan.TargetBatch()
	for _, expected := range []string{
		"route flush table 51821",
		"10.0.0.64/26 via 192.168.10.1 dev ens224",
		"default via 192.168.10.1 dev ens224",
	} {
		if !strings.Contains(batch, expected) {
			t.Errorf("batch does not contain %q\n%s", expected, batch)
		}
	}
	if !strings.Contains(plan.Summary(), "Main routes rule: priority 10000") {
		t.Fatalf("plan summary does not show the main-routes preservation rule")
	}
	localBatch := plan.LocalBatch()
	if strings.Contains(localBatch, "route flush") || !strings.Contains(localBatch, "route replace table 51810") {
		t.Fatalf("local table must be updated in place:\n%s", localBatch)
	}
}

func TestBuildRejectsForceConflict(t *testing.T) {
	cfg := testConfig()
	cfg.Routing.ForceLocal = []model.RouteEntry{{CIDR: "10.0.0.0/24"}}
	cfg.Routing.ForceInternational = []model.RouteEntry{{CIDR: "10.0.0.1/32"}}
	if _, err := Build(cfg, []string{"10.0.0.0/8"}, 0); err == nil {
		t.Fatal("Build() accepted conflicting force routes")
	}
}

func testConfig() model.Config {
	cfg := model.DefaultConfig()
	cfg.Local = model.Link{Interface: "ens192", Address: "198.51.100.10/24", Gateway: "198.51.100.1"}
	cfg.International = model.Link{Interface: "ens224", Address: "192.168.10.11/24", Gateway: "192.168.10.1"}
	return cfg
}

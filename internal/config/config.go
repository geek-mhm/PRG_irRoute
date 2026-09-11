package config

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"irroute/internal/model"
)

func Validate(cfg model.Config) error {
	var problems []string
	if cfg.SchemaVersion != model.SchemaVersion {
		problems = append(problems, fmt.Sprintf("unsupported schema version %d", cfg.SchemaVersion))
	}
	if cfg.Mode != "host" {
		problems = append(problems, "phase one supports host mode only")
	}
	problems = append(problems, validateLink("local", cfg.Local)...)
	problems = append(problems, validateLink("international", cfg.International)...)
	if cfg.Local.Interface != "" && cfg.Local.Interface == cfg.International.Interface {
		problems = append(problems, "local and international interfaces must be different")
	}
	if cfg.Routing.DefaultRoute != "international" {
		problems = append(problems, "phase one requires international as the default route")
	}
	if cfg.Routing.FailurePolicy != "manual" {
		problems = append(problems, "phase one supports the manual failure policy only")
	}
	ids := []int{cfg.Routing.LocalTable, cfg.Routing.TableA, cfg.Routing.TableB}
	for _, id := range ids {
		if id < 1 || id > 2147483647 {
			problems = append(problems, fmt.Sprintf("routing table %d is outside the supported range", id))
		}
	}
	if cfg.Routing.LocalTable == cfg.Routing.TableA || cfg.Routing.LocalTable == cfg.Routing.TableB || cfg.Routing.TableA == cfg.Routing.TableB {
		problems = append(problems, "routing table IDs must be unique")
	}
	if cfg.Routing.MainRoutesPriority <= 0 || cfg.Routing.SourceRulePriority <= 0 || cfg.Routing.MainRulePriority <= 0 {
		problems = append(problems, "routing rule priorities must be positive")
	} else if cfg.Routing.MainRoutesPriority >= cfg.Routing.SourceRulePriority || cfg.Routing.SourceRulePriority >= cfg.Routing.MainRulePriority {
		problems = append(problems, "rule priorities must be ordered as main routes, public source, then main policy")
	}
	if cfg.Routing.MainRulePriority >= 32766 {
		problems = append(problems, "managed rule priorities must precede the system main rule at priority 32766")
	}
	for _, entry := range append(append([]model.RouteEntry{}, cfg.Routing.ForceLocal...), cfg.Routing.ForceInternational...) {
		if _, err := parseIPv4Prefix(entry.CIDR); err != nil {
			problems = append(problems, fmt.Sprintf("invalid force route %q: %v", entry.CIDR, err))
		}
	}
	if conflict := firstConflict(cfg.Routing.ForceLocal, cfg.Routing.ForceInternational); conflict != "" {
		problems = append(problems, conflict)
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func validateLink(name string, link model.Link) []string {
	var problems []string
	if strings.TrimSpace(link.Interface) == "" {
		problems = append(problems, name+" interface is required")
	}
	prefix, err := netip.ParsePrefix(link.Address)
	if err != nil || !prefix.Addr().Is4() {
		problems = append(problems, name+" address must be a valid IPv4 CIDR")
	}
	gateway, err := netip.ParseAddr(link.Gateway)
	if err != nil || !gateway.Is4() {
		problems = append(problems, name+" gateway must be a valid IPv4 address")
	} else if prefix.IsValid() && prefix.Addr().Is4() && !prefix.Contains(gateway) {
		problems = append(problems, name+" gateway must be inside the configured interface network")
	}
	return problems
}

func parseIPv4Prefix(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, "/") {
		value += "/32"
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return netip.Prefix{}, err
	}
	if !prefix.Addr().Is4() {
		return netip.Prefix{}, errors.New("only IPv4 prefixes are supported")
	}
	return prefix.Masked(), nil
}

func firstConflict(local, international []model.RouteEntry) string {
	for _, left := range local {
		lp, err := parseIPv4Prefix(left.CIDR)
		if err != nil {
			continue
		}
		for _, right := range international {
			rp, err := parseIPv4Prefix(right.CIDR)
			if err != nil {
				continue
			}
			if lp.Contains(rp.Addr()) || rp.Contains(lp.Addr()) {
				return fmt.Sprintf("force route conflict between %s and %s", lp, rp)
			}
		}
	}
	return ""
}

package routing

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"irroute/internal/config"
	"irroute/internal/model"
)

type Route struct {
	Destination netip.Prefix
	Gateway     netip.Addr
	Interface   string
	Source      netip.Addr
	Kind        string
}

type Plan struct {
	ActiveTable        int
	TargetTable        int
	LocalTable         int
	SourceRulePriority int
	MainRulePriority   int
	PublicSource       netip.Addr
	LocalRoutes        []Route
	TargetRoutes       []Route
	EffectiveLocal     []netip.Prefix
	ForceInternational []netip.Prefix
}

func Build(cfg model.Config, iranCIDRs []string, currentActiveTable int) (Plan, error) {
	if err := config.Validate(cfg); err != nil {
		return Plan{}, err
	}
	iran, err := parsePrefixes(iranCIDRs)
	if err != nil {
		return Plan{}, fmt.Errorf("Iran CIDR data: %w", err)
	}
	forceLocal, err := routeEntries(cfg.Routing.ForceLocal)
	if err != nil {
		return Plan{}, err
	}
	forceInternational, err := routeEntries(cfg.Routing.ForceInternational)
	if err != nil {
		return Plan{}, err
	}

	effectiveLocal := SubtractAll(iran, forceInternational)
	effectiveLocal = Normalize(append(effectiveLocal, forceLocal...))

	localAddress, _ := netip.ParsePrefix(cfg.Local.Address)
	internationalAddress, _ := netip.ParsePrefix(cfg.International.Address)
	localGateway, _ := netip.ParseAddr(cfg.Local.Gateway)
	internationalGateway, _ := netip.ParseAddr(cfg.International.Gateway)
	targetTable := cfg.Routing.TableA
	if currentActiveTable == cfg.Routing.TableA {
		targetTable = cfg.Routing.TableB
	}

	plan := Plan{
		ActiveTable:        currentActiveTable,
		TargetTable:        targetTable,
		LocalTable:         cfg.Routing.LocalTable,
		SourceRulePriority: cfg.Routing.SourceRulePriority,
		MainRulePriority:   cfg.Routing.MainRulePriority,
		PublicSource:       localAddress.Addr(),
		EffectiveLocal:     effectiveLocal,
		ForceInternational: Normalize(forceInternational),
	}
	plan.LocalRoutes = []Route{
		connectedRoute(localAddress, cfg.Local.Interface),
		defaultRoute(localGateway, cfg.Local.Interface, localAddress.Addr(), "local-default"),
	}
	plan.TargetRoutes = []Route{
		connectedRoute(localAddress, cfg.Local.Interface),
		connectedRoute(internationalAddress, cfg.International.Interface),
	}
	for _, prefix := range effectiveLocal {
		if prefix == localAddress.Masked() || prefix == internationalAddress.Masked() {
			continue
		}
		plan.TargetRoutes = append(plan.TargetRoutes, Route{
			Destination: prefix,
			Gateway:     localGateway,
			Interface:   cfg.Local.Interface,
			Source:      localAddress.Addr(),
			Kind:        "local",
		})
	}
	for _, prefix := range plan.ForceInternational {
		if prefix == localAddress.Masked() || prefix == internationalAddress.Masked() {
			continue
		}
		plan.TargetRoutes = append(plan.TargetRoutes, Route{
			Destination: prefix,
			Gateway:     internationalGateway,
			Interface:   cfg.International.Interface,
			Source:      internationalAddress.Addr(),
			Kind:        "force-international",
		})
	}
	plan.TargetRoutes = append(plan.TargetRoutes, defaultRoute(internationalGateway, cfg.International.Interface, internationalAddress.Addr(), "international-default"))
	return plan, nil
}

func (p Plan) LocalBatch() string {
	return routeBatch(p.LocalTable, p.LocalRoutes, false, "replace")
}

func (p Plan) TargetBatch() string {
	return routeBatch(p.TargetTable, p.TargetRoutes, true, "add")
}

func (p Plan) TargetLoadBatch() string {
	return routeBatch(p.TargetTable, p.TargetRoutes, false, "add")
}

func (p Plan) Summary() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Active table: %d\n", p.ActiveTable)
	fmt.Fprintf(&builder, "Target table: %d\n", p.TargetTable)
	fmt.Fprintf(&builder, "Local source table: %d\n", p.LocalTable)
	fmt.Fprintf(&builder, "Effective local prefixes: %d\n", len(p.EffectiveLocal))
	fmt.Fprintf(&builder, "Force-international prefixes: %d\n", len(p.ForceInternational))
	fmt.Fprintf(&builder, "Target table routes: %d\n", len(p.TargetRoutes))
	fmt.Fprintf(&builder, "Source rule: priority %d from %s/32 lookup %d\n", p.SourceRulePriority, p.PublicSource, p.LocalTable)
	fmt.Fprintf(&builder, "Main rule: priority %d lookup %d\n", p.MainRulePriority, p.TargetTable)
	return builder.String()
}

func routeBatch(table int, routes []Route, flush bool, verb string) string {
	var builder strings.Builder
	if flush {
		fmt.Fprintf(&builder, "route flush table %d\n", table)
	}
	for _, route := range routes {
		if route.Destination.Bits() == 0 {
			fmt.Fprintf(&builder, "route %s table %d default via %s dev %s src %s\n", verb, table, route.Gateway, route.Interface, route.Source)
			continue
		}
		if !route.Gateway.IsValid() {
			fmt.Fprintf(&builder, "route %s table %d %s dev %s src %s scope link\n", verb, table, route.Destination, route.Interface, route.Source)
			continue
		}
		fmt.Fprintf(&builder, "route %s table %d %s via %s dev %s src %s\n", verb, table, route.Destination, route.Gateway, route.Interface, route.Source)
	}
	return builder.String()
}

func connectedRoute(address netip.Prefix, interfaceName string) Route {
	return Route{Destination: address.Masked(), Interface: interfaceName, Source: address.Addr(), Kind: "connected"}
}

func defaultRoute(gateway netip.Addr, interfaceName string, source netip.Addr, kind string) Route {
	return Route{Destination: netip.PrefixFrom(netip.IPv4Unspecified(), 0), Gateway: gateway, Interface: interfaceName, Source: source, Kind: kind}
}

func parsePrefixes(values []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := ParseIPv4Prefix(value)
		if err != nil {
			return nil, fmt.Errorf("invalid prefix %q: %w", value, err)
		}
		prefixes = append(prefixes, prefix)
	}
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("prefix list is empty")
	}
	return Normalize(prefixes), nil
}

func routeEntries(entries []model.RouteEntry) ([]netip.Prefix, error) {
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.CIDR)
	}
	return parseOptionalPrefixes(values)
}

func parseOptionalPrefixes(values []string) ([]netip.Prefix, error) {
	if len(values) == 0 {
		return nil, nil
	}
	prefixes, err := parsePrefixes(values)
	if err != nil {
		return nil, err
	}
	sort.Slice(prefixes, func(i, j int) bool {
		if prefixes[i].Addr() != prefixes[j].Addr() {
			return prefixes[i].Addr().Less(prefixes[j].Addr())
		}
		return prefixes[i].Bits() < prefixes[j].Bits()
	})
	return prefixes, nil
}

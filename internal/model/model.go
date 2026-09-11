package model

import "time"

const SchemaVersion = 1

type Config struct {
	SchemaVersion int           `json:"schema_version"`
	Mode          string        `json:"mode"`
	Local         Link          `json:"local"`
	International Link          `json:"international"`
	Routing       RoutingConfig `json:"routing"`
	DNS           DNSConfig     `json:"dns"`
	Updates       UpdateConfig  `json:"updates"`
}

type Link struct {
	Interface string `json:"interface"`
	MAC       string `json:"mac,omitempty"`
	Address   string `json:"address"`
	Gateway   string `json:"gateway"`
}

type RoutingConfig struct {
	DefaultRoute       string       `json:"default_route"`
	FailurePolicy      string       `json:"failure_policy"`
	LocalTable         int          `json:"local_table"`
	TableA             int          `json:"table_a"`
	TableB             int          `json:"table_b"`
	MainRoutesPriority int          `json:"main_routes_rule_priority"`
	SourceRulePriority int          `json:"source_rule_priority"`
	MainRulePriority   int          `json:"main_rule_priority"`
	ForceLocal         []RouteEntry `json:"force_local,omitempty"`
	ForceInternational []RouteEntry `json:"force_international,omitempty"`
}

type RouteEntry struct {
	CIDR  string `json:"cidr"`
	Label string `json:"label,omitempty"`
}

type DNSConfig struct {
	Mode          string   `json:"mode"`
	Local         []string `json:"local,omitempty"`
	International []string `json:"international,omitempty"`
}

type UpdateConfig struct {
	Channel string `json:"channel"`
	URL     string `json:"url,omitempty"`
}

type State struct {
	SchemaVersion int       `json:"schema_version"`
	Enabled       bool      `json:"enabled"`
	ActiveTable   int       `json:"active_table,omitempty"`
	Generation    uint64    `json:"generation"`
	DataSHA256    string    `json:"data_sha256,omitempty"`
	LastAppliedAt time.Time `json:"last_applied_at,omitempty"`
	LastSnapshot  string    `json:"last_snapshot,omitempty"`
}

type Snapshot struct {
	SchemaVersion int       `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`
	Reason        string    `json:"reason"`
	Config        Config    `json:"config"`
	State         State     `json:"state"`
	IranCIDRs     []string  `json:"iran_cidrs"`
}

func DefaultConfig() Config {
	return Config{
		SchemaVersion: SchemaVersion,
		Mode:          "host",
		Routing: RoutingConfig{
			DefaultRoute:       "international",
			FailurePolicy:      "manual",
			LocalTable:         51810,
			TableA:             51820,
			TableB:             51821,
			MainRoutesPriority: 10000,
			SourceRulePriority: 10010,
			MainRulePriority:   10020,
		},
		DNS:     DNSConfig{Mode: "system"},
		Updates: UpdateConfig{Channel: "stable"},
	}
}

func NormalizeConfig(cfg Config) Config {
	if cfg.Routing.MainRoutesPriority == 0 && cfg.Routing.SourceRulePriority == 51800 && cfg.Routing.MainRulePriority == 51810 {
		cfg.Routing.MainRoutesPriority = 10000
		cfg.Routing.SourceRulePriority = 10010
		cfg.Routing.MainRulePriority = 10020
	} else if cfg.Routing.MainRoutesPriority == 0 && cfg.Routing.SourceRulePriority > 10 {
		cfg.Routing.MainRoutesPriority = cfg.Routing.SourceRulePriority - 10
	}
	return cfg
}

func DefaultState() State {
	return State{SchemaVersion: SchemaVersion}
}

// Package falkordb provides a client for FalkorDB graph database operations.
// Used for agent routing, team resolution, and relationship queries.
package falkordb

import (
	"time"
)

// Agent represents an agent in the graph database.
type Agent struct {
	Name           string            `json:"name"`
	Role           string            `json:"role"`
	Description    string            `json:"description,omitempty"`
	Focus          string            `json:"focus,omitempty"`
	Type           AgentType         `json:"type"`
	DefinitionPath string            `json:"definition_path,omitempty"`
	OperationsDir  string            `json:"operations_dir,omitempty"`
	Capabilities   []string          `json:"capabilities,omitempty"`
	TaskTypes      []string          `json:"task_types,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// AgentType categorizes agents by their function.
type AgentType string

const (
	AgentTypeCoordinator AgentType = "coordinator"
	AgentTypeSpecialist  AgentType = "specialist"
	AgentTypeWorker      AgentType = "worker"
	AgentTypeLead        AgentType = "lead"
	AgentTypePM          AgentType = "pm"
)

// Team represents a team of agents.
type Team struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Focus       string   `json:"focus,omitempty"`
	Leader      string   `json:"leader,omitempty"`
	PM          string   `json:"pm,omitempty"`
	Members     []string `json:"members,omitempty"`
}

// Route represents a routing decision from one agent to another.
type Route struct {
	Source    string   `json:"source"`
	Target    string   `json:"target"`
	TaskType  string   `json:"task_type"`
	Priority  int      `json:"priority,omitempty"`
	Chain     []string `json:"chain,omitempty"` // Reporting chain
	TeamName  string   `json:"team_name,omitempty"`
}

// RouteQuery specifies routing lookup parameters.
type RouteQuery struct {
	TaskType     string   `json:"task_type"`
	FromAgent    string   `json:"from_agent,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	TeamFilter   string   `json:"team_filter,omitempty"`
}

// RouteResult contains the result of a routing query.
type RouteResult struct {
	Agent          Agent    `json:"agent"`
	Route          Route    `json:"route"`
	Team           *Team    `json:"team,omitempty"`
	AlternateRoute []Route  `json:"alternate_routes,omitempty"`
}

// Relationship represents a relationship between agents.
type Relationship struct {
	Type       RelationType `json:"type"`
	FromAgent  string       `json:"from_agent"`
	ToAgent    string       `json:"to_agent"`
	Properties map[string]any `json:"properties,omitempty"`
}

// RelationType defines the type of relationship between agents.
type RelationType string

const (
	RelRoutesTo   RelationType = "ROUTES_TO"
	RelReportsTo  RelationType = "REPORTS_TO"
	RelMemberOf   RelationType = "MEMBER_OF"
	RelLeads      RelationType = "LEADS"
	RelManages    RelationType = "MANAGES"
	RelDelegates  RelationType = "DELEGATES_TO"
	RelCollabWith RelationType = "COLLABORATES_WITH"
)

// GraphStats contains statistics about the agent graph.
type GraphStats struct {
	AgentCount       int       `json:"agent_count"`
	TeamCount        int       `json:"team_count"`
	RelationshipCount int      `json:"relationship_count"`
	LastUpdated      time.Time `json:"last_updated"`
}

// CacheEntry represents a cached routing entry.
type CacheEntry struct {
	Route     *RouteResult `json:"route"`
	CachedAt  time.Time    `json:"cached_at"`
	ExpiresAt time.Time    `json:"expires_at"`
	HitCount  int          `json:"hit_count"`
}

// FallbackRoutes provides static routing when FalkorDB is unavailable.
var FallbackRoutes = map[string]string{
	"budget":               "BARAKA",
	"debt":                 "BARAKA",
	"savings":              "BARAKA",
	"investments":          "BARAKA",
	"shariah_compliance":   "BARAKA",
	"security_assessment":  "PALADIN",
	"hardening":            "HRDN",
	"vulnerability_remediation": "HRDN",
	"threat_hunting":       "DART",
	"pentesting":           "GHST",
	"security_coordination": "NEXUS",
	"deep_research":        "Axiom",
	"investigation":        "Axiom",
	"analysis":             "Proof",
	"executive_communication": "SIGNAL",
	"email_correspondence": "SIGNAL",
	"brand_strategy":       "BRND",
	"linkedin_profile":     "BRND",
	"resume":               "BRND",
	"code_development":     "FOUNDRY",
	"build":                "FOUNDRY",
	"deploy":               "FOUNDRY",
	"test":                 "FOUNDRY",
	"database_dashboard":   "Hive",
	"kubernetes_deployment": "Kuber",
	"network_config":       "Netty",
	"team_creation":        "AGNT",
	"agent_design":         "AGNT",
	"agent_fix":            "AGNT",
	"agent_feedback":       "Dispatch",
}

// FallbackTeams provides static team mapping when FalkorDB is unavailable.
var FallbackTeams = map[string]Team{
	"BARAKA": {
		Name:        "BARAKA",
		Description: "Finance and Shariah compliance",
		Leader:      "AMIL",
		PM:          "WAKIL",
	},
	"CRBRS": {
		Name:        "CRBRS",
		Description: "Security operations",
		Leader:      "PALADIN",
		PM:          "NEXUS",
	},
	"Axiom": {
		Name:        "Axiom",
		Description: "Research and verification",
		Leader:      "Thesis",
		PM:          "Proof",
	},
	"SIGNAL": {
		Name:        "SIGNAL",
		Description: "Communications",
		Leader:      "Chorus",
		PM:          "Pulse",
	},
	"BRND": {
		Name:        "BRND",
		Description: "Personal brand",
		Leader:      "Marq",
		PM:          "Herald",
	},
	"FOUNDRY": {
		Name:        "FOUNDRY",
		Description: "Full development lifecycle",
		Leader:      "ANVIL",
		PM:          "Spark",
	},
	"AGNT": {
		Name:        "AGNT",
		Description: "Agent architecture",
		Leader:      "APEX",
		PM:          "Dispatch",
	},
}

// QueryResult represents a raw query result from FalkorDB.
type QueryResult struct {
	Columns []string         `json:"columns"`
	Rows    [][]any          `json:"rows"`
	Stats   map[string]any   `json:"stats,omitempty"`
}

package falkordb

import (
	"context"
	"fmt"
)

// GetTeam retrieves a team by name.
func (c *Client) GetTeam(ctx context.Context, name string) (*Team, error) {
	// Try cache/fallback first if not connected
	if !c.config.Enabled || !c.IsConnected() {
		if team, ok := FallbackTeams[name]; ok {
			return &team, nil
		}
		return nil, fmt.Errorf("team not found: %s", name)
	}

	cypher := `
		MATCH (t:Team {name: $name})
		OPTIONAL MATCH (leader:Agent)-[:LEADS]->(t)
		OPTIONAL MATCH (pm:Agent)-[:MANAGES]->(t)
		OPTIONAL MATCH (member:Agent)-[:MEMBER_OF]->(t)
		RETURN t.name, t.description, t.focus,
		       leader.name AS leader,
		       pm.name AS pm,
		       collect(DISTINCT member.name) AS members
	`

	result, err := c.Query(ctx, cypher, map[string]any{"name": name})
	if err != nil {
		// Try fallback
		if team, ok := FallbackTeams[name]; ok {
			return &team, nil
		}
		return nil, err
	}

	if len(result.Rows) == 0 {
		// Try fallback
		if team, ok := FallbackTeams[name]; ok {
			return &team, nil
		}
		return nil, fmt.Errorf("team not found: %s", name)
	}

	row := result.Rows[0]
	return &Team{
		Name:        getString(row, 0),
		Description: getString(row, 1),
		Focus:       getString(row, 2),
		Leader:      getString(row, 3),
		PM:          getString(row, 4),
		Members:     getStringSlice(row, 5),
	}, nil
}

// ListTeams retrieves all teams.
func (c *Client) ListTeams(ctx context.Context) ([]Team, error) {
	if !c.config.Enabled || !c.IsConnected() {
		var teams []Team
		for _, team := range FallbackTeams {
			teams = append(teams, team)
		}
		return teams, nil
	}

	cypher := `
		MATCH (t:Team)
		OPTIONAL MATCH (leader:Agent)-[:LEADS]->(t)
		OPTIONAL MATCH (pm:Agent)-[:MANAGES]->(t)
		RETURN t.name, t.description, t.focus,
		       leader.name AS leader,
		       pm.name AS pm
		ORDER BY t.name
	`

	result, err := c.Query(ctx, cypher, nil)
	if err != nil {
		return nil, err
	}

	var teams []Team
	for _, row := range result.Rows {
		teams = append(teams, Team{
			Name:        getString(row, 0),
			Description: getString(row, 1),
			Focus:       getString(row, 2),
			Leader:      getString(row, 3),
			PM:          getString(row, 4),
		})
	}

	return teams, nil
}

// GetTeamMembers returns all members of a team.
func (c *Client) GetTeamMembers(ctx context.Context, teamName string) ([]Agent, error) {
	if !c.config.Enabled || !c.IsConnected() {
		return nil, fmt.Errorf("FalkorDB not connected")
	}

	cypher := `
		MATCH (a:Agent)-[:MEMBER_OF]->(t:Team {name: $team})
		RETURN a.name, a.role, a.description, a.focus, a.type,
		       a.definition_path, a.operations_dir
		ORDER BY a.name
	`

	result, err := c.Query(ctx, cypher, map[string]any{"team": teamName})
	if err != nil {
		return nil, err
	}

	var agents []Agent
	for _, row := range result.Rows {
		agents = append(agents, Agent{
			Name:           getString(row, 0),
			Role:           getString(row, 1),
			Description:    getString(row, 2),
			Focus:          getString(row, 3),
			Type:           AgentType(getString(row, 4)),
			DefinitionPath: getString(row, 5),
			OperationsDir:  getString(row, 6),
		})
	}

	return agents, nil
}

// GetTeamForAgent returns the team that an agent belongs to.
func (c *Client) GetTeamForAgent(ctx context.Context, agentName string) (*Team, error) {
	if !c.config.Enabled || !c.IsConnected() {
		// Check fallback teams
		for teamName, team := range FallbackTeams {
			if team.Leader == agentName || team.PM == agentName {
				t := team
				t.Name = teamName
				return &t, nil
			}
		}
		return nil, fmt.Errorf("no team found for agent: %s", agentName)
	}

	cypher := `
		MATCH (a:Agent {name: $name})-[:MEMBER_OF|LEADS|MANAGES]->(t:Team)
		OPTIONAL MATCH (leader:Agent)-[:LEADS]->(t)
		OPTIONAL MATCH (pm:Agent)-[:MANAGES]->(t)
		RETURN t.name, t.description, t.focus,
		       leader.name AS leader,
		       pm.name AS pm
		LIMIT 1
	`

	result, err := c.Query(ctx, cypher, map[string]any{"name": agentName})
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("no team found for agent: %s", agentName)
	}

	row := result.Rows[0]
	return &Team{
		Name:        getString(row, 0),
		Description: getString(row, 1),
		Focus:       getString(row, 2),
		Leader:      getString(row, 3),
		PM:          getString(row, 4),
	}, nil
}

// InvokeTeam returns all information needed to invoke an entire team.
func (c *Client) InvokeTeam(ctx context.Context, teamName string) (*TeamInvocation, error) {
	team, err := c.GetTeam(ctx, teamName)
	if err != nil {
		return nil, err
	}

	members, err := c.GetTeamMembers(ctx, teamName)
	if err != nil {
		// Return with just leader/PM info
		return &TeamInvocation{
			Team:    *team,
			Members: nil,
		}, nil
	}

	return &TeamInvocation{
		Team:    *team,
		Members: members,
	}, nil
}

// TeamInvocation contains all information needed to invoke a team.
type TeamInvocation struct {
	Team    Team    `json:"team"`
	Members []Agent `json:"members"`
}

// GetReportingChain returns the reporting chain for an agent.
func (c *Client) GetReportingChain(ctx context.Context, agentName string) ([]Agent, error) {
	if !c.config.Enabled || !c.IsConnected() {
		return nil, fmt.Errorf("FalkorDB not connected")
	}

	cypher := `
		MATCH path = (a:Agent {name: $name})-[:REPORTS_TO*]->(top:Agent)
		WHERE NOT (top)-[:REPORTS_TO]->()
		WITH nodes(path) AS chain
		UNWIND chain AS agent
		RETURN DISTINCT agent.name, agent.role, agent.type
	`

	result, err := c.Query(ctx, cypher, map[string]any{"name": agentName})
	if err != nil {
		return nil, err
	}

	var chain []Agent
	for _, row := range result.Rows {
		chain = append(chain, Agent{
			Name: getString(row, 0),
			Role: getString(row, 1),
			Type: AgentType(getString(row, 2)),
		})
	}

	return chain, nil
}

// TeamApprovalChain returns the approval chain for a team operation.
// This is used for operations that require team lead/PM approval.
func (c *Client) TeamApprovalChain(ctx context.Context, teamName string) ([]string, error) {
	team, err := c.GetTeam(ctx, teamName)
	if err != nil {
		return nil, err
	}

	// Standard approval chain: PM first, then Lead
	var chain []string
	if team.PM != "" {
		chain = append(chain, team.PM)
	}
	if team.Leader != "" && team.Leader != team.PM {
		chain = append(chain, team.Leader)
	}

	return chain, nil
}

// SpecialRules returns any special rules for a team member.
// For example, HALAL has veto authority on investments.
func (c *Client) SpecialRules(ctx context.Context, agentName string) ([]string, error) {
	if !c.config.Enabled || !c.IsConnected() {
		// Hardcoded special rules
		switch agentName {
		case "HALAL":
			return []string{"veto_authority:investments"}, nil
		case "Polish":
			return []string{"must_approve:communications"}, nil
		}
		return nil, nil
	}

	cypher := `
		MATCH (a:Agent {name: $name})
		RETURN a.special_rules
	`

	result, err := c.Query(ctx, cypher, map[string]any{"name": agentName})
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 {
		return nil, nil
	}

	return getStringSlice(result.Rows[0], 0), nil
}

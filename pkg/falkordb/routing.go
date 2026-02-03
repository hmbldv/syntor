package falkordb

import (
	"context"
	"fmt"
	"strings"
)

// RouteTask finds the best agent to handle a task type.
// This is the primary routing function per the CLAUDE.md specification.
func (c *Client) RouteTask(ctx context.Context, query RouteQuery) (*RouteResult, error) {
	// Check cache first
	cacheKey := getCacheKey(query)
	if cached, ok := c.getFromCache(cacheKey); ok {
		return cached, nil
	}

	// If not connected, use fallback
	if !c.config.Enabled || !c.IsConnected() {
		return c.routeWithFallback(query)
	}

	// Execute the routing query from CLAUDE.md
	cypher := `
		MATCH (sage:Agent {name: 'Sage'})-[r:ROUTES_TO]->(target:Agent)
		WHERE r.task_type = $task_type
		OPTIONAL MATCH (target)-[:REPORTS_TO*1..3]->(chain:Agent)
		OPTIONAL MATCH (target)-[:MEMBER_OF]->(team:Team)
		RETURN target.name AS agent, target.role AS role, target.focus AS focus,
		       team.name AS team, collect(DISTINCT chain.name) AS chain,
		       target.definition_path AS definition_path, target.operations_dir AS operations_dir
	`

	result, err := c.Query(ctx, cypher, map[string]any{
		"task_type": query.TaskType,
	})
	if err != nil {
		// Fall back to static routing on error
		return c.routeWithFallback(query)
	}

	// Parse the result
	if len(result.Rows) == 0 {
		// No route found, try fallback
		return c.routeWithFallback(query)
	}

	row := result.Rows[0]
	routeResult := &RouteResult{
		Agent: Agent{
			Name: getString(row, 0),
			Role: getString(row, 1),
			Focus: getString(row, 2),
			DefinitionPath: getString(row, 5),
			OperationsDir: getString(row, 6),
		},
		Route: Route{
			Source:   "Sage",
			Target:   getString(row, 0),
			TaskType: query.TaskType,
			TeamName: getString(row, 3),
			Chain:    getStringSlice(row, 4),
		},
	}

	// Get team details if available
	if teamName := getString(row, 3); teamName != "" {
		team, _ := c.GetTeam(ctx, teamName)
		routeResult.Team = team
	}

	// Cache the result
	c.setCache(cacheKey, routeResult)

	return routeResult, nil
}

// routeWithFallback uses the static fallback routing table.
func (c *Client) routeWithFallback(query RouteQuery) (*RouteResult, error) {
	agentName, ok := FallbackRoutes[query.TaskType]
	if !ok {
		// Try partial match
		for taskType, agent := range FallbackRoutes {
			if strings.Contains(query.TaskType, taskType) || strings.Contains(taskType, query.TaskType) {
				agentName = agent
				ok = true
				break
			}
		}
	}

	if !ok {
		return nil, fmt.Errorf("no route found for task type: %s", query.TaskType)
	}

	result := &RouteResult{
		Agent: Agent{
			Name: agentName,
		},
		Route: Route{
			Source:   "Sage",
			Target:   agentName,
			TaskType: query.TaskType,
		},
	}

	// Try to get team info from fallback
	for teamName, team := range FallbackTeams {
		if team.Leader == agentName || team.PM == agentName {
			result.Team = &Team{
				Name:        teamName,
				Description: team.Description,
				Leader:      team.Leader,
				PM:          team.PM,
			}
			result.Route.TeamName = teamName
			break
		}
	}

	return result, nil
}

// GetAgent retrieves an agent by name.
func (c *Client) GetAgent(ctx context.Context, name string) (*Agent, error) {
	if !c.config.Enabled || !c.IsConnected() {
		return nil, fmt.Errorf("FalkorDB not connected")
	}

	cypher := `
		MATCH (a:Agent {name: $name})
		RETURN a.name, a.role, a.description, a.focus, a.type,
		       a.definition_path, a.operations_dir, a.capabilities, a.task_types,
		       a.ollama_model, a.model_category
	`

	result, err := c.Query(ctx, cypher, map[string]any{"name": name})
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("agent not found: %s", name)
	}

	row := result.Rows[0]
	return &Agent{
		Name:           getString(row, 0),
		Role:           getString(row, 1),
		Description:    getString(row, 2),
		Focus:          getString(row, 3),
		Type:           AgentType(getString(row, 4)),
		DefinitionPath: getString(row, 5),
		OperationsDir:  getString(row, 6),
		Capabilities:   getStringSlice(row, 7),
		TaskTypes:      getStringSlice(row, 8),
		OllamaModel:    getString(row, 9),
		ModelCategory:  getString(row, 10),
	}, nil
}

// GetAgentModel retrieves just the Ollama model for an agent.
// This is optimized for model selection during handoffs.
func (c *Client) GetAgentModel(ctx context.Context, agentName string) (string, error) {
	if !c.config.Enabled || !c.IsConnected() {
		return "", fmt.Errorf("FalkorDB not connected")
	}

	cypher := `MATCH (a:Agent {name: $name}) RETURN a.ollama_model`

	result, err := c.Query(ctx, cypher, map[string]any{"name": agentName})
	if err != nil {
		return "", err
	}

	if len(result.Rows) == 0 {
		return "", fmt.Errorf("agent not found: %s", agentName)
	}

	model := getString(result.Rows[0], 0)
	if model == "" {
		return "", fmt.Errorf("no model configured for agent: %s", agentName)
	}

	return model, nil
}

// ListAgents retrieves all agents, optionally filtered by type.
func (c *Client) ListAgents(ctx context.Context, agentType AgentType) ([]Agent, error) {
	if !c.config.Enabled || !c.IsConnected() {
		return nil, fmt.Errorf("FalkorDB not connected")
	}

	var cypher string
	params := map[string]any{}

	if agentType != "" {
		cypher = `
			MATCH (a:Agent {type: $type})
			RETURN a.name, a.role, a.description, a.focus, a.type,
			       a.definition_path, a.operations_dir
			ORDER BY a.name
		`
		params["type"] = string(agentType)
	} else {
		cypher = `
			MATCH (a:Agent)
			RETURN a.name, a.role, a.description, a.focus, a.type,
			       a.definition_path, a.operations_dir
			ORDER BY a.name
		`
	}

	result, err := c.Query(ctx, cypher, params)
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

// FindAgentsByCapability finds agents with a specific capability.
func (c *Client) FindAgentsByCapability(ctx context.Context, capability string) ([]Agent, error) {
	if !c.config.Enabled || !c.IsConnected() {
		return nil, fmt.Errorf("FalkorDB not connected")
	}

	cypher := `
		MATCH (a:Agent)
		WHERE $capability IN a.capabilities
		RETURN a.name, a.role, a.description, a.focus, a.type,
		       a.definition_path, a.operations_dir, a.capabilities
		ORDER BY a.name
	`

	result, err := c.Query(ctx, cypher, map[string]any{"capability": capability})
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
			Capabilities:   getStringSlice(row, 7),
		})
	}

	return agents, nil
}

// GetRoutingChain returns the full routing chain from source to target.
func (c *Client) GetRoutingChain(ctx context.Context, source, target string) ([]string, error) {
	if !c.config.Enabled || !c.IsConnected() {
		return []string{source, target}, nil
	}

	cypher := `
		MATCH path = shortestPath((s:Agent {name: $source})-[:ROUTES_TO|REPORTS_TO*]->(t:Agent {name: $target}))
		RETURN [node IN nodes(path) | node.name] AS chain
	`

	result, err := c.Query(ctx, cypher, map[string]any{
		"source": source,
		"target": target,
	})
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("no path found from %s to %s", source, target)
	}

	return getStringSlice(result.Rows[0], 0), nil
}

// GetRelationships returns all relationships for an agent.
func (c *Client) GetRelationships(ctx context.Context, agentName string) ([]Relationship, error) {
	if !c.config.Enabled || !c.IsConnected() {
		return nil, fmt.Errorf("FalkorDB not connected")
	}

	cypher := `
		MATCH (a:Agent {name: $name})-[r]->(b)
		RETURN type(r) AS type, a.name AS from, b.name AS to, properties(r) AS props
		UNION
		MATCH (a:Agent {name: $name})<-[r]-(b)
		RETURN type(r) AS type, b.name AS from, a.name AS to, properties(r) AS props
	`

	result, err := c.Query(ctx, cypher, map[string]any{"name": agentName})
	if err != nil {
		return nil, err
	}

	var relationships []Relationship
	for _, row := range result.Rows {
		rel := Relationship{
			Type:      RelationType(getString(row, 0)),
			FromAgent: getString(row, 1),
			ToAgent:   getString(row, 2),
		}
		if len(row) > 3 {
			if props, ok := row[3].(map[string]any); ok {
				rel.Properties = props
			}
		}
		relationships = append(relationships, rel)
	}

	return relationships, nil
}

// Helper functions for result parsing

func getString(row []any, idx int) string {
	if idx >= len(row) {
		return ""
	}
	if s, ok := row[idx].(string); ok {
		return s
	}
	return ""
}

func getStringSlice(row []any, idx int) []string {
	if idx >= len(row) {
		return nil
	}
	if arr, ok := row[idx].([]any); ok {
		var result []string
		for _, v := range arr {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

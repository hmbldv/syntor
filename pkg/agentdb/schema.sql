-- Agent Database Schema for SYNTOR
-- Database: HIVE (192.168.1.61:5433)
-- Schema: agents

-- Create schema
CREATE SCHEMA IF NOT EXISTS agents;

-- Agent definitions table with versioning
CREATE TABLE IF NOT EXISTS agents.definitions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        VARCHAR(50) NOT NULL,
    version         INTEGER NOT NULL DEFAULT 1,
    is_current      BOOLEAN NOT NULL DEFAULT true,

    -- Identity
    name            VARCHAR(100) NOT NULL,
    role            TEXT,
    team            VARCHAR(50),

    -- Rich Content (stored as JSONB for flexibility)
    system_prompt   TEXT NOT NULL,           -- Full 200+ line prompt
    personality     JSONB,                    -- tone, style, demeanor, phrases
    expertise       JSONB,                    -- domain → level mapping
    interaction_protocols JSONB,              -- per-agent interaction rules
    decision_framework    JSONB,              -- decision-making process
    behavioral_rules      JSONB,              -- do/don't rules

    -- Operational (stored as JSONB arrays)
    capabilities    JSONB DEFAULT '[]'::jsonb,
    task_types      JSONB DEFAULT '[]'::jsonb,
    model_config    JSONB,

    -- Metadata
    created_at      TIMESTAMPTZ DEFAULT now(),
    updated_at      TIMESTAMPTZ DEFAULT now(),

    -- Ensure unique agent_id + version combination
    CONSTRAINT unique_agent_version UNIQUE (agent_id, version)
);

-- Index for fast lookups
CREATE INDEX IF NOT EXISTS idx_definitions_agent_id ON agents.definitions(agent_id);
CREATE INDEX IF NOT EXISTS idx_definitions_current ON agents.definitions(is_current) WHERE is_current = true;
CREATE INDEX IF NOT EXISTS idx_definitions_team ON agents.definitions(team);

-- GIN index for JSONB array searches
CREATE INDEX IF NOT EXISTS idx_definitions_capabilities ON agents.definitions USING GIN (capabilities);
CREATE INDEX IF NOT EXISTS idx_definitions_task_types ON agents.definitions USING GIN (task_types);

-- Version history table for tracking changes
CREATE TABLE IF NOT EXISTS agents.definition_history (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    definition_id   UUID REFERENCES agents.definitions(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL,
    changed_fields  JSONB,                    -- Which fields changed
    changed_at      TIMESTAMPTZ DEFAULT now(),
    changed_by      VARCHAR(100)              -- Who made the change
);

CREATE INDEX IF NOT EXISTS idx_history_definition ON agents.definition_history(definition_id);
CREATE INDEX IF NOT EXISTS idx_history_changed_at ON agents.definition_history(changed_at DESC);

-- View for current definitions only
CREATE OR REPLACE VIEW agents.v_current_definitions AS
SELECT
    id, agent_id, version, name, role, team,
    system_prompt, personality, expertise,
    interaction_protocols, decision_framework, behavioral_rules,
    capabilities, task_types, model_config,
    created_at, updated_at
FROM agents.definitions
WHERE is_current = true;

-- Function to automatically log changes
CREATE OR REPLACE FUNCTION agents.log_definition_change()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        INSERT INTO agents.definition_history (
            definition_id, version, changed_fields, changed_by
        ) VALUES (
            OLD.id,
            OLD.version,
            jsonb_build_object(
                'name', CASE WHEN OLD.name != NEW.name THEN jsonb_build_object('old', OLD.name, 'new', NEW.name) END,
                'role', CASE WHEN OLD.role != NEW.role THEN jsonb_build_object('old', OLD.role, 'new', NEW.role) END,
                'team', CASE WHEN OLD.team != NEW.team THEN jsonb_build_object('old', OLD.team, 'new', NEW.team) END,
                'system_prompt', CASE WHEN OLD.system_prompt != NEW.system_prompt THEN jsonb_build_object('changed', true) END,
                'personality', CASE WHEN OLD.personality != NEW.personality THEN jsonb_build_object('changed', true) END
            ) - 'null',
            current_user
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for change logging
DROP TRIGGER IF EXISTS tr_log_definition_change ON agents.definitions;
CREATE TRIGGER tr_log_definition_change
    AFTER UPDATE ON agents.definitions
    FOR EACH ROW
    EXECUTE FUNCTION agents.log_definition_change();

-- Function to ensure only one current version per agent
CREATE OR REPLACE FUNCTION agents.ensure_single_current()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.is_current = true THEN
        UPDATE agents.definitions
        SET is_current = false, updated_at = now()
        WHERE agent_id = NEW.agent_id
        AND is_current = true
        AND id != NEW.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for single current version
DROP TRIGGER IF EXISTS tr_ensure_single_current ON agents.definitions;
CREATE TRIGGER tr_ensure_single_current
    BEFORE INSERT OR UPDATE ON agents.definitions
    FOR EACH ROW
    EXECUTE FUNCTION agents.ensure_single_current();

-- Sample data for SNTR agent
INSERT INTO agents.definitions (
    agent_id, name, role, team, system_prompt, personality, capabilities, task_types
) VALUES (
    'sntr',
    'SNTR',
    'Primary Orchestrator',
    'Core',
    E'## Identity\nYou are SNTR (pronounced ''center''), the primary AI orchestration agent for SYNTOR.\n\n## Your Voice\n- **Tone**: Helpful, competent, and direct\n- **Style**: Concise but thorough\n\n## Your Responsibilities\n- Coordinating multi-agent workflows\n- Executing filesystem operations using tools\n- Understanding user intent and routing to specialists',
    '{
        "tone": "Helpful, competent, and direct",
        "style": "Concise but thorough",
        "demeanor": "Professional assistant who takes initiative",
        "phrases": ["Let me check that for you...", "I''ll execute that now.", "Here''s what I found:"],
        "avoid": ["I cannot access your filesystem", "As an AI, I don''t have access to..."]
    }'::jsonb,
    '["tool-execution", "task-routing", "planning", "orchestration"]'::jsonb,
    '["code_development", "file_operations", "general_assistance"]'::jsonb
) ON CONFLICT (agent_id, version) DO NOTHING;

-- Grant permissions (adjust as needed)
-- GRANT USAGE ON SCHEMA agents TO syntor_app;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA agents TO syntor_app;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA agents TO syntor_app;

COMMENT ON SCHEMA agents IS 'Agent definitions and configuration for SYNTOR';
COMMENT ON TABLE agents.definitions IS 'Rich agent definitions with versioning support';
COMMENT ON TABLE agents.definition_history IS 'Audit log of changes to agent definitions';
COMMENT ON VIEW agents.v_current_definitions IS 'Current (active) version of each agent definition';

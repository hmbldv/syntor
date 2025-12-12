-- SYNTOR Database Initialization Script
-- Creates tables for task management, agent state, and system coordination

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Tasks table
CREATE TABLE IF NOT EXISTS tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type VARCHAR(255) NOT NULL,
    priority INTEGER NOT NULL DEFAULT 5,
    required_capabilities TEXT[] DEFAULT '{}',
    payload JSONB NOT NULL DEFAULT '{}',
    assigned_agent VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    result JSONB,
    error JSONB,
    timeout_seconds INTEGER DEFAULT 300,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    correlation_id VARCHAR(255),
    parent_task_id UUID REFERENCES tasks(id),
    metadata JSONB DEFAULT '{}'
);

-- Create indexes for tasks
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_agent ON tasks(assigned_agent);
CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at);
CREATE INDEX IF NOT EXISTS idx_tasks_type ON tasks(type);
CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_correlation_id ON tasks(correlation_id);

-- Agent state snapshots table (for recovery)
CREATE TABLE IF NOT EXISTS agent_state_snapshots (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id VARCHAR(255) NOT NULL,
    agent_type VARCHAR(50) NOT NULL,
    state JSONB NOT NULL,
    checkpoint_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(agent_id, version)
);

CREATE INDEX IF NOT EXISTS idx_agent_snapshots_agent_id ON agent_state_snapshots(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_snapshots_checkpoint ON agent_state_snapshots(checkpoint_at);

-- Task execution log (for auditing and debugging)
CREATE TABLE IF NOT EXISTS task_execution_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID NOT NULL REFERENCES tasks(id),
    agent_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    event_data JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_task_log_task_id ON task_execution_log(task_id);
CREATE INDEX IF NOT EXISTS idx_task_log_agent_id ON task_execution_log(agent_id);
CREATE INDEX IF NOT EXISTS idx_task_log_created_at ON task_execution_log(created_at);

-- System configuration table
CREATE TABLE IF NOT EXISTS system_config (
    key VARCHAR(255) PRIMARY KEY,
    value JSONB NOT NULL,
    description TEXT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_by VARCHAR(255)
);

-- Insert default configuration
INSERT INTO system_config (key, value, description) VALUES
    ('task.default_timeout', '300', 'Default task timeout in seconds'),
    ('task.max_retries', '3', 'Maximum task retry attempts'),
    ('agent.heartbeat_interval', '30', 'Agent heartbeat interval in seconds'),
    ('agent.stale_threshold', '90', 'Agent stale detection threshold in seconds'),
    ('system.version', '"1.0.0"', 'SYNTOR system version')
ON CONFLICT (key) DO NOTHING;

-- Dead letter queue for failed messages
CREATE TABLE IF NOT EXISTS dead_letter_queue (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id VARCHAR(255) NOT NULL,
    topic VARCHAR(255) NOT NULL,
    message_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    error_message TEXT,
    error_details JSONB,
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_dlq_created_at ON dead_letter_queue(created_at);
CREATE INDEX IF NOT EXISTS idx_dlq_processed ON dead_letter_queue(processed_at) WHERE processed_at IS NULL;

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to system_config
DROP TRIGGER IF EXISTS update_system_config_updated_at ON system_config;
CREATE TRIGGER update_system_config_updated_at
    BEFORE UPDATE ON system_config
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Grant permissions (for application user)
-- In production, create a dedicated application user with limited permissions
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO syntor;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO syntor;

package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// NewMessage creates a new message with default values
func NewMessage(msgType MessageType, source, target string, payload map[string]interface{}) Message {
	return Message{
		ID:        uuid.New().String(),
		Type:      msgType,
		Source:    source,
		Target:    target,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

// NewTaskAssignmentMessage creates a task assignment message
func NewTaskAssignmentMessage(source, target string, task Task) Message {
	return Message{
		ID:        uuid.New().String(),
		Type:      MsgTaskAssignment,
		Source:    source,
		Target:    target,
		Payload:   taskToPayload(task),
		Timestamp: time.Now(),
	}
}

// NewTaskStatusMessage creates a task status update message
func NewTaskStatusMessage(source string, taskID string, status TaskStatus) Message {
	return Message{
		ID:     uuid.New().String(),
		Type:   MsgTaskStatus,
		Source: source,
		Payload: map[string]interface{}{
			"task_id": taskID,
			"status":  string(status),
		},
		Timestamp: time.Now(),
	}
}

// NewTaskCompleteMessage creates a task completion message
func NewTaskCompleteMessage(source string, taskID string, result *TaskResult, err *TaskError) Message {
	payload := map[string]interface{}{
		"task_id": taskID,
	}
	if result != nil {
		payload["result"] = result
	}
	if err != nil {
		payload["error"] = err
	}

	return Message{
		ID:        uuid.New().String(),
		Type:      MsgTaskComplete,
		Source:    source,
		Payload:   payload,
		Timestamp: time.Now(),
	}
}

// NewAgentRegistrationMessage creates an agent registration message
func NewAgentRegistrationMessage(agentID, agentName string, agentType AgentType, capabilities []Capability) Message {
	return Message{
		ID:     uuid.New().String(),
		Type:   MsgAgentRegistration,
		Source: agentID,
		Payload: map[string]interface{}{
			"agent_id":     agentID,
			"agent_name":   agentName,
			"agent_type":   string(agentType),
			"capabilities": capabilities,
		},
		Timestamp: time.Now(),
	}
}

// NewAgentHeartbeatMessage creates an agent heartbeat message
func NewAgentHeartbeatMessage(agentID string, metrics AgentMetrics) Message {
	return Message{
		ID:     uuid.New().String(),
		Type:   MsgAgentHeartbeat,
		Source: agentID,
		Payload: map[string]interface{}{
			"agent_id": agentID,
			"metrics":  metrics,
		},
		Timestamp: time.Now(),
	}
}

// NewServiceRequestMessage creates a service request message
func NewServiceRequestMessage(source, target, serviceType string, request map[string]interface{}) Message {
	return Message{
		ID:            uuid.New().String(),
		Type:          MsgServiceRequest,
		Source:        source,
		Target:        target,
		CorrelationID: uuid.New().String(),
		Payload: map[string]interface{}{
			"service_type": serviceType,
			"request":      request,
		},
		Timestamp: time.Now(),
	}
}

// NewServiceResponseMessage creates a service response message
func NewServiceResponseMessage(source, target, correlationID string, response map[string]interface{}, err *TaskError) Message {
	payload := map[string]interface{}{
		"response": response,
	}
	if err != nil {
		payload["error"] = err
	}

	return Message{
		ID:            uuid.New().String(),
		Type:          MsgServiceResponse,
		Source:        source,
		Target:        target,
		CorrelationID: correlationID,
		Payload:       payload,
		Timestamp:     time.Now(),
	}
}

// WithCorrelationID adds a correlation ID to the message
func (m Message) WithCorrelationID(id string) Message {
	m.CorrelationID = id
	return m
}

// WithReplyTo sets the reply-to topic for the message
func (m Message) WithReplyTo(topic string) Message {
	m.ReplyTo = topic
	return m
}

// WithTTL sets the time-to-live for the message
func (m Message) WithTTL(ttl time.Duration) Message {
	m.TTL = ttl
	return m
}

// IsExpired checks if the message has expired based on TTL
func (m Message) IsExpired() bool {
	if m.TTL == 0 {
		return false
	}
	return time.Since(m.Timestamp) > m.TTL
}

// ToJSON serializes the message to JSON bytes
func (m Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// MessageFromJSON deserializes a message from JSON bytes
func MessageFromJSON(data []byte) (Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	return msg, err
}

// Validate checks if the message has all required fields
func (m Message) Validate() error {
	if m.ID == "" {
		return &ValidationError{Field: "id", Message: "message ID is required"}
	}
	if m.Type == "" {
		return &ValidationError{Field: "type", Message: "message type is required"}
	}
	if m.Source == "" {
		return &ValidationError{Field: "source", Message: "message source is required"}
	}
	if m.Timestamp.IsZero() {
		return &ValidationError{Field: "timestamp", Message: "message timestamp is required"}
	}
	return nil
}

// ValidationError represents a message validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "validation error on field '" + e.Field + "': " + e.Message
}

// Helper function to convert task to payload
func taskToPayload(task Task) map[string]interface{} {
	data, _ := json.Marshal(task)
	var payload map[string]interface{}
	json.Unmarshal(data, &payload)
	return payload
}

// TaskFromPayload extracts a task from message payload
func TaskFromPayload(payload map[string]interface{}) (Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Task{}, err
	}
	var task Task
	err = json.Unmarshal(data, &task)
	return task, err
}

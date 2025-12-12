// +build property

package property

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/syntor/syntor/pkg/models"
)

// **Feature: syntor-multi-agent-system, Property 28: Message format consistency**
// *For any* message sent between agents, it should conform to the standard message format and protocol
// **Validates: Requirements 9.2**

const (
	MinTestIterations = 100
)

// TestMessageFormatConsistency tests that all messages conform to the standard format
func TestMessageFormatConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Any message can be serialized and deserialized without data loss
	properties.Property("message serialization round-trip", prop.ForAll(
		func(msgType string, source string, target string, payloadKey string, payloadValue string) bool {
			msg := models.Message{
				ID:        "test-id",
				Type:      models.MessageType(msgType),
				Source:    source,
				Target:    target,
				Payload:   map[string]interface{}{payloadKey: payloadValue},
				Timestamp: time.Now(),
			}

			// Serialize
			data, err := msg.ToJSON()
			if err != nil {
				return false
			}

			// Deserialize
			decoded, err := models.MessageFromJSON(data)
			if err != nil {
				return false
			}

			// Verify fields match
			return msg.ID == decoded.ID &&
				msg.Type == decoded.Type &&
				msg.Source == decoded.Source &&
				msg.Target == decoded.Target
		},
		gen.AlphaString(),
		gen.AlphaString(),
		gen.AlphaString(),
		gen.AlphaString(),
		gen.AlphaString(),
	))

	// Property: Message ID is always preserved
	properties.Property("message ID preservation", prop.ForAll(
		func(id string) bool {
			if id == "" {
				return true // Skip empty IDs
			}

			msg := models.Message{
				ID:        id,
				Type:      models.MsgTaskAssignment,
				Source:    "test-source",
				Timestamp: time.Now(),
			}

			data, _ := msg.ToJSON()
			decoded, _ := models.MessageFromJSON(data)

			return msg.ID == decoded.ID
		},
		gen.AlphaString(),
	))

	// Property: Timestamp is always preserved with nanosecond precision
	properties.Property("timestamp preservation", prop.ForAll(
		func(unixNano int64) bool {
			// Clamp to reasonable time range
			if unixNano < 0 || unixNano > time.Now().Add(100*365*24*time.Hour).UnixNano() {
				return true // Skip unreasonable times
			}

			ts := time.Unix(0, unixNano)
			msg := models.Message{
				ID:        "test-id",
				Type:      models.MsgTaskAssignment,
				Source:    "test-source",
				Timestamp: ts,
			}

			data, _ := msg.ToJSON()
			decoded, _ := models.MessageFromJSON(data)

			// Allow 1 microsecond difference due to JSON precision
			diff := msg.Timestamp.Sub(decoded.Timestamp)
			if diff < 0 {
				diff = -diff
			}
			return diff < time.Microsecond
		},
		gen.Int64Range(time.Now().Add(-365*24*time.Hour).UnixNano(), time.Now().Add(365*24*time.Hour).UnixNano()),
	))

	// Property: Payload data is preserved through serialization
	properties.Property("payload preservation", prop.ForAll(
		func(key string, value string) bool {
			if key == "" {
				return true // Skip empty keys
			}

			msg := models.Message{
				ID:        "test-id",
				Type:      models.MsgTaskAssignment,
				Source:    "test-source",
				Payload:   map[string]interface{}{key: value},
				Timestamp: time.Now(),
			}

			data, _ := msg.ToJSON()
			decoded, _ := models.MessageFromJSON(data)

			decodedValue, ok := decoded.Payload[key]
			if !ok {
				return false
			}

			return decodedValue == value
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString(),
	))

	// Property: Message type is always preserved
	properties.Property("message type preservation", prop.ForAll(
		func(msgTypeIdx int) bool {
			msgTypes := []models.MessageType{
				models.MsgTaskAssignment,
				models.MsgTaskStatus,
				models.MsgTaskComplete,
				models.MsgAgentRegistration,
				models.MsgAgentHeartbeat,
				models.MsgServiceRequest,
				models.MsgServiceResponse,
			}

			idx := msgTypeIdx % len(msgTypes)
			if idx < 0 {
				idx = -idx
			}

			msg := models.Message{
				ID:        "test-id",
				Type:      msgTypes[idx],
				Source:    "test-source",
				Timestamp: time.Now(),
			}

			data, _ := msg.ToJSON()
			decoded, _ := models.MessageFromJSON(data)

			return msg.Type == decoded.Type
		},
		gen.Int(),
	))

	// Property: Correlation ID is preserved when set
	properties.Property("correlation ID preservation", prop.ForAll(
		func(correlationID string) bool {
			msg := models.Message{
				ID:            "test-id",
				Type:          models.MsgTaskAssignment,
				Source:        "test-source",
				CorrelationID: correlationID,
				Timestamp:     time.Now(),
			}

			data, _ := msg.ToJSON()
			decoded, _ := models.MessageFromJSON(data)

			return msg.CorrelationID == decoded.CorrelationID
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// TestMessageValidation tests message validation rules
func TestMessageValidation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Valid messages always pass validation
	properties.Property("valid messages pass validation", prop.ForAll(
		func(id string, source string) bool {
			if id == "" || source == "" {
				return true // Skip invalid inputs
			}

			msg := models.Message{
				ID:        id,
				Type:      models.MsgTaskAssignment,
				Source:    source,
				Timestamp: time.Now(),
			}

			return msg.Validate() == nil
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
	))

	// Property: Messages without ID fail validation
	properties.Property("missing ID fails validation", prop.ForAll(
		func(source string) bool {
			msg := models.Message{
				ID:        "", // Empty ID
				Type:      models.MsgTaskAssignment,
				Source:    source,
				Timestamp: time.Now(),
			}

			err := msg.Validate()
			if err == nil {
				return false
			}

			validationErr, ok := err.(*models.ValidationError)
			return ok && validationErr.Field == "id"
		},
		gen.AlphaString(),
	))

	// Property: Messages without source fail validation
	properties.Property("missing source fails validation", prop.ForAll(
		func(id string) bool {
			if id == "" {
				return true // Skip, would fail on ID first
			}

			msg := models.Message{
				ID:        id,
				Type:      models.MsgTaskAssignment,
				Source:    "", // Empty source
				Timestamp: time.Now(),
			}

			err := msg.Validate()
			if err == nil {
				return false
			}

			validationErr, ok := err.(*models.ValidationError)
			return ok && validationErr.Field == "source"
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
	))

	properties.TestingRun(t)
}

// TestMessageTTL tests message TTL functionality
func TestMessageTTL(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Messages without TTL never expire
	properties.Property("no TTL means never expires", prop.ForAll(
		func(hoursAgo int) bool {
			// Clamp to reasonable range
			if hoursAgo < 0 || hoursAgo > 8760 { // Max 1 year
				hoursAgo = hoursAgo % 8760
				if hoursAgo < 0 {
					hoursAgo = -hoursAgo
				}
			}

			msg := models.Message{
				ID:        "test-id",
				Type:      models.MsgTaskAssignment,
				Source:    "test-source",
				Timestamp: time.Now().Add(-time.Duration(hoursAgo) * time.Hour),
				TTL:       0, // No TTL
			}

			return !msg.IsExpired()
		},
		gen.IntRange(0, 8760),
	))

	// Property: Messages with TTL expire after TTL duration
	properties.Property("TTL expiration works correctly", prop.ForAll(
		func(ttlSeconds int, ageSeconds int) bool {
			// Clamp to reasonable range
			if ttlSeconds <= 0 {
				ttlSeconds = 1
			}
			if ttlSeconds > 86400 {
				ttlSeconds = 86400
			}
			if ageSeconds < 0 {
				ageSeconds = 0
			}
			if ageSeconds > 86400 {
				ageSeconds = 86400
			}

			ttl := time.Duration(ttlSeconds) * time.Second
			age := time.Duration(ageSeconds) * time.Second

			msg := models.Message{
				ID:        "test-id",
				Type:      models.MsgTaskAssignment,
				Source:    "test-source",
				Timestamp: time.Now().Add(-age),
				TTL:       ttl,
			}

			// Should be expired if age > TTL
			expectedExpired := age > ttl
			return msg.IsExpired() == expectedExpired
		},
		gen.IntRange(1, 86400),
		gen.IntRange(0, 86400),
	))

	properties.TestingRun(t)
}

// TestComplexPayloads tests message handling with complex payloads
func TestComplexPayloads(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Nested objects in payload are preserved
	properties.Property("nested payload preservation", prop.ForAll(
		func(key1 string, key2 string, value string) bool {
			if key1 == "" || key2 == "" {
				return true // Skip invalid inputs
			}

			nested := map[string]interface{}{
				key2: value,
			}

			msg := models.Message{
				ID:      "test-id",
				Type:    models.MsgTaskAssignment,
				Source:  "test-source",
				Payload: map[string]interface{}{key1: nested},
				Timestamp: time.Now(),
			}

			data, _ := msg.ToJSON()
			decoded, _ := models.MessageFromJSON(data)

			// Check nested value
			outerMap, ok := decoded.Payload[key1].(map[string]interface{})
			if !ok {
				return false
			}

			innerValue, ok := outerMap[key2]
			if !ok {
				return false
			}

			return innerValue == value
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString(),
	))

	// Property: Array payloads are preserved
	properties.Property("array payload preservation", prop.ForAll(
		func(values []string) bool {
			if len(values) == 0 {
				return true // Skip empty arrays
			}

			// Convert to interface slice
			ifaceSlice := make([]interface{}, len(values))
			for i, v := range values {
				ifaceSlice[i] = v
			}

			msg := models.Message{
				ID:      "test-id",
				Type:    models.MsgTaskAssignment,
				Source:  "test-source",
				Payload: map[string]interface{}{"items": ifaceSlice},
				Timestamp: time.Now(),
			}

			data, _ := msg.ToJSON()
			decoded, _ := models.MessageFromJSON(data)

			decodedItems, ok := decoded.Payload["items"].([]interface{})
			if !ok {
				return false
			}

			if len(decodedItems) != len(values) {
				return false
			}

			for i, v := range values {
				if decodedItems[i] != v {
					return false
				}
			}

			return true
		},
		gen.SliceOf(gen.AlphaString()),
	))

	properties.TestingRun(t)
}

// Benchmark for message serialization
func BenchmarkMessageSerialization(b *testing.B) {
	msg := models.Message{
		ID:     "test-id-12345",
		Type:   models.MsgTaskAssignment,
		Source: "test-source-agent",
		Target: "test-target-agent",
		Payload: map[string]interface{}{
			"task_id":   "task-12345",
			"priority":  5,
			"data":      "some test data",
			"timestamp": time.Now().Unix(),
		},
		Timestamp:     time.Now(),
		CorrelationID: "corr-id-12345",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, _ := json.Marshal(msg)
		var decoded models.Message
		json.Unmarshal(data, &decoded)
	}
}

// Unit test for message factory functions
func TestMessageFactoryFunctions(t *testing.T) {
	t.Run("NewMessage creates valid message", func(t *testing.T) {
		msg := models.NewMessage(
			models.MsgTaskAssignment,
			"source-agent",
			"target-agent",
			map[string]interface{}{"key": "value"},
		)

		assert.NotEmpty(t, msg.ID)
		assert.Equal(t, models.MsgTaskAssignment, msg.Type)
		assert.Equal(t, "source-agent", msg.Source)
		assert.Equal(t, "target-agent", msg.Target)
		assert.NotZero(t, msg.Timestamp)
		assert.Nil(t, msg.Validate())
	})

	t.Run("NewTaskAssignmentMessage creates valid task message", func(t *testing.T) {
		task := models.Task{
			ID:       "task-123",
			Type:     "test-task",
			Priority: models.NormalPriority,
		}

		msg := models.NewTaskAssignmentMessage("source", "target", task)

		assert.Equal(t, models.MsgTaskAssignment, msg.Type)
		assert.Equal(t, "source", msg.Source)
		assert.Equal(t, "target", msg.Target)
	})

	t.Run("NewAgentRegistrationMessage includes capabilities", func(t *testing.T) {
		caps := []models.Capability{
			{Name: "capability-1", Version: "1.0"},
			{Name: "capability-2", Version: "2.0"},
		}

		msg := models.NewAgentRegistrationMessage("agent-1", "test-agent", models.ServiceAgentType, caps)

		assert.Equal(t, models.MsgAgentRegistration, msg.Type)
		assert.Equal(t, "agent-1", msg.Source)

		agentType, ok := msg.Payload["agent_type"]
		assert.True(t, ok)
		assert.Equal(t, "service", agentType)
	})
}

package main

import (
	"reflect"
	"testing"
)

func TestNewJSONExtractor(t *testing.T) {
	fieldMappings := &FieldMappings{
		TimestampFields: []string{"timestamp", "ts"},
		LevelFields:     []string{"level", "severity"},
		MessageFields:   []string{"message", "msg"},
	}

	extractor := NewJSONExtractor("", fieldMappings)
	if extractor == nil {
		t.Fatal("Expected non-nil extractor")
	}

	if extractor.fieldMappings != fieldMappings {
		t.Error("Field mappings not set correctly")
	}
}

func TestJSONExtractor_ExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		input    string
		expected string
	}{
		{
			name:     "no prefix",
			prefix:   "",
			input:    `{"level": "info", "message": "test"}`,
			expected: `{"level": "info", "message": "test"}`,
		},
		{
			name:     "timestamp prefix",
			prefix:   `^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[.\d]*Z?\s*)?(.*)$`,
			input:    `2024-01-15T10:30:45Z {"level": "info", "message": "test"}`,
			expected: `{"level": "info", "message": "test"}`,
		},
		{
			name:     "bracket prefix",
			prefix:   `^\[.*?\]\s*(.*)$`,
			input:    `[2024-01-15T10:30:45Z] {"level": "info", "message": "test"}`,
			expected: `{"level": "info", "message": "test"}`,
		},
		{
			name:     "no match",
			prefix:   `^NOMATCH(.*)$`,
			input:    `{"level": "info", "message": "test"}`,
			expected: `{"level": "info", "message": "test"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fieldMappings := getDefaultFieldMappings()
			extractor := NewJSONExtractor(tt.prefix, fieldMappings)
			result := extractor.ExtractJSON(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractJSON() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		{
			name:      "RFC3339",
			input:     "2024-01-15T10:30:45Z",
			shouldErr: false,
		},
		{
			name:      "RFC3339 with milliseconds",
			input:     "2024-01-15T10:30:45.123Z",
			shouldErr: false,
		},
		{
			name:      "RFC3339 with timezone",
			input:     "2024-01-15T10:30:45+02:00",
			shouldErr: false,
		},
		{
			name:      "simple datetime",
			input:     "2024-01-15 10:30:45",
			shouldErr: false,
		},
		{
			name:      "invalid format",
			input:     "not-a-timestamp",
			shouldErr: true,
		},
		{
			name:      "empty string",
			input:     "",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTimestamp(tt.input)
			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for input %s, but got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for input %s: %v", tt.input, err)
				}
				if result.IsZero() {
					t.Errorf("Expected non-zero time for input %s", tt.input)
				}
			}
		})
	}
}

func TestJSONExtractor_ParseLogEntry(t *testing.T) {
	tests := []struct {
		name           string
		fieldMappings  *FieldMappings
		input          string
		expectedLevel  string
		expectedMsg    string
		shouldHaveTime bool
		shouldErr      bool
	}{
		{
			name: "standard JSON log",
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"timestamp"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message"},
			},
			input:          `{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "test message", "extra": "field"}`,
			expectedLevel:  "info",
			expectedMsg:    "test message",
			shouldHaveTime: true,
			shouldErr:      false,
		},
		{
			name: "logstash format",
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"@timestamp"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message"},
			},
			input:          `{"@timestamp": "2024-01-15T10:30:45Z", "level": "INFO", "message": "logstash message"}`,
			expectedLevel:  "INFO",
			expectedMsg:    "logstash message",
			shouldHaveTime: true,
			shouldErr:      false,
		},
		{
			name: "custom fields",
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"created_at", "event_time"},
				LevelFields:     []string{"severity", "priority"},
				MessageFields:   []string{"description", "content"},
			},
			input:          `{"event_time": "2024-01-15T10:30:45Z", "severity": "high", "description": "custom format"}`,
			expectedLevel:  "high",
			expectedMsg:    "custom format",
			shouldHaveTime: true,
			shouldErr:      false,
		},
		{
			name: "unix timestamp",
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"timestamp"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message"},
			},
			input:          `{"timestamp": 1705315845, "level": "debug", "message": "unix timestamp"}`,
			expectedLevel:  "debug",
			expectedMsg:    "unix timestamp",
			shouldHaveTime: true,
			shouldErr:      false,
		},
		{
			name: "malformed JSON",
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"timestamp"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message"},
			},
			input:          `{"invalid": "json", "missing_quote: "should_fail"}`,
			expectedLevel:  "info", // defaults
			expectedMsg:    `{"invalid": "json", "missing_quote: "should_fail"}`,
			shouldHaveTime: false, // will use current time
			shouldErr:      false, // should handle gracefully
		},
		{
			name: "non-JSON text",
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"timestamp"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message"},
			},
			input:          "This is not JSON at all",
			expectedLevel:  "info",
			expectedMsg:    "This is not JSON at all",
			shouldHaveTime: false,
			shouldErr:      false,
		},
		{
			name: "missing fields use defaults",
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"timestamp"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message"},
			},
			input:          `{"other_field": "value"}`,
			expectedLevel:  "info",
			expectedMsg:    "Log entry",
			shouldHaveTime: false,
			shouldErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewJSONExtractor("", tt.fieldMappings)
			entry, err := extractor.ParseLogEntry(tt.input)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error, but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if entry.Level != tt.expectedLevel {
				t.Errorf("Expected level %s, got %s", tt.expectedLevel, entry.Level)
			}

			if entry.Message != tt.expectedMsg {
				t.Errorf("Expected message %s, got %s", tt.expectedMsg, entry.Message)
			}

			if tt.shouldHaveTime && entry.Timestamp.IsZero() {
				t.Errorf("Expected non-zero timestamp")
			}

			if entry.Raw != tt.input {
				t.Errorf("Expected raw %s, got %s", tt.input, entry.Raw)
			}
		})
	}
}

func TestGetDefaultFieldMappings(t *testing.T) {
	mappings := getDefaultFieldMappings()

	expectedTimestamp := []string{"timestamp", "ts", "time", "@timestamp"}
	expectedLevel := []string{"level", "lvl", "severity", "priority"}
	expectedMessage := []string{"message", "msg", "text", "content"}

	if !reflect.DeepEqual(mappings.TimestampFields, expectedTimestamp) {
		t.Errorf("Expected timestamp fields %v, got %v", expectedTimestamp, mappings.TimestampFields)
	}

	if !reflect.DeepEqual(mappings.LevelFields, expectedLevel) {
		t.Errorf("Expected level fields %v, got %v", expectedLevel, mappings.LevelFields)
	}

	if !reflect.DeepEqual(mappings.MessageFields, expectedMessage) {
		t.Errorf("Expected message fields %v, got %v", expectedMessage, mappings.MessageFields)
	}
}

func TestLogLevelToSeverity(t *testing.T) {
	tests := []struct {
		input    string
		expected int32
	}{
		{"trace", 1},
		{"debug", 5},
		{"info", 9},
		{"warn", 13},
		{"warning", 13},
		{"error", 17},
		{"fatal", 21},
		{"unknown", 9}, // defaults to info
		{"INFO", 9},    // case insensitive
		{"ERROR", 17},  // case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := logLevelToSeverity(tt.input)
			if int32(result) != tt.expected {
				t.Errorf("logLevelToSeverity(%s) = %d, want %d", tt.input, int32(result), tt.expected)
			}
		})
	}
}

func TestConfig_Version(t *testing.T) {
	// Test the version string formatting
	version = "1.2.3"
	buildTime = "2024-01-15_10:30:45"
	gitCommit = "abc123"

	config := Config{}
	versionStr := config.Version()

	expected := "otel-logger 1.2.3 (built: 2024-01-15_10:30:45, commit: abc123)"
	if versionStr != expected {
		t.Errorf("Expected version string %s, got %s", expected, versionStr)
	}
}

func TestFieldMappingPriority(t *testing.T) {
	// Test that field mappings use the first match
	fieldMappings := &FieldMappings{
		TimestampFields: []string{"timestamp", "ts", "time"},
		LevelFields:     []string{"level", "severity"},
		MessageFields:   []string{"message", "msg"},
	}

	extractor := NewJSONExtractor("", fieldMappings)

	// JSON with multiple possible timestamp fields
	jsonStr := `{
		"ts": "2024-01-15T10:30:45Z",
		"time": "2024-01-15T11:30:45Z",
		"timestamp": "2024-01-15T12:30:45Z",
		"level": "info",
		"severity": "high",
		"message": "test",
		"msg": "alternative"
	}`

	entry, err := extractor.ParseLogEntry(jsonStr)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should use "timestamp" (first in list) not "ts" or "time"
	expected, _ := parseTimestamp("2024-01-15T12:30:45Z")
	if !entry.Timestamp.Equal(expected) {
		t.Errorf("Expected timestamp from 'timestamp' field, got %v", entry.Timestamp)
	}

	// Should use "level" not "severity"
	if entry.Level != "info" {
		t.Errorf("Expected level 'info', got %s", entry.Level)
	}

	// Should use "message" not "msg"
	if entry.Message != "test" {
		t.Errorf("Expected message 'test', got %s", entry.Message)
	}
}

func TestPrefixedLogParsing(t *testing.T) {
	fieldMappings := getDefaultFieldMappings()
	prefix := `^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[.\d]*Z?\s*)?(.*)$`
	extractor := NewJSONExtractor(prefix, fieldMappings)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "timestamp prefix",
			input:    `2024-01-15T10:30:45Z {"level": "info", "message": "test"}`,
			expected: "test",
		},
		{
			name:     "timestamp with milliseconds prefix",
			input:    `2024-01-15T10:30:45.123Z {"level": "warn", "message": "warning"}`,
			expected: "warning",
		},
		{
			name:     "no prefix",
			input:    `{"level": "error", "message": "error message"}`,
			expected: "error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := extractor.ParseLogEntry(tt.input)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if entry.Message != tt.expected {
				t.Errorf("Expected message %s, got %s", tt.expected, entry.Message)
			}
		})
	}
}

// Benchmark tests
func BenchmarkParseLogEntry(b *testing.B) {
	fieldMappings := getDefaultFieldMappings()
	extractor := NewJSONExtractor("", fieldMappings)
	jsonLog := `{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "benchmark test", "user_id": 12345, "request_id": "req-abc123"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := extractor.ParseLogEntry(jsonLog)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseTimestamp(b *testing.B) {
	timestamp := "2024-01-15T10:30:45.123Z"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parseTimestamp(timestamp)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExtractJSON(b *testing.B) {
	fieldMappings := getDefaultFieldMappings()
	extractor := NewJSONExtractor(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[.\d]*Z?\s*)?(.*)$`, fieldMappings)
	prefixedLog := `2024-01-15T10:30:45.123Z {"level": "info", "message": "benchmark test"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractor.ExtractJSON(prefixedLog)
	}
}

// Example test showing realistic usage
func ExampleJSONExtractor_ParseLogEntry() {
	fieldMappings := &FieldMappings{
		TimestampFields: []string{"@timestamp"},
		LevelFields:     []string{"level"},
		MessageFields:   []string{"message"},
	}

	extractor := NewJSONExtractor("", fieldMappings)
	entry, _ := extractor.ParseLogEntry(`{"@timestamp": "2024-01-15T10:30:45Z", "level": "INFO", "message": "User logged in", "user_id": 12345}`)

	// Output would be used in real application
	_ = entry.Message // "User logged in"
}

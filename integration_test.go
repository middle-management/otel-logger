package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestJSONExtractionIntegration tests the complete JSON extraction pipeline
func TestJSONExtractionIntegration(t *testing.T) {
	tests := []struct {
		name           string
		input          []string
		fieldMappings  *FieldMappings
		prefix         string
		expectedCount  int
		expectedLevels []string
	}{
		{
			name: "standard JSON logs",
			input: []string{
				`{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "User logged in", "user_id": 12345}`,
				`{"timestamp": "2024-01-15T10:30:46Z", "level": "error", "message": "Database connection failed", "error": "timeout"}`,
				`{"timestamp": "2024-01-15T10:30:47Z", "level": "debug", "message": "Cache hit", "key": "user:12345"}`,
			},
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"timestamp"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message"},
			},
			expectedCount:  3,
			expectedLevels: []string{"info", "error", "debug"},
		},
		{
			name: "logstash format",
			input: []string{
				`{"@timestamp": "2024-01-15T10:30:45Z", "level": "INFO", "message": "Application started", "version": "1.0.0"}`,
				`{"@timestamp": "2024-01-15T10:30:46Z", "level": "WARN", "message": "High memory usage", "memory": "85%"}`,
			},
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"@timestamp"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message"},
			},
			expectedCount:  2,
			expectedLevels: []string{"INFO", "WARN"},
		},
		{
			name: "prefixed logs",
			input: []string{
				`2024-01-15T10:30:45Z {"level": "info", "message": "Prefixed log entry"}`,
				`2024-01-15T10:30:46.123Z {"level": "error", "message": "Error with milliseconds"}`,
			},
			fieldMappings:  getDefaultFieldMappings(),
			prefix:         `^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[.\d]*Z?\s*)?(.*)$`,
			expectedCount:  2,
			expectedLevels: []string{"info", "error"},
		},
		{
			name: "mixed valid and invalid JSON",
			input: []string{
				`{"level": "info", "message": "Valid JSON"}`,
				`This is not JSON at all`,
				`{"level": "error", "message": "Another valid JSON"}`,
				`{"malformed": "json", "missing_quote: "should_fail"}`,
			},
			fieldMappings:  getDefaultFieldMappings(),
			expectedCount:  4,                                         // All should be processed, invalid ones as plain text
			expectedLevels: []string{"info", "info", "error", "info"}, // invalid ones default to "info"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewJSONExtractor(tt.prefix, tt.fieldMappings)

			var parsedEntries []*LogEntry
			for _, input := range tt.input {
				entry, err := extractor.ParseLogEntry(input)
				if err != nil {
					t.Errorf("Unexpected error parsing entry: %v", err)
					continue
				}
				parsedEntries = append(parsedEntries, entry)
			}

			if len(parsedEntries) != tt.expectedCount {
				t.Errorf("Expected %d entries, got %d", tt.expectedCount, len(parsedEntries))
			}

			for i, entry := range parsedEntries {
				if i < len(tt.expectedLevels) && entry.Level != tt.expectedLevels[i] {
					t.Errorf("Entry %d: expected level %s, got %s", i, tt.expectedLevels[i], entry.Level)
				}
			}
		})
	}
}

// TestConfigurationIntegration tests different configuration scenarios
func TestConfigurationIntegration(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		valid  bool
	}{
		{
			name: "valid config",
			config: &Config{
				Timeout:       10 * time.Second,
				BatchSize:     50,
				FlushInterval: 5 * time.Second,
			},
			valid: true,
		},
		{
			name: "config with custom batching",
			config: &Config{
				Timeout:       10 * time.Second,
				BatchSize:     100,
				FlushInterval: 10 * time.Second,
			},
			valid: true,
		},
		{
			name: "config with field mappings",
			config: &Config{
				TimestampFields: []string{"timestamp", "ts"},
				LevelFields:     []string{"level", "severity"},
				MessageFields:   []string{"message", "msg"},
				Timeout:         5 * time.Second,
				BatchSize:       25,
				FlushInterval:   2 * time.Second,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Try to create a logger provider - this tests the configuration
			provider, err := createLoggerProvider(ctx, tt.config)

			if tt.valid {
				if err != nil {
					t.Errorf("Expected valid config, but got error: %v", err)
					return
				}
				if provider == nil {
					t.Error("Expected non-nil provider for valid config")
					return
				}

				// Clean up
				if shutdownErr := provider.Shutdown(ctx); shutdownErr != nil {
					t.Logf("Error shutting down provider: %v", shutdownErr)
				}
			} else {
				if err == nil {
					t.Error("Expected error for invalid config, but got nil")
					if provider != nil {
						provider.Shutdown(ctx)
					}
				}
			}
		})
	}
}

// TestFieldMappingPriorityIntegration tests field mapping priority in realistic scenarios
func TestFieldMappingPriorityIntegration(t *testing.T) {
	// Simulate logs from different systems with overlapping field names
	logInputs := []struct {
		name   string
		format string
		input  string
	}{
		{
			name:   "winston.js",
			format: "winston",
			input:  `{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Winston log", "service": "api"}`,
		},
		{
			name:   "bunyan",
			format: "bunyan",
			input:  `{"time": "2024-01-15T10:30:45Z", "level": 30, "msg": "Bunyan log", "name": "api"}`,
		},
		{
			name:   "logrus",
			format: "logrus",
			input:  `{"time": "2024-01-15T10:30:45Z", "level": "info", "msg": "Logrus log", "component": "api"}`,
		},
		{
			name:   "zap",
			format: "zap",
			input:  `{"ts": 1705315845.123, "level": "info", "msg": "Zap log", "caller": "api/main.go:42"}`,
		},
	}

	// Test with different field mapping configurations
	mappingConfigs := []struct {
		name     string
		mappings *FieldMappings
	}{
		{
			name: "timestamp priority",
			mappings: &FieldMappings{
				TimestampFields: []string{"timestamp", "time", "ts"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message", "msg"},
			},
		},
		{
			name: "time priority",
			mappings: &FieldMappings{
				TimestampFields: []string{"time", "timestamp", "ts"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"msg", "message"},
			},
		},
	}

	for _, mappingConfig := range mappingConfigs {
		for _, logInput := range logInputs {
			t.Run(mappingConfig.name+"_"+logInput.name, func(t *testing.T) {
				extractor := NewJSONExtractor("", mappingConfig.mappings)
				entry, err := extractor.ParseLogEntry(logInput.input)

				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}

				// Verify that the entry was parsed successfully
				if entry.Message == "" {
					t.Error("Expected non-empty message")
				}

				if entry.Level == "" {
					t.Error("Expected non-empty level")
				}

				if entry.Timestamp.IsZero() {
					t.Error("Expected non-zero timestamp")
				}

				// Verify raw log is preserved
				if entry.Raw != logInput.input {
					t.Error("Expected raw log to be preserved")
				}
			})
		}
	}
}

// TestLogProcessingPipeline tests the complete log processing pipeline
func TestLogProcessingPipeline(t *testing.T) {
	config := &Config{
		Timeout:         5 * time.Second,
		BatchSize:       10,
		FlushInterval:   1 * time.Second,
		TimestampFields: []string{"timestamp", "@timestamp"},
		LevelFields:     []string{"level", "severity"},
		MessageFields:   []string{"message", "msg"},
	}

	// Create field mappings from config
	fieldMappings := &FieldMappings{
		TimestampFields: config.TimestampFields,
		LevelFields:     config.LevelFields,
		MessageFields:   config.MessageFields,
	}

	extractor := NewJSONExtractor(config.JSONPrefix, fieldMappings)

	// Test logs from different sources
	testLogs := []string{
		`{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Application started"}`,
		`{"@timestamp": "2024-01-15T10:30:46Z", "severity": "warning", "msg": "High CPU usage"}`,
		`{"timestamp": "2024-01-15T10:30:47Z", "level": "error", "message": "Database error", "error": "connection timeout"}`,
		`This is a plain text log entry`,
		`{"malformed": "json", "missing": quote}`, // Invalid JSON
	}

	var processedEntries []*LogEntry
	for _, logLine := range testLogs {
		entry, err := extractor.ParseLogEntry(logLine)
		if err != nil {
			t.Errorf("Error processing log line: %v", err)
			continue
		}
		processedEntries = append(processedEntries, entry)
	}

	// Verify all logs were processed
	if len(processedEntries) != len(testLogs) {
		t.Errorf("Expected %d processed entries, got %d", len(testLogs), len(processedEntries))
	}

	// Verify specific characteristics
	for i, entry := range processedEntries {
		if entry.Raw != testLogs[i] {
			t.Errorf("Entry %d: raw log not preserved", i)
		}

		if entry.Level == "" {
			t.Errorf("Entry %d: level should not be empty", i)
		}

		if entry.Message == "" {
			t.Errorf("Entry %d: message should not be empty", i)
		}

		if entry.Timestamp.IsZero() {
			t.Errorf("Entry %d: timestamp should not be zero", i)
		}
	}
}

// TestErrorHandling tests various error conditions
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		config        *Config
		expectError   bool
		errorContains string
	}{
		{
			name: "negative batch size",
			config: &Config{
				BatchSize:     -10,
				Timeout:       10 * time.Second,
				FlushInterval: 5 * time.Second,
			},
			expectError:   false, // Validation happens at runtime, not during provider creation
			errorContains: "",
		},
		{
			name: "zero timeout",
			config: &Config{
				BatchSize:     50,
				Timeout:       0,
				FlushInterval: 5 * time.Second,
			},
			expectError:   false, // Zero timeout doesn't cause immediate error in provider creation
			errorContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			provider, err := createLoggerProvider(ctx, tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
					if provider != nil {
						provider.Shutdown(ctx)
					}
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if provider != nil {
					if shutdownErr := provider.Shutdown(ctx); shutdownErr != nil {
						t.Logf("Error shutting down provider: %v", shutdownErr)
					}
				}
			}
		})
	}
}

// TestSeverityMapping tests the log level to severity mapping
func TestSeverityMapping(t *testing.T) {
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
		{"unknown", 9},
		{"INFO", 9},   // case insensitive
		{"ERROR", 17}, // case insensitive
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

// TestCommandExecution tests the command wrapping functionality
func TestCommandExecution(t *testing.T) {
	tests := []struct {
		name        string
		command     []string
		expectError bool
	}{
		{
			name:        "simple echo command",
			command:     []string{"echo", "Hello World"},
			expectError: false,
		},
		{
			name:        "command with stderr output",
			command:     []string{"sh", "-c", "echo 'stdout'; echo 'stderr' >&2"},
			expectError: false,
		},
		{
			name:        "command with JSON output",
			command:     []string{"echo", `{"level":"info","message":"Test JSON"}`},
			expectError: false,
		},
		{
			name:        "failing command",
			command:     []string{"sh", "-c", "exit 1"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Timeout:       5 * time.Second,
				BatchSize:     10,
				FlushInterval: 1 * time.Second,
				Command:       tt.command,
			}

			ctx := context.Background()
			provider, err := createLoggerProvider(ctx, config)
			if err != nil {
				t.Fatalf("Failed to create logger provider: %v", err)
			}
			defer provider.Shutdown(ctx)

			fieldMappings := getDefaultFieldMappings()
			extractor := NewJSONExtractor(config.JSONPrefix, fieldMappings)
			logger := provider.Logger("test-command")
			processor := NewLogProcessor(logger)

			err = executeCommand(ctx, config, extractor, processor)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			if flushErr := provider.ForceFlush(ctx); flushErr != nil {
				t.Logf("Error flushing logs: %v", flushErr)
			}
		})
	}
}

// TestStreamTagging tests that log entries are properly tagged with stream information
func TestStreamTagging(t *testing.T) {
	entry := &LogEntry{
		Timestamp: time.Now(),
		Level:     "info",
		Message:   "Test message",
		Fields:    map[string]interface{}{},
		Raw:       "test log",
		Stream:    "stdout",
	}

	// Create a logger provider to test stream tagging
	ctx := context.Background()
	config := &Config{}

	provider, err := createLoggerProvider(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create logger provider: %v", err)
	}
	defer provider.Shutdown(ctx)

	logger := provider.Logger("test")
	processor := NewLogProcessor(logger)

	// Process the entry - this should include stream tagging
	processor.ProcessLogEntry(ctx, entry)

	// Verify that the entry has the stream field set
	if entry.Stream != "stdout" {
		t.Errorf("Expected stream 'stdout', got '%s'", entry.Stream)
	}

	// Test different stream types
	streams := []string{"stdout", "stderr", "system"}
	for _, stream := range streams {
		testEntry := &LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   fmt.Sprintf("Test message for %s", stream),
			Fields:    map[string]interface{}{},
			Raw:       fmt.Sprintf("test log for %s", stream),
			Stream:    stream,
		}

		processor.ProcessLogEntry(ctx, testEntry)

		if testEntry.Stream != stream {
			t.Errorf("Expected stream '%s', got '%s'", stream, testEntry.Stream)
		}
	}
}

// TestCommandWrappingIntegration tests the complete command wrapping pipeline
func TestCommandWrappingIntegration(t *testing.T) {
	// Test basic command execution modes
	tests := []struct {
		name     string
		hasCmd   bool
		command  []string
		useStdin bool
	}{
		{
			name:     "stdin mode",
			hasCmd:   false,
			useStdin: true,
		},
		{
			name:     "command mode",
			hasCmd:   true,
			command:  []string{"echo", "test"},
			useStdin: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Timeout:       5 * time.Second,
				BatchSize:     10,
				FlushInterval: 1 * time.Second,
			}

			if tt.hasCmd {
				config.Command = tt.command
			}

			// Test that we can detect the mode correctly
			if len(config.Command) > 0 && !tt.hasCmd {
				t.Error("Expected no command but found one")
			}

			if len(config.Command) == 0 && tt.hasCmd {
				t.Error("Expected command but found none")
			}

			// Verify the configuration is valid
			ctx := context.Background()
			provider, err := createLoggerProvider(ctx, config)
			if err != nil {
				t.Fatalf("Failed to create logger provider: %v", err)
			}
			defer provider.Shutdown(ctx)
		})
	}
}

// BenchmarkCompleteLogProcessing benchmarks the complete log processing pipeline
func BenchmarkCompleteLogProcessing(b *testing.B) {
	fieldMappings := getDefaultFieldMappings()
	extractor := NewJSONExtractor("", fieldMappings)

	testLog := `{"timestamp": "2024-01-15T10:30:45.123Z", "level": "info", "message": "User action completed", "user_id": 12345, "action": "login", "ip": "192.168.1.1", "user_agent": "Mozilla/5.0", "duration_ms": 234}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry, err := extractor.ParseLogEntry(testLog)
		if err != nil {
			b.Fatal(err)
		}

		// Simulate some processing
		_ = entry.Level
		_ = entry.Message
		_ = entry.Timestamp
		_ = len(entry.Fields)
	}
}

// BenchmarkJSONExtraction benchmarks JSON extraction with prefixes
func BenchmarkJSONExtraction(b *testing.B) {
	fieldMappings := getDefaultFieldMappings()
	extractor := NewJSONExtractor(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[.\d]*Z?\s*)?(.*)$`, fieldMappings)

	prefixedLog := `2024-01-15T10:30:45.123Z {"level": "info", "message": "Prefixed log entry", "service": "api"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jsonPart := extractor.ExtractJSON(prefixedLog)
		if jsonPart == "" {
			b.Fatal("Failed to extract JSON")
		}
	}
}

// TestParallellSortJSON tests parsing of the real PostgreSQL EXPLAIN ANALYZE JSON file
func TestParallellSortJSON(t *testing.T) {
	// Read the actual parallellsort.json file
	content, err := os.ReadFile("examples/parallellsort.json")
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	reader := strings.NewReader(string(content))
	continuationPattern := regexp.MustCompile(`^[ \t]`)

	var entries []string
	for logEntry := range multilineLogIterator(reader, continuationPattern) {
		entries = append(entries, logEntry)
	}

	// Should be parsed as a single log entry
	if len(entries) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(entries))
		if len(entries) > 1 {
			t.Logf("First entry length: %d", len(entries[0]))
			for i, entry := range entries {
				t.Logf("Entry %d (first 100 chars): %s", i, entry[:min(100, len(entry))])
			}
		}
		return
	}

	// Verify it's valid JSON (should be an array)
	var jsonData []map[string]any
	if err := json.Unmarshal([]byte(entries[0]), &jsonData); err != nil {
		t.Errorf("Failed to parse as JSON array: %v", err)
		t.Logf("First 200 chars of entry: %s", entries[0][:min(200, len(entries[0]))])
		return
	}

	// Verify expected structure
	if len(jsonData) == 0 {
		t.Error("Expected at least one element in JSON array")
		return
	}

	// Check for expected top-level fields in the first element
	firstElement := jsonData[0]

	if _, ok := firstElement["Plan"]; !ok {
		t.Error("Expected 'Plan' field in JSON object")
	}

	if _, ok := firstElement["Execution Time"]; !ok {
		t.Error("Expected 'Execution Time' field in JSON object")
	}

	if _, ok := firstElement["Planning Time"]; !ok {
		t.Error("Expected 'Planning Time' field in JSON object")
	}

	// Verify the Plan field is a nested structure
	if plan, ok := firstElement["Plan"].(map[string]any); ok {
		if nodeType, ok := plan["Node Type"].(string); !ok || nodeType == "" {
			t.Error("Expected 'Node Type' in Plan object")
		}
	} else {
		t.Error("Expected 'Plan' to be a nested object")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

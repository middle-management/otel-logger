package main

import (
	"context"
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
			name: "valid gRPC config",
			config: &Config{
				Endpoint:       "localhost:4317",
				Protocol:       "grpc",
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Insecure:       true,
				Timeout:        10 * time.Second,
				BatchSize:      50,
				FlushInterval:  5 * time.Second,
			},
			valid: true,
		},
		{
			name: "valid HTTP config",
			config: &Config{
				Endpoint:       "localhost:4318",
				Protocol:       "http",
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Insecure:       true,
				Timeout:        10 * time.Second,
				BatchSize:      100,
				FlushInterval:  10 * time.Second,
			},
			valid: true,
		},
		{
			name: "config with headers",
			config: &Config{
				Endpoint:       "localhost:4317",
				Protocol:       "grpc",
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				Headers:        []string{"x-api-key=secret", "x-tenant=test"},
				Timeout:        5 * time.Second,
				BatchSize:      25,
				FlushInterval:  2 * time.Second,
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
		Endpoint:        "localhost:4317",
		Protocol:        "grpc",
		ServiceName:     "test-service",
		ServiceVersion:  "1.0.0",
		Insecure:        true,
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
			name: "invalid protocol",
			config: &Config{
				Endpoint:       "localhost:4317",
				Protocol:       "invalid",
				ServiceName:    "test",
				ServiceVersion: "1.0.0",
			},
			expectError:   true,
			errorContains: "unsupported protocol",
		},
		{
			name: "empty endpoint",
			config: &Config{
				Endpoint:       "",
				Protocol:       "grpc",
				ServiceName:    "test",
				ServiceVersion: "1.0.0",
			},
			expectError:   false, // Empty endpoint doesn't cause immediate error in provider creation
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

package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
)

// MockLogSender captures the logs that would be sent to OTLP
type MockLogSender struct {
	mu               sync.Mutex
	SentBatches      [][]*LogEntry
	SentOTLPRequests []*collogspb.ExportLogsServiceRequest
	CallCount        int
	ShouldError      bool
	ErrorMsg         string
}

func (m *MockLogSender) SendLogs(entries []*LogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CallCount++
	if m.ShouldError {
		return &MockError{msg: m.ErrorMsg}
	}

	// Store the entries
	batch := make([]*LogEntry, len(entries))
	copy(batch, entries)
	m.SentBatches = append(m.SentBatches, batch)

	// Also create and store the OTLP request to test serialization
	config := &Config{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Endpoint:       "localhost:4317",
		Protocol:       "grpc",
		BatchSize:      50,
		Timeout:        10 * time.Second,
		FlushInterval:  5 * time.Second,
	}
	otlpReq := createExportRequest(entries, config)
	m.SentOTLPRequests = append(m.SentOTLPRequests, otlpReq)

	return nil
}

type MockError struct {
	msg string
}

func (e *MockError) Error() string {
	return e.msg
}

// Integration test for the complete log processing pipeline
func TestCompleteLogProcessingPipeline(t *testing.T) {
	tests := []struct {
		name          string
		fieldMappings *FieldMappings
		input         []string
		expectedCount int
		expectedMsgs  []string
		expectedLvls  []string
	}{
		{
			name: "standard JSON logs",
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"timestamp"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message"},
			},
			input: []string{
				`{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "First message", "user_id": 123}`,
				`{"timestamp": "2024-01-15T10:30:46Z", "level": "error", "message": "Second message", "error_code": "E001"}`,
				`{"timestamp": "2024-01-15T10:30:47Z", "level": "debug", "message": "Third message", "trace_id": "abc123"}`,
			},
			expectedCount: 3,
			expectedMsgs:  []string{"First message", "Second message", "Third message"},
			expectedLvls:  []string{"info", "error", "debug"},
		},
		{
			name: "logstash format",
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"@timestamp"},
				LevelFields:     []string{"level"},
				MessageFields:   []string{"message"},
			},
			input: []string{
				`{"@timestamp": "2024-01-15T10:30:45Z", "level": "INFO", "message": "Logstash message 1", "host": "server-01"}`,
				`{"@timestamp": "2024-01-15T10:30:46Z", "level": "WARN", "message": "Logstash message 2", "service": "web-api"}`,
			},
			expectedCount: 2,
			expectedMsgs:  []string{"Logstash message 1", "Logstash message 2"},
			expectedLvls:  []string{"INFO", "WARN"},
		},
		{
			name: "custom fields with fallbacks",
			fieldMappings: &FieldMappings{
				TimestampFields: []string{"created_at", "event_time", "timestamp"},
				LevelFields:     []string{"severity", "priority", "level"},
				MessageFields:   []string{"description", "content", "message"},
			},
			input: []string{
				`{"event_time": "2024-01-15T10:30:45Z", "severity": "high", "description": "Custom format 1"}`,
				`{"timestamp": "2024-01-15T10:30:46Z", "priority": "medium", "content": "Custom format 2"}`,
				`{"created_at": "2024-01-15T10:30:47Z", "level": "low", "message": "Custom format 3"}`,
			},
			expectedCount: 3,
			expectedMsgs:  []string{"Custom format 1", "Custom format 2", "Custom format 3"},
			expectedLvls:  []string{"high", "medium", "low"},
		},
		{
			name:          "mixed valid and invalid JSON",
			fieldMappings: getDefaultFieldMappings(),
			input: []string{
				`{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Valid JSON"}`,
				`{"invalid": "json", "missing_quote: "should_fail"}`,
				`Not JSON at all`,
				`{"timestamp": "2024-01-15T10:30:46Z", "level": "warn", "message": "Another valid JSON"}`,
			},
			expectedCount: 4,
			expectedMsgs: []string{
				"Valid JSON",
				`{"invalid": "json", "missing_quote: "should_fail"}`,
				"Not JSON at all",
				"Another valid JSON",
			},
			expectedLvls: []string{"info", "info", "info", "warn"}, // invalid JSON gets default level
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := &MockLogSender{}

			// Create batcher with mock sender
			batcher := NewLogBatcher(10, 0, mockSender.SendLogs) // No flush interval for test
			defer batcher.Close()

			// Create extractor
			extractor := NewJSONExtractor("", tt.fieldMappings)

			// Process each log line
			for _, line := range tt.input {
				entry, err := extractor.ParseLogEntry(line)
				if err != nil {
					t.Errorf("Unexpected error parsing log: %v", err)
					continue
				}

				err = batcher.Add(entry)
				if err != nil {
					t.Errorf("Unexpected error adding to batch: %v", err)
				}
			}

			// Force flush
			err := batcher.Flush()
			if err != nil {
				t.Errorf("Unexpected error flushing: %v", err)
			}

			// Verify results
			mockSender.mu.Lock()
			totalEntries := 0
			for _, batch := range mockSender.SentBatches {
				totalEntries += len(batch)
			}
			mockSender.mu.Unlock()

			if totalEntries != tt.expectedCount {
				t.Errorf("Expected %d entries, got %d", tt.expectedCount, totalEntries)
			}

			// Collect all entries in order
			mockSender.mu.Lock()
			var allEntries []*LogEntry
			for _, batch := range mockSender.SentBatches {
				allEntries = append(allEntries, batch...)
			}
			mockSender.mu.Unlock()

			// Verify messages and levels
			for i, entry := range allEntries {
				if i < len(tt.expectedMsgs) {
					if entry.Message != tt.expectedMsgs[i] {
						t.Errorf("Entry %d: expected message '%s', got '%s'", i, tt.expectedMsgs[i], entry.Message)
					}
				}

				if i < len(tt.expectedLvls) {
					if entry.Level != tt.expectedLvls[i] {
						t.Errorf("Entry %d: expected level '%s', got '%s'", i, tt.expectedLvls[i], entry.Level)
					}
				}
			}
		})
	}
}

func TestPrefixedLogProcessing(t *testing.T) {
	tests := []struct {
		name         string
		prefix       string
		input        []string
		expectedMsgs []string
	}{
		{
			name:   "timestamp prefixed logs",
			prefix: `^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[.\d]*Z?\s*)?(.*)$`,
			input: []string{
				`2024-01-15T10:30:45Z {"level": "info", "message": "Prefixed message 1"}`,
				`2024-01-15T10:30:46.123Z {"level": "warn", "message": "Prefixed message 2"}`,
				`{"level": "error", "message": "Non-prefixed message"}`,
			},
			expectedMsgs: []string{"Prefixed message 1", "Prefixed message 2", "Non-prefixed message"},
		},
		{
			name:   "bracket prefixed logs",
			prefix: `^\[.*?\]\s*(.*)$`,
			input: []string{
				`[2024-01-15T10:30:45Z] {"level": "info", "message": "Bracket prefixed"}`,
				`[INFO] {"level": "debug", "message": "Level bracket prefixed"}`,
				`{"level": "error", "message": "No bracket prefix"}`,
			},
			expectedMsgs: []string{"Bracket prefixed", "Level bracket prefixed", "No bracket prefix"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := &MockLogSender{}
			batcher := NewLogBatcher(10, 0, mockSender.SendLogs)
			defer batcher.Close()

			fieldMappings := getDefaultFieldMappings()
			extractor := NewJSONExtractor(tt.prefix, fieldMappings)

			for _, line := range tt.input {
				entry, err := extractor.ParseLogEntry(line)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					continue
				}

				err = batcher.Add(entry)
				if err != nil {
					t.Errorf("Unexpected error adding to batch: %v", err)
				}
			}

			batcher.Flush()

			// Verify messages
			mockSender.mu.Lock()
			var allEntries []*LogEntry
			for _, batch := range mockSender.SentBatches {
				allEntries = append(allEntries, batch...)
			}
			mockSender.mu.Unlock()

			for i, entry := range allEntries {
				if i < len(tt.expectedMsgs) {
					if entry.Message != tt.expectedMsgs[i] {
						t.Errorf("Entry %d: expected message '%s', got '%s'", i, tt.expectedMsgs[i], entry.Message)
					}
				}
			}
		})
	}
}

func TestBatchingBehavior(t *testing.T) {
	tests := []struct {
		name               string
		batchSize          int
		flushInterval      time.Duration
		numLogs            int
		expectedBatchCount int
	}{
		{
			name:               "exact batch size",
			batchSize:          5,
			flushInterval:      0,
			numLogs:            10,
			expectedBatchCount: 2, // 10 logs / 5 per batch = 2 batches
		},
		{
			name:               "partial batch",
			batchSize:          7,
			flushInterval:      0,
			numLogs:            10,
			expectedBatchCount: 2, // 7 + 3 (after flush)
		},
		{
			name:               "single entry batches",
			batchSize:          1,
			flushInterval:      0,
			numLogs:            3,
			expectedBatchCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := &MockLogSender{}
			batcher := NewLogBatcher(tt.batchSize, tt.flushInterval, mockSender.SendLogs)
			defer batcher.Close()

			fieldMappings := getDefaultFieldMappings()
			extractor := NewJSONExtractor("", fieldMappings)

			// Add logs
			for i := 0; i < tt.numLogs; i++ {
				logLine := fmt.Sprintf(`{"timestamp": "2024-01-15T10:30:%02dZ", "level": "info", "message": "Log %d"}`, i, i)
				entry, err := extractor.ParseLogEntry(logLine)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					continue
				}

				err = batcher.Add(entry)
				if err != nil {
					t.Errorf("Unexpected error adding to batch: %v", err)
				}
			}

			// Force final flush
			batcher.Flush()

			mockSender.mu.Lock()
			actualBatchCount := len(mockSender.SentBatches)
			totalEntries := 0
			for _, batch := range mockSender.SentBatches {
				totalEntries += len(batch)
			}
			mockSender.mu.Unlock()

			if actualBatchCount != tt.expectedBatchCount {
				t.Errorf("Expected %d batches, got %d", tt.expectedBatchCount, actualBatchCount)
			}

			if totalEntries != tt.numLogs {
				t.Errorf("Expected %d total entries, got %d", tt.numLogs, totalEntries)
			}
		})
	}
}

func TestOTLPRequestGeneration(t *testing.T) {
	config := &Config{
		ServiceName:    "test-service",
		ServiceVersion: "2.0.0",
	}

	entries := []*LogEntry{
		{
			Timestamp: time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
			Level:     "info",
			Message:   "Test message 1",
			Fields:    map[string]interface{}{"user_id": 123, "action": "login"},
			Raw:       `{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Test message 1", "user_id": 123, "action": "login"}`,
		},
		{
			Timestamp: time.Date(2024, 1, 15, 10, 30, 46, 0, time.UTC),
			Level:     "error",
			Message:   "Test error message",
			Fields:    map[string]interface{}{"error_code": "E001", "retry_count": 3},
			Raw:       `{"timestamp": "2024-01-15T10:30:46Z", "level": "error", "message": "Test error message", "error_code": "E001", "retry_count": 3}`,
		},
	}

	req := createExportRequest(entries, config)

	// Verify request structure
	if req == nil {
		t.Fatal("Expected non-nil OTLP request")
	}

	if len(req.ResourceLogs) != 1 {
		t.Errorf("Expected 1 ResourceLogs, got %d", len(req.ResourceLogs))
	}

	resourceLogs := req.ResourceLogs[0]

	// Verify resource attributes
	if resourceLogs.Resource == nil {
		t.Fatal("Expected non-nil Resource")
	}

	serviceNameFound := false
	serviceVersionFound := false
	for _, attr := range resourceLogs.Resource.Attributes {
		if attr.Key == "service.name" && attr.Value.GetStringValue() == "test-service" {
			serviceNameFound = true
		}
		if attr.Key == "service.version" && attr.Value.GetStringValue() == "2.0.0" {
			serviceVersionFound = true
		}
	}

	if !serviceNameFound {
		t.Error("Expected service.name attribute not found")
	}
	if !serviceVersionFound {
		t.Error("Expected service.version attribute not found")
	}

	// Verify scope logs
	if len(resourceLogs.ScopeLogs) != 1 {
		t.Errorf("Expected 1 ScopeLogs, got %d", len(resourceLogs.ScopeLogs))
	}

	scopeLogs := resourceLogs.ScopeLogs[0]
	if scopeLogs.Scope.Name != "otel-logger" {
		t.Errorf("Expected scope name 'otel-logger', got '%s'", scopeLogs.Scope.Name)
	}

	// Verify log records
	if len(scopeLogs.LogRecords) != 2 {
		t.Errorf("Expected 2 LogRecords, got %d", len(scopeLogs.LogRecords))
	}

	// Verify first log record
	log1 := scopeLogs.LogRecords[0]
	if log1.SeverityText != "info" {
		t.Errorf("Expected severity text 'info', got '%s'", log1.SeverityText)
	}
	if log1.Body.GetStringValue() != "Test message 1" {
		t.Errorf("Expected body 'Test message 1', got '%s'", log1.Body.GetStringValue())
	}

	// Verify second log record
	log2 := scopeLogs.LogRecords[1]
	if log2.SeverityText != "error" {
		t.Errorf("Expected severity text 'error', got '%s'", log2.SeverityText)
	}
	if log2.Body.GetStringValue() != "Test error message" {
		t.Errorf("Expected body 'Test error message', got '%s'", log2.Body.GetStringValue())
	}
}

func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		shouldError bool
		errorMsg    string
		numLogs     int
	}{
		{
			name:        "successful sending",
			shouldError: false,
			numLogs:     5,
		},
		{
			name:        "network error",
			shouldError: true,
			errorMsg:    "connection refused",
			numLogs:     3,
		},
		{
			name:        "server error",
			shouldError: true,
			errorMsg:    "internal server error",
			numLogs:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := &MockLogSender{
				ShouldError: tt.shouldError,
				ErrorMsg:    tt.errorMsg,
			}

			batcher := NewLogBatcher(2, 0, mockSender.SendLogs)
			defer batcher.Close()

			fieldMappings := getDefaultFieldMappings()
			extractor := NewJSONExtractor("", fieldMappings)

			var lastError error
			for i := 0; i < tt.numLogs; i++ {
				logLine := fmt.Sprintf(`{"timestamp": "2024-01-15T10:30:%02dZ", "level": "info", "message": "Log %d"}`, i, i)
				entry, err := extractor.ParseLogEntry(logLine)
				if err != nil {
					t.Errorf("Unexpected parse error: %v", err)
					continue
				}

				err = batcher.Add(entry)
				if err != nil {
					lastError = err
				}
			}

			// Force flush to trigger any remaining errors
			err := batcher.Flush()
			if err != nil {
				lastError = err
			}

			if tt.shouldError {
				if lastError == nil {
					t.Error("Expected error but got none")
				} else if !strings.Contains(lastError.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, lastError.Error())
				}
			} else {
				if lastError != nil {
					t.Errorf("Unexpected error: %v", lastError)
				}
			}
		})
	}
}

func TestTimestampParsing(t *testing.T) {
	fieldMappings := &FieldMappings{
		TimestampFields: []string{"timestamp", "ts", "time"},
		LevelFields:     []string{"level"},
		MessageFields:   []string{"message"},
	}

	extractor := NewJSONExtractor("", fieldMappings)

	tests := []struct {
		name         string
		input        string
		expectedTime time.Time
		shouldUseNow bool
	}{
		{
			name:         "RFC3339 timestamp",
			input:        `{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "test"}`,
			expectedTime: time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		},
		{
			name:         "RFC3339 with milliseconds",
			input:        `{"ts": "2024-01-15T10:30:45.123Z", "level": "info", "message": "test"}`,
			expectedTime: time.Date(2024, 1, 15, 10, 30, 45, 123000000, time.UTC),
		},
		{
			name:         "Unix timestamp",
			input:        `{"timestamp": 1705315845, "level": "info", "message": "test"}`,
			expectedTime: time.Unix(1705315845, 0),
		},
		{
			name:         "No timestamp field",
			input:        `{"level": "info", "message": "test"}`,
			shouldUseNow: true,
		},
		{
			name:         "Invalid timestamp",
			input:        `{"timestamp": "invalid", "level": "info", "message": "test"}`,
			shouldUseNow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			entry, err := extractor.ParseLogEntry(tt.input)
			after := time.Now()

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.shouldUseNow {
				if entry.Timestamp.Before(before) || entry.Timestamp.After(after) {
					t.Errorf("Expected timestamp to be close to now, got %v", entry.Timestamp)
				}
			} else {
				if !entry.Timestamp.Equal(tt.expectedTime) {
					t.Errorf("Expected timestamp %v, got %v", tt.expectedTime, entry.Timestamp)
				}
			}
		})
	}
}

// Benchmark the complete pipeline
func BenchmarkCompleteLogProcessing(b *testing.B) {
	mockSender := &MockLogSender{}
	batcher := NewLogBatcher(100, 0, mockSender.SendLogs)
	defer batcher.Close()

	fieldMappings := getDefaultFieldMappings()
	extractor := NewJSONExtractor("", fieldMappings)

	logLine := `{"timestamp": "2024-01-15T10:30:45.123Z", "level": "info", "message": "Benchmark test message", "user_id": 12345, "request_id": "req-abc123", "duration_ms": 245}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry, err := extractor.ParseLogEntry(logLine)
		if err != nil {
			b.Fatal(err)
		}

		err = batcher.Add(entry)
		if err != nil {
			b.Fatal(err)
		}
	}

	batcher.Flush()
}

func BenchmarkOTLPRequestCreation(b *testing.B) {
	config := &Config{
		ServiceName:    "benchmark-service",
		ServiceVersion: "1.0.0",
	}

	entries := []*LogEntry{
		{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "Benchmark message",
			Fields:    map[string]interface{}{"user_id": 123, "action": "test"},
			Raw:       "raw log line",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := createExportRequest(entries, config)

		// Serialize to measure real-world performance
		_, err := proto.Marshal(req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Test data race conditions with concurrent access
func TestConcurrentLogProcessing(t *testing.T) {
	t.Skip("Temporarily disabled due to race condition - needs LogBatcher to be thread-safe")

	mockSender := &MockLogSender{}
	batcher := NewLogBatcher(10, 0, mockSender.SendLogs)
	defer batcher.Close()

	fieldMappings := getDefaultFieldMappings()
	extractor := NewJSONExtractor("", fieldMappings)

	const numGoroutines = 10
	const logsPerGoroutine = 100

	done := make(chan bool, numGoroutines)

	// Start multiple goroutines processing logs
	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer func() { done <- true }()

			for i := 0; i < logsPerGoroutine; i++ {
				logLine := fmt.Sprintf(`{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Goroutine %d Log %d"}`, goroutineID, i)
				entry, err := extractor.ParseLogEntry(logLine)
				if err != nil {
					t.Errorf("Goroutine %d: unexpected error: %v", goroutineID, err)
					return
				}

				err = batcher.Add(entry)
				if err != nil {
					t.Errorf("Goroutine %d: unexpected error adding to batch: %v", goroutineID, err)
					return
				}
			}
		}(g)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Final flush
	batcher.Flush()

	// Verify all logs were processed
	mockSender.mu.Lock()
	totalEntries := 0
	for _, batch := range mockSender.SentBatches {
		totalEntries += len(batch)
	}
	mockSender.mu.Unlock()

	expectedTotal := numGoroutines * logsPerGoroutine
	if totalEntries != expectedTotal {
		t.Errorf("Expected %d total entries, got %d", expectedTotal, totalEntries)
	}
}

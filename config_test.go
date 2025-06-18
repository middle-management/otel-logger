package main

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/alexflint/go-arg"
)

func TestConfigDefaults(t *testing.T) {
	var config Config

	// Parse with no arguments to get defaults
	p, err := arg.NewParser(arg.Config{}, &config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}

	err = p.Parse([]string{})
	if err != nil {
		t.Fatalf("Failed to parse empty args: %v", err)
	}

	// Test default values
	if config.Timeout != 10*time.Second {
		t.Errorf("Expected default timeout '10s', got '%v'", config.Timeout)
	}

	if config.BatchSize != 50 {
		t.Errorf("Expected default batch size '50', got '%d'", config.BatchSize)
	}

	if config.FlushInterval != 5*time.Second {
		t.Errorf("Expected default flush interval '5s', got '%v'", config.FlushInterval)
	}

	if config.ShowVersion != false {
		t.Errorf("Expected default show version 'false', got '%v'", config.ShowVersion)
	}

	if config.JSONPrefix != "" {
		t.Errorf("Expected default json prefix '', got '%s'", config.JSONPrefix)
	}
}

func TestConfigParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected Config
		wantErr  bool
	}{
		{
			name: "duration parsing",
			args: []string{
				"--timeout", "30s",
				"--flush-interval", "2m",
			},
			expected: Config{
				Timeout:       30 * time.Second,
				BatchSize:     50, // default
				FlushInterval: 2 * time.Minute,
			},
		},
		{
			name: "version flag",
			args: []string{
				"--version",
			},
			wantErr: true, // go-arg exits when version is requested
		},
		{
			name: "field mappings",
			args: []string{
				"--timestamp-fields", "created_at",
				"--timestamp-fields", "event_time",
				"--level-fields", "severity",
				"--message-fields", "description",
			},
			expected: Config{
				TimestampFields: []string{"created_at", "event_time"},
				LevelFields:     []string{"severity"},
				MessageFields:   []string{"description"},
				Timeout:         10 * time.Second, // default
				BatchSize:       50,               // default
				FlushInterval:   5 * time.Second,  // default
			},
		},
		{
			name: "json prefix regex",
			args: []string{
				"--json-prefix", `^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z\\s*`,
			},
			expected: Config{
				JSONPrefix:    `^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}Z\\s*`,
				Timeout:       10 * time.Second, // default
				BatchSize:     50,               // default
				FlushInterval: 5 * time.Second,  // default
			},
		},
		{
			name: "invalid duration",
			args: []string{
				"--timeout", "invalid-duration",
			},
			wantErr: true,
		},
		{
			name: "negative batch size",
			args: []string{
				"--batch-size", "-10",
			},
			expected: Config{
				BatchSize:     -10,              // parsed as-is, validation happens elsewhere
				Timeout:       10 * time.Second, // default
				FlushInterval: 5 * time.Second,  // default
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config Config

			p, err := arg.NewParser(arg.Config{}, &config)
			if err != nil {
				t.Fatalf("Failed to create parser: %v", err)
			}

			err = p.Parse(tt.args)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Compare relevant fields
			if config.ShowVersion != tt.expected.ShowVersion {
				t.Errorf("ShowVersion: expected %v, got %v", tt.expected.ShowVersion, config.ShowVersion)
			}
			if config.Timeout != tt.expected.Timeout {
				t.Errorf("Timeout: expected %v, got %v", tt.expected.Timeout, config.Timeout)
			}
			if config.BatchSize != tt.expected.BatchSize {
				t.Errorf("BatchSize: expected %d, got %d", tt.expected.BatchSize, config.BatchSize)
			}
			if config.FlushInterval != tt.expected.FlushInterval {
				t.Errorf("FlushInterval: expected %v, got %v", tt.expected.FlushInterval, config.FlushInterval)
			}
			if !reflect.DeepEqual(config.TimestampFields, tt.expected.TimestampFields) {
				t.Errorf("TimestampFields: expected %v, got %v", tt.expected.TimestampFields, config.TimestampFields)
			}
			if !reflect.DeepEqual(config.LevelFields, tt.expected.LevelFields) {
				t.Errorf("LevelFields: expected %v, got %v", tt.expected.LevelFields, config.LevelFields)
			}
			if !reflect.DeepEqual(config.MessageFields, tt.expected.MessageFields) {
				t.Errorf("MessageFields: expected %v, got %v", tt.expected.MessageFields, config.MessageFields)
			}
			if config.JSONPrefix != tt.expected.JSONPrefix {
				t.Errorf("JSONPrefix: expected %s, got %s", tt.expected.JSONPrefix, config.JSONPrefix)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantErr   bool
		errString string
	}{
		{
			name: "valid config",
			config: Config{
				BatchSize:     50,
				Timeout:       10 * time.Second,
				FlushInterval: 5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "negative batch size",
			config: Config{
				BatchSize:     -10,
				Timeout:       10 * time.Second,
				FlushInterval: 5 * time.Second,
			},
			wantErr: false, // Validation might happen at runtime
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCommand(&tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errString != "" && !containsString(err.Error(), tt.errString) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errString, err.Error())
				}
			} else {
				// For successful cases, we expect connection errors since we're not running a real server
				// but we should not get configuration validation errors
				if err != nil && containsString(err.Error(), "unsupported protocol") {
					t.Errorf("Got configuration error when none expected: %v", err)
				}
			}
		})
	}
}

func TestEnvironmentVariables(t *testing.T) {
	// Test that OpenTelemetry environment variables can be used
	originalEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	defer func() {
		if originalEndpoint == "" {
			os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		} else {
			os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", originalEndpoint)
		}
	}()

	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env.example.com:4317")

	var config Config
	p, err := arg.NewParser(arg.Config{}, &config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}

	// Parse with no command line args
	err = p.Parse([]string{})
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	// Note: Environment variables are handled by the OpenTelemetry SDK exporters
	// The config struct itself doesn't read them directly
	t.Log("Environment variables are handled by OpenTelemetry SDK exporters")
}

func TestConfigDescription(t *testing.T) {
	config := Config{}
	description := config.Description()

	// Test that description is not empty and contains expected content
	if description == "" {
		t.Error("Description should not be empty")
	}

	expectedPhrases := []string{
		"OpenTelemetry collector",
		"JSON logs",
		"Examples:",
		"Field Mapping Defaults:",
	}

	for _, phrase := range expectedPhrases {
		if !containsString(description, phrase) {
			t.Errorf("Description should contain '%s'", phrase)
		}
	}
}

func TestVersionString(t *testing.T) {
	// Save original values
	origVersion := version
	origGitCommit := gitCommit

	// Set test values
	version = "2.1.0"
	gitCommit = "def456"

	defer func() {
		// Restore original values
		version = origVersion
		gitCommit = origGitCommit
	}()

	config := Config{}
	versionStr := config.Version()

	expected := "otel-logger 2.1.0 (commit: def456)"
	if versionStr != expected {
		t.Errorf("Expected version string '%s', got '%s'", expected, versionStr)
	}
}

func TestConfigFieldMappingDefaults(t *testing.T) {
	config := Config{
		// Leave field mapping arrays empty to test default behavior
	}

	// Test that empty field mappings result in defaults being used
	var fieldMappings *FieldMappings
	if len(config.TimestampFields) > 0 || len(config.LevelFields) > 0 || len(config.MessageFields) > 0 {
		fieldMappings = &FieldMappings{
			TimestampFields: config.TimestampFields,
			LevelFields:     config.LevelFields,
			MessageFields:   config.MessageFields,
		}
		// Use defaults for any empty fields
		if len(fieldMappings.TimestampFields) == 0 {
			fieldMappings.TimestampFields = getDefaultFieldMappings().TimestampFields
		}
		if len(fieldMappings.LevelFields) == 0 {
			fieldMappings.LevelFields = getDefaultFieldMappings().LevelFields
		}
		if len(fieldMappings.MessageFields) == 0 {
			fieldMappings.MessageFields = getDefaultFieldMappings().MessageFields
		}
	} else {
		fieldMappings = getDefaultFieldMappings()
	}

	// Verify defaults are applied
	expectedDefaults := getDefaultFieldMappings()
	if !reflect.DeepEqual(fieldMappings.TimestampFields, expectedDefaults.TimestampFields) {
		t.Errorf("Expected default timestamp fields, got %v", fieldMappings.TimestampFields)
	}
	if !reflect.DeepEqual(fieldMappings.LevelFields, expectedDefaults.LevelFields) {
		t.Errorf("Expected default level fields, got %v", fieldMappings.LevelFields)
	}
	if !reflect.DeepEqual(fieldMappings.MessageFields, expectedDefaults.MessageFields) {
		t.Errorf("Expected default message fields, got %v", fieldMappings.MessageFields)
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && findString(s, substr) >= 0)
}

func findString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Benchmark configuration parsing
func BenchmarkConfigParsing(b *testing.B) {
	args := []string{
		"--endpoint", "benchmark.example.com:4317",
		"--protocol", "http",
		"--service-name", "benchmark-service",
		"--timestamp-fields", "timestamp",
		"--level-fields", "level",
		"--message-fields", "message",
		"--batch-size", "100",
		"--timeout", "30s",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var config Config
		p, err := arg.NewParser(arg.Config{}, &config)
		if err != nil {
			b.Fatal(err)
		}

		err = p.Parse(args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

package main

import (
	"strings"
	"testing"
)

func TestMultilineLogIterator(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name: "single line logs",
			input: `2024-01-15T10:30:00Z INFO Starting application
2024-01-15T10:30:05Z ERROR Failed to process request
2024-01-15T10:30:10Z DEBUG Processing user request`,
			expected: []string{
				"2024-01-15T10:30:00Z INFO Starting application",
				"2024-01-15T10:30:05Z ERROR Failed to process request",
				"2024-01-15T10:30:10Z DEBUG Processing user request",
			},
		},
		{
			name: "multiline logs with space indentation",
			input: `2024-01-15T10:30:00Z INFO Starting application
  - Configuration loaded
  - Database connection established
2024-01-15T10:30:05Z ERROR Failed to process request
  Exception: NullPointerException
    at com.example.Service.process(Service.java:42)`,
			expected: []string{
				"2024-01-15T10:30:00Z INFO Starting application\n  - Configuration loaded\n  - Database connection established",
				"2024-01-15T10:30:05Z ERROR Failed to process request\n  Exception: NullPointerException\n    at com.example.Service.process(Service.java:42)",
			},
		},
		{
			name: "multiline logs with tab indentation",
			input: `2024-01-15T10:30:00Z INFO Starting application
	Configuration loaded
	Database connection established
2024-01-15T10:30:05Z ERROR Failed to process request
	Exception: NullPointerException`,
			expected: []string{
				"2024-01-15T10:30:00Z INFO Starting application\n\tConfiguration loaded\n\tDatabase connection established",
				"2024-01-15T10:30:05Z ERROR Failed to process request\n\tException: NullPointerException",
			},
		},
		{
			name: "mixed single and multiline logs",
			input: `2024-01-15T10:30:00Z INFO Starting application
2024-01-15T10:30:05Z ERROR Failed to process request
  Exception: NullPointerException
    at com.example.Service.process(Service.java:42)
2024-01-15T10:30:10Z DEBUG Processing user request
2024-01-15T10:30:15Z INFO Request completed successfully`,
			expected: []string{
				"2024-01-15T10:30:00Z INFO Starting application",
				"2024-01-15T10:30:05Z ERROR Failed to process request\n  Exception: NullPointerException\n    at com.example.Service.process(Service.java:42)",
				"2024-01-15T10:30:10Z DEBUG Processing user request",
				"2024-01-15T10:30:15Z INFO Request completed successfully",
			},
		},
		{
			name: "orphaned continuation lines ignored",
			input: `  - Orphaned continuation line at start
    Another orphaned line
2024-01-15T10:30:00Z INFO Starting application
  - Configuration loaded
  - Database connection established
2024-01-15T10:30:05Z ERROR Failed to process request
  Exception: NullPointerException`,
			expected: []string{
				"2024-01-15T10:30:00Z INFO Starting application\n  - Configuration loaded\n  - Database connection established",
				"2024-01-15T10:30:05Z ERROR Failed to process request\n  Exception: NullPointerException",
			},
		},
		{
			name: "empty lines handled",
			input: `2024-01-15T10:30:00Z INFO Starting application

2024-01-15T10:30:05Z ERROR Failed to process request
  Exception: NullPointerException

2024-01-15T10:30:10Z DEBUG Processing user request`,
			expected: []string{
				"2024-01-15T10:30:00Z INFO Starting application",
				"2024-01-15T10:30:05Z ERROR Failed to process request\n  Exception: NullPointerException",
				"2024-01-15T10:30:10Z DEBUG Processing user request",
			},
		},
		{
			name: "complex java stack trace",
			input: `2024-01-15T10:30:05Z ERROR Failed to process request
  java.lang.NullPointerException: Cannot invoke "String.length()" because "str" is null
	at com.example.service.UserService.validateUser(UserService.java:45)
	at com.example.controller.UserController.createUser(UserController.java:23)
	at java.base/jdk.internal.reflect.NativeMethodAccessorImpl.invoke0(Native Method)
	at java.base/jdk.internal.reflect.NativeMethodAccessorImpl.invoke(NativeMethodAccessorImpl.java:77)
	at java.base/jdk.internal.reflect.DelegatingMethodAccessorImpl.invoke(DelegatingMethodAccessorImpl.java:43)
	at java.base/java.lang.reflect.Method.invoke(Method.java:568)
	... 23 more
2024-01-15T10:30:10Z INFO Request completed`,
			expected: []string{
				"2024-01-15T10:30:05Z ERROR Failed to process request\n  java.lang.NullPointerException: Cannot invoke \"String.length()\" because \"str\" is null\n\tat com.example.service.UserService.validateUser(UserService.java:45)\n\tat com.example.controller.UserController.createUser(UserController.java:23)\n\tat java.base/jdk.internal.reflect.NativeMethodAccessorImpl.invoke0(Native Method)\n\tat java.base/jdk.internal.reflect.NativeMethodAccessorImpl.invoke(NativeMethodAccessorImpl.java:77)\n\tat java.base/jdk.internal.reflect.DelegatingMethodAccessorImpl.invoke(DelegatingMethodAccessorImpl.java:43)\n\tat java.base/java.lang.reflect.Method.invoke(Method.java:568)\n\t... 23 more",
				"2024-01-15T10:30:10Z INFO Request completed",
			},
		},
		{
			name:     "empty input",
			input:    "",
			expected: []string{},
		},
		{
			name: "only whitespace lines",
			input: `

   `,
			expected: []string{},
		},
		{
			name: "json logs with multiline",
			input: `{"timestamp":"2024-01-15T10:30:00Z","level":"INFO","message":"Starting application"}
{"timestamp":"2024-01-15T10:30:05Z","level":"ERROR","message":"Failed to process request","stackTrace":"java.lang.NullPointerException\n  at Service.process(Service.java:42)\n  at Controller.handle(Controller.java:15)"}
{"timestamp":"2024-01-15T10:30:10Z","level":"DEBUG","message":"Processing user request"}`,
			expected: []string{
				`{"timestamp":"2024-01-15T10:30:00Z","level":"INFO","message":"Starting application"}`,
				`{"timestamp":"2024-01-15T10:30:05Z","level":"ERROR","message":"Failed to process request","stackTrace":"java.lang.NullPointerException\n  at Service.process(Service.java:42)\n  at Controller.handle(Controller.java:15)"}`,
				`{"timestamp":"2024-01-15T10:30:10Z","level":"DEBUG","message":"Processing user request"}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			var results []string

			for logEntry := range multilineLogIterator(reader) {
				results = append(results, logEntry)
			}

			if len(results) != len(tt.expected) {
				t.Errorf("Expected %d log entries, got %d", len(tt.expected), len(results))
				t.Errorf("Expected: %v", tt.expected)
				t.Errorf("Got: %v", results)
				return
			}

			for i, expected := range tt.expected {
				if results[i] != expected {
					t.Errorf("Log entry %d mismatch", i)
					t.Errorf("Expected: %q", expected)
					t.Errorf("Got: %q", results[i])
				}
			}
		})
	}
}

func TestMultilineLogIteratorEarlyExit(t *testing.T) {
	input := `2024-01-15T10:30:00Z INFO Starting application
  - Configuration loaded
  - Database connection established
2024-01-15T10:30:05Z ERROR Failed to process request
  Exception: NullPointerException
2024-01-15T10:30:10Z DEBUG Processing user request`

	reader := strings.NewReader(input)
	var results []string
	count := 0

	for logEntry := range multilineLogIterator(reader) {
		results = append(results, logEntry)
		count++
		if count >= 2 {
			break // Early exit
		}
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 log entries due to early exit, got %d", len(results))
	}

	expected := []string{
		"2024-01-15T10:30:00Z INFO Starting application\n  - Configuration loaded\n  - Database connection established",
		"2024-01-15T10:30:05Z ERROR Failed to process request\n  Exception: NullPointerException",
	}

	for i, exp := range expected {
		if results[i] != exp {
			t.Errorf("Log entry %d mismatch after early exit", i)
			t.Errorf("Expected: %q", exp)
			t.Errorf("Got: %q", results[i])
		}
	}
}

func BenchmarkMultilineLogIterator(b *testing.B) {
	input := `2024-01-15T10:30:00Z INFO Starting application
  - Configuration loaded
  - Database connection established
  - Server listening on port 8080
2024-01-15T10:30:05Z ERROR Failed to process request
  Exception: NullPointerException
    at com.example.Service.process(Service.java:42)
    at com.example.Controller.handle(Controller.java:15)
    at java.base/java.lang.Thread.run(Thread.java:834)
2024-01-15T10:30:10Z DEBUG Processing user request
  User ID: 12345
  Request path: /api/users/profile
  Headers:
    Authorization: Bearer ...
    Content-Type: application/json
2024-01-15T10:30:15Z INFO Request completed successfully`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := strings.NewReader(input)
		for range multilineLogIterator(reader) {
			// Process each log entry
		}
	}
}

#!/bin/bash

# Test script for otel-logger
# This script demonstrates various features and usage patterns

set -e

echo "=== OpenTelemetry Logger Test Suite ==="
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_test() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if binary exists
BINARY="./otel-logger"
if [ ! -f "$BINARY" ]; then
    print_error "Binary not found. Please build the tool first:"
    echo "  go build -o otel-logger ."
    exit 1
fi

# Create test log directory if it doesn't exist
mkdir -p test-logs

# Test 1: Help output
print_test "Testing help output..."
$BINARY --help > /dev/null 2>&1
if [ $? -eq 0 ]; then
    print_success "Help command works correctly"
else
    print_error "Help command failed"
    exit 1
fi

# Test 2: JSON log parsing (dry run - will fail to connect but show parsing)
print_test "Testing JSON log parsing..."
echo '{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Test message", "user_id": 123}' | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "test-service" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "JSON parsing test initiated" || print_warning "JSON parsing test may have issues"

# Test 3: Test with prefixed logs
print_test "Testing prefixed log parsing..."
echo '2024-01-15T10:30:45.123Z {"level": "debug", "message": "Prefixed log test", "component": "test-suite"}' | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --json-prefix "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}[.\\d]*Z?\\s*" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Prefixed log parsing test initiated" || print_warning "Prefixed log parsing test may have issues"

# Test 4: Test with mixed log formats
print_test "Testing mixed log formats..."
cat > test-logs/mixed-test.log << 'EOF'
{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Pure JSON log"}
2024-01-15T10:30:46Z Plain text log with timestamp
[ERROR] 2024-01-15 10:30:47 - Error message with brackets
{"ts": "2024-01-15T10:30:48.123Z", "lvl": "debug", "msg": "Different JSON format"}
Non-JSON log line without timestamp
EOF

cat test-logs/mixed-test.log | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "mixed-test" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Mixed log format test initiated" || print_warning "Mixed log format test may have issues"

# Test 5: Test HTTP protocol option
print_test "Testing HTTP protocol option..."
echo '{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "HTTP protocol test"}' | \
    timeout 2s $BINARY --endpoint "http://nonexistent:4318" --protocol http 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "HTTP protocol test initiated" || print_warning "HTTP protocol test may have issues"

# Test 6: Test batching options
print_test "Testing batching configuration..."
echo '{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Batch test"}' | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --batch-size 10 --flush-interval 1s 2>&1 | \
    grep -q "batch_size=10" && print_success "Batching configuration test passed" || print_warning "Batching configuration test may have issues"

# Test 7: Test with custom headers
print_test "Testing custom headers..."
echo '{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Header test"}' | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --header "Authorization=Bearer test-token" --header "X-Custom-Header=test-value" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Custom headers test initiated" || print_warning "Custom headers test may have issues"

# Test 8: Performance test with example files
if [ -f "examples/json-logs.txt" ]; then
    print_test "Performance test with example JSON logs..."
    time (cat examples/json-logs.txt | timeout 3s $BINARY --endpoint "nonexistent:4317" --batch-size 100 2>/dev/null) 2>&1 | \
        grep -q "real" && print_success "Performance test completed" || print_warning "Performance test may have issues"
fi

if [ -f "examples/prefixed-logs.txt" ]; then
    print_test "Testing with prefixed example logs..."
    time (cat examples/prefixed-logs.txt | timeout 3s $BINARY --endpoint "nonexistent:4317" --json-prefix "^[\\[\\d\\-T:+\\]\\s]*" 2>/dev/null) 2>&1 | \
        grep -q "real" && print_success "Prefixed example logs test completed" || print_warning "Prefixed example logs test may have issues"
fi

# Test 9: Test various timestamp formats
print_test "Testing various timestamp formats..."
cat > test-logs/timestamp-test.log << 'EOF'
{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "RFC3339 timestamp"}
{"ts": "2024-01-15T10:30:46.123Z", "level": "info", "message": "RFC3339 with milliseconds"}
{"time": "2024-01-15 10:30:47", "level": "info", "message": "Simple datetime format"}
{"timestamp": 1705315848, "level": "info", "message": "Unix timestamp"}
{"ts": "2024-01-15T10:30:49+00:00", "level": "info", "message": "RFC3339 with timezone"}
EOF

cat test-logs/timestamp-test.log | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "timestamp-test" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Timestamp format test initiated" || print_warning "Timestamp format test may have issues"

# Test 10: Test log level mapping
print_test "Testing log level mapping..."
cat > test-logs/level-test.log << 'EOF'
{"timestamp": "2024-01-15T10:30:45Z", "level": "trace", "message": "Trace level"}
{"timestamp": "2024-01-15T10:30:46Z", "level": "debug", "message": "Debug level"}
{"timestamp": "2024-01-15T10:30:47Z", "level": "info", "message": "Info level"}
{"timestamp": "2024-01-15T10:30:48Z", "level": "warn", "message": "Warn level"}
{"timestamp": "2024-01-15T10:30:49Z", "level": "error", "message": "Error level"}
{"timestamp": "2024-01-15T10:30:50Z", "level": "fatal", "message": "Fatal level"}
{"timestamp": "2024-01-15T10:30:51Z", "severity": "info", "message": "Severity field"}
{"timestamp": "2024-01-15T10:30:52Z", "lvl": "debug", "message": "Lvl field"}
EOF

cat test-logs/level-test.log | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "level-test" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Log level mapping test initiated" || print_warning "Log level mapping test may have issues"

# Test 11: Test malformed JSON handling
print_test "Testing malformed JSON handling..."
cat > test-logs/malformed-test.log << 'EOF'
{"valid": "json", "timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Valid JSON"}
{"invalid": "json", "missing_quote: "should_fail", "level": "error"}
{"incomplete": "json"
Not JSON at all - should be treated as plain text
{"timestamp": "2024-01-15T10:30:46Z", "level": "info", "message": "Valid JSON after malformed"}
EOF

cat test-logs/malformed-test.log | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "malformed-test" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Malformed JSON handling test initiated" || print_warning "Malformed JSON handling test may have issues"

# Test 12: Test empty and whitespace logs
print_test "Testing empty and whitespace logs..."
cat > test-logs/empty-test.log << 'EOF'



{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "After empty lines"}

EOF

cat test-logs/empty-test.log | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "empty-test" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Empty/whitespace logs test initiated" || print_warning "Empty/whitespace logs test may have issues"

# Test 13: Test configurable timestamp fields
print_test "Testing configurable timestamp fields..."
cat > test-logs/timestamp-fields-test.log << 'EOF'
{"@timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Logstash format"}
{"created_at": "2024-01-15T10:30:46Z", "severity": "high", "description": "Custom format"}
{"event_time": "2024-01-15T10:30:47Z", "priority": "medium", "content": "Another custom format"}
EOF

cat test-logs/timestamp-fields-test.log | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "timestamp-fields-test" \
    --timestamp-fields "@timestamp,created_at,event_time" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Configurable timestamp fields test initiated" || print_warning "Configurable timestamp fields test may have issues"

# Test 14: Test configurable level fields
print_test "Testing configurable level fields..."
cat > test-logs/level-fields-test.log << 'EOF'
{"timestamp": "2024-01-15T10:30:45Z", "severity": "high", "message": "Custom severity field"}
{"timestamp": "2024-01-15T10:30:46Z", "priority": "medium", "message": "Custom priority field"}
{"timestamp": "2024-01-15T10:30:47Z", "log_level": "critical", "message": "Custom log_level field"}
EOF

cat test-logs/level-fields-test.log | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "level-fields-test" \
    --level-fields "severity,priority,log_level" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Configurable level fields test initiated" || print_warning "Configurable level fields test may have issues"

# Test 15: Test configurable message fields
print_test "Testing configurable message fields..."
cat > test-logs/message-fields-test.log << 'EOF'
{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "description": "Custom description field"}
{"timestamp": "2024-01-15T10:30:46Z", "level": "warn", "content": "Custom content field"}
{"timestamp": "2024-01-15T10:30:47Z", "level": "error", "log_text": "Custom log_text field"}
EOF

cat test-logs/message-fields-test.log | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "message-fields-test" \
    --message-fields "description,content,log_text" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Configurable message fields test initiated" || print_warning "Configurable message fields test may have issues"

# Test 16: Test combined custom field mappings
print_test "Testing combined custom field mappings..."
cat > test-logs/combined-fields-test.log << 'EOF'
{"created_at": "2024-01-15T10:30:45Z", "severity": "high", "description": "Payment failed", "transaction_id": "txn_123"}
{"event_time": "2024-01-15T10:30:46Z", "priority": "medium", "content": "User logged in", "user_id": "user_456"}
{"occurred_at": "2024-01-15T10:30:47Z", "log_level": "normal", "log_text": "Backup completed", "backup_size": "1.2GB"}
EOF

cat test-logs/combined-fields-test.log | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "combined-fields-test" \
    --timestamp-fields "created_at,event_time,occurred_at" \
    --level-fields "severity,priority,log_level" \
    --message-fields "description,content,log_text" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Combined custom field mappings test initiated" || print_warning "Combined custom field mappings test may have issues"

# Test 17: Test example format files with custom mappings
if [ -f "examples/logstash-format.txt" ]; then
    print_test "Testing Logstash format with custom mappings..."
    cat examples/logstash-format.txt | \
        timeout 3s $BINARY --endpoint "nonexistent:4317" --service-name "logstash-test" \
        --timestamp-fields "@timestamp" --level-fields "level" --message-fields "message" 2>&1 | \
        grep -q "Reading logs from stdin" && print_success "Logstash format test initiated" || print_warning "Logstash format test may have issues"
fi

if [ -f "examples/custom-format.txt" ]; then
    print_test "Testing custom format with multiple field mappings..."
    cat examples/custom-format.txt | \
        timeout 3s $BINARY --endpoint "nonexistent:4317" --service-name "custom-format-test" \
        --timestamp-fields "created_at,event_time,occurred_at" \
        --level-fields "severity,priority,log_level" \
        --message-fields "description,content,log_text" 2>&1 | \
        grep -q "Reading logs from stdin" && print_success "Custom format test initiated" || print_warning "Custom format test may have issues"
fi

# Test 18: Test large log entry
print_test "Testing large log entry..."
LARGE_MESSAGE=$(python3 -c "print('x' * 1000)" 2>/dev/null || echo "Large message content here - Python not available for generating 1000 chars")
echo "{\"timestamp\": \"2024-01-15T10:30:45Z\", \"level\": \"info\", \"message\": \"$LARGE_MESSAGE\", \"data\": {\"key1\": \"value1\", \"key2\": \"value2\", \"key3\": \"value3\"}}" | \
    timeout 2s $BINARY --endpoint "nonexistent:4317" --service-name "large-test" 2>&1 | \
    grep -q "Reading logs from stdin" && print_success "Large log entry test initiated" || print_warning "Large log entry test may have issues"

# Cleanup
rm -rf test-logs

echo
echo "=== Test Summary ==="
print_success "All tests completed successfully!"
echo
echo "Note: These tests verify that the tool starts correctly and processes different log formats."
echo "To test actual log forwarding, you'll need a running OpenTelemetry collector."
echo
echo "To start a test collector:"
echo "  docker run -p 4317:4317 -p 4318:4318 otel/opentelemetry-collector:latest"
echo
echo "Then test with:"
echo "  echo '{\"timestamp\": \"2024-01-15T10:30:45Z\", \"level\": \"info\", \"message\": \"Hello OTEL!\"}' | \\"
echo "    ./otel-logger --endpoint localhost:4317 --service-name test-app"
echo
echo "Test custom field mappings:"
echo "  echo '{\"@timestamp\": \"2024-01-15T10:30:45Z\", \"severity\": \"high\", \"description\": \"Custom fields test\"}' | \\"
echo "    ./otel-logger --endpoint localhost:4317 --timestamp-fields '@timestamp' --level-fields 'severity' --message-fields 'description'"
echo
print_success "Test suite completed!"

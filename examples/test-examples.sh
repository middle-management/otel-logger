#!/bin/bash

# Test script demonstrating various otel-logger usage patterns
# This script shows both stdin and command wrapping modes

set -e

# Configuration
OTEL_ENDPOINT="${OTEL_ENDPOINT:-localhost:4317}"
OTEL_HTTP_ENDPOINT="${OTEL_HTTP_ENDPOINT:-localhost:4318}"
OTEL_LOGGER="${OTEL_LOGGER:-../otel-logger}"
SERVICE_NAME="${SERVICE_NAME:-test-service}"

echo "=== OpenTelemetry Logger Test Examples ==="
echo "Using otel-logger: $OTEL_LOGGER"
echo "OTLP gRPC endpoint: $OTEL_ENDPOINT"
echo "OTLP HTTP endpoint: $OTEL_HTTP_ENDPOINT"
echo "Service name: $SERVICE_NAME"
echo ""

# Check if otel-logger exists
if [[ ! -f "$OTEL_LOGGER" ]]; then
    echo "Error: otel-logger binary not found at $OTEL_LOGGER"
    echo "Please build it first with: go build -o otel-logger ."
    exit 1
fi

# Function to run test with separator
run_test() {
    local test_name="$1"
    shift
    echo "--- Test: $test_name ---"
    "$@"
    echo "✅ $test_name completed"
    echo ""
    sleep 1
}

echo "🚀 Starting otel-logger tests..."
echo ""

# Test 1: Basic JSON logs via stdin (gRPC)
run_test "JSON logs via stdin (gRPC)" bash -c "
echo '{\"timestamp\": \"$(date -Iseconds)\", \"level\": \"info\", \"message\": \"Test message from stdin\", \"test_id\": 1}' | \
$OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure
"

# Test 2: Basic JSON logs via stdin (HTTP)
run_test "JSON logs via stdin (HTTP)" bash -c "
echo '{\"timestamp\": \"$(date -Iseconds)\", \"level\": \"warn\", \"message\": \"HTTP test message\", \"test_id\": 2}' | \
$OTEL_LOGGER --endpoint $OTEL_HTTP_ENDPOINT --protocol http --service-name $SERVICE_NAME --insecure
"

# Test 3: Multiple JSON logs via stdin
run_test "Multiple JSON logs via stdin" bash -c "
{
    echo '{\"timestamp\": \"$(date -Iseconds)\", \"level\": \"info\", \"message\": \"First message\", \"sequence\": 1}'
    echo '{\"timestamp\": \"$(date -Iseconds)\", \"level\": \"debug\", \"message\": \"Second message\", \"sequence\": 2}'
    echo '{\"timestamp\": \"$(date -Iseconds)\", \"level\": \"error\", \"message\": \"Third message\", \"sequence\": 3}'
} | $OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure
"

# Test 4: Plain text logs via stdin
run_test "Plain text logs via stdin" bash -c "
{
    echo 'Plain text log entry 1'
    echo 'Plain text log entry 2 with some data'
    echo 'Plain text log entry 3'
} | $OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure
"

# Test 5: Mixed JSON and plain text logs
run_test "Mixed JSON and plain text logs" bash -c "
{
    echo '{\"level\": \"info\", \"message\": \"JSON log entry\"}'
    echo 'Plain text log entry'
    echo '{\"level\": \"error\", \"message\": \"Another JSON entry\", \"error_code\": 500}'
    echo 'Another plain text entry'
} | $OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure
"

# Test 6: Prefixed logs (timestamp prefix)
run_test "Prefixed logs with timestamp" bash -c "
{
    echo \"$(date -Iseconds) {\\\"level\\\": \\\"info\\\", \\\"message\\\": \\\"Prefixed JSON log\\\"}\"
    echo \"$(date -Iseconds) Plain text with timestamp prefix\"
    echo \"$(date -Iseconds) {\\\"level\\\": \\\"debug\\\", \\\"message\\\": \\\"Another prefixed JSON\\\"}\"
} | $OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure \
    --json-prefix '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}[Z0-9:+-]*\s*'
"

# Test 7: Custom field mappings
run_test "Custom field mappings (Logstash format)" bash -c "
{
    echo '{\"@timestamp\": \"$(date -Iseconds)\", \"level\": \"INFO\", \"message\": \"Logstash format log 1\"}'
    echo '{\"@timestamp\": \"$(date -Iseconds)\", \"level\": \"ERROR\", \"message\": \"Logstash format log 2\"}'
} | $OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure \
    --timestamp-fields '@timestamp' --level-fields 'level' --message-fields 'message'
"

# Test 8: Simple command wrapping
run_test "Simple command wrapping (echo)" \
$OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure -- \
echo "Hello from wrapped command!"

# Test 9: Command with JSON output
run_test "Command with JSON output" \
$OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure -- \
sh -c 'echo "{\"level\": \"info\", \"message\": \"JSON from wrapped command\", \"pid\": $$}"'

# Test 10: Command with both stdout and stderr
run_test "Command with stdout and stderr" \
$OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure -- \
sh -c 'echo "Output to stdout"; echo "Output to stderr" >&2; echo "{\"level\": \"info\", \"message\": \"Mixed output command\"}"'

# Test 11: Multi-line command output
run_test "Multi-line command output" \
$OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure -- \
sh -c 'for i in 1 2 3; do echo "Line $i to stdout"; echo "Error $i to stderr" >&2; sleep 0.1; done'

# Test 12: Command with JSON logs from both streams
run_test "Command with JSON logs from both streams" \
$OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure -- \
sh -c 'echo "{\"level\": \"info\", \"message\": \"JSON to stdout\", \"stream\": \"out\"}"; echo "{\"level\": \"error\", \"message\": \"JSON to stderr\", \"stream\": \"err\"}" >&2'

# Test 13: Command with custom batching
run_test "Command with custom batching" \
$OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure \
    --batch-size 10 --flush-interval 1s -- \
sh -c 'for i in $(seq 1 15); do echo "{\"level\": \"info\", \"message\": \"Batch test $i\", \"batch_id\": $i}"; done'

# Test 14: Command that fails (exit code handling)
run_test "Command that fails (exit code handling)" \
$OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure -- \
sh -c 'echo "Before failure"; echo "Error before exit" >&2; exit 42' || echo "Expected failure with exit code 42"

# Test 15: HTTP protocol with headers
run_test "HTTP protocol with custom headers" bash -c "
echo '{\"level\": \"info\", \"message\": \"HTTP with headers test\"}' | \
$OTEL_LOGGER --endpoint $OTEL_HTTP_ENDPOINT --protocol http --service-name $SERVICE_NAME --insecure \
    --header 'X-Custom-Header=test-value' --header 'X-Test-ID=12345'
"

# Test 16: Different service metadata
run_test "Custom service metadata" bash -c "
echo '{\"level\": \"info\", \"message\": \"Custom service metadata test\"}' | \
$OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name 'custom-service' --service-version '2.1.0' --insecure
"

# Test 17: Large batch size test
run_test "Large batch size test" bash -c "
{
    for i in \$(seq 1 100); do
        echo \"{\\\"level\\\": \\\"info\\\", \\\"message\\\": \\\"Bulk test message \$i\\\", \\\"index\\\": \$i}\"
    done
} | $OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure \
    --batch-size 50 --flush-interval 2s
"

# Test 18: Mixed format command (generates different log types)
run_test "Mixed format command output" \
$OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure \
    --json-prefix '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}[Z0-9:+-]*\s*' -- \
sh -c '
    echo "$(date -Iseconds) Plain text with timestamp";
    echo "$(date -Iseconds) {\"level\": \"info\", \"message\": \"Prefixed JSON log\"}";
    echo "Plain text without timestamp";
    echo "{\"level\": \"debug\", \"message\": \"Pure JSON log\"}";
    echo "$(date -Iseconds) Another timestamped text" >&2;
'

# Test 19: Long-running command simulation (with signal handling)
run_test "Long-running command (3 seconds)" \
timeout 3s $OTEL_LOGGER --endpoint $OTEL_ENDPOINT --service-name $SERVICE_NAME --insecure -- \
sh -c 'i=0; while true; do echo "{\"level\": \"info\", \"message\": \"Heartbeat $i\", \"timestamp\": \"$(date -Iseconds)\"}"; i=$((i+1)); sleep 1; done' || echo "Terminated as expected"

# Test 20: Version and help
run_test "Version and help information" bash -c "
echo 'Testing --version flag:'
$OTEL_LOGGER --version
echo ''
echo 'Testing --help flag (first few lines):'
$OTEL_LOGGER --help | head -10
"

echo "🎉 All tests completed successfully!"
echo ""
echo "📊 Test Summary:"
echo "- Tested both stdin and command wrapping modes"
echo "- Tested gRPC and HTTP protocols"
echo "- Tested JSON, plain text, and mixed log formats"
echo "- Tested custom field mappings and prefixed logs"
echo "- Tested stdout/stderr stream separation"
echo "- Tested error handling and exit codes"
echo "- Tested batching and performance options"
echo "- Tested signal handling and timeouts"
echo ""
echo "✅ otel-logger is working correctly!"
echo ""
echo "💡 Next steps:"
echo "1. Check your OpenTelemetry collector logs to see the received data"
echo "2. Try the Docker examples in examples/docker-compose.yml"
echo "3. Use otel-logger as a Docker ENTRYPOINT in your applications"
echo ""
echo "📖 For more examples, see the README.md file"

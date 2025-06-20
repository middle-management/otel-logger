#!/bin/bash

# Test script for multiline log handling

set -e

echo "Testing multiline log handling..."

# Create a test command that outputs multiline logs
cat << 'EOF' > test_multiline_logs.sh
#!/bin/bash

echo "2024-01-15T10:30:00Z INFO Starting application"
echo "  - Configuration loaded"
echo "  - Database connection established"
echo "  - Server listening on port 8080"

echo "2024-01-15T10:30:05Z ERROR Failed to process request"
echo "  Exception: NullPointerException"
echo "    at com.example.Service.process(Service.java:42)"
echo "    at com.example.Controller.handle(Controller.java:15)"
echo "    at java.base/java.lang.Thread.run(Thread.java:834)"

echo "2024-01-15T10:30:10Z DEBUG Processing user request"
echo "  User ID: 12345"
echo "  Request path: /api/users/profile"
echo "  Headers:"
echo "    Authorization: Bearer ..."
echo "    Content-Type: application/json"

echo "2024-01-15T10:30:15Z INFO Request completed successfully"
EOF

chmod +x test_multiline_logs.sh

echo "Generated test logs:"
echo "==================="
./test_multiline_logs.sh
echo ""

echo "Processing with otel-logger (showing passthrough output):"
echo "========================================================"

# Test with passthrough enabled to see the recombined logs
OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317" \
OTEL_EXPORTER_OTLP_INSECURE=true \
./test-otel-logger \
  --passthrough-stdout \
  --timeout 5s \
  ./test_multiline_logs.sh

echo ""
echo "Test completed!"

# Clean up
rm -f test_multiline_logs.sh

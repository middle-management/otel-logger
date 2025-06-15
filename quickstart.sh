#!/bin/bash

# Quick Start Script for OpenTelemetry Logger Testing
# This script sets up a complete observability stack and demonstrates the otel-logger tool

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

print_header() {
    echo -e "${CYAN}===========================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}===========================================${NC}"
}

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
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

# Check if Docker is installed and running
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker first."
        exit 1
    fi

    if ! docker info &> /dev/null; then
        print_error "Docker is not running. Please start Docker first."
        exit 1
    fi

    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        print_error "Docker Compose is not available. Please install Docker Compose."
        exit 1
    fi
}

# Build the otel-logger if it doesn't exist
build_logger() {
    if [ ! -f "./otel-logger" ]; then
        print_info "Building otel-logger..."
        go build -o otel-logger .
        if [ $? -eq 0 ]; then
            print_success "otel-logger built successfully"
        else
            print_error "Failed to build otel-logger"
            exit 1
        fi
    else
        print_info "otel-logger binary already exists"
    fi
}

# Start Docker services
start_services() {
    print_info "Starting OpenTelemetry observability stack..."

    # Use docker compose if available, otherwise fall back to docker-compose
    if docker compose version &> /dev/null; then
        COMPOSE_CMD="docker compose"
    else
        COMPOSE_CMD="docker-compose"
    fi

    $COMPOSE_CMD up -d otel-collector jaeger prometheus grafana elasticsearch loki

    print_info "Waiting for services to start up..."
    sleep 15

    # Check if key services are running
    if docker ps | grep -q otel-collector; then
        print_success "OpenTelemetry Collector is running"
    else
        print_warning "OpenTelemetry Collector may not be running properly"
    fi
}

# Wait for services to be ready
wait_for_services() {
    print_info "Waiting for services to be ready..."

    # Wait for OTEL Collector
    for i in {1..30}; do
        if curl -s http://localhost:4318 > /dev/null 2>&1; then
            print_success "OpenTelemetry Collector HTTP endpoint is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            print_warning "OpenTelemetry Collector HTTP endpoint may not be ready"
        fi
        sleep 2
    done

    # Wait for gRPC endpoint
    for i in {1..30}; do
        if timeout 1 bash -c 'cat < /dev/null > /dev/tcp/localhost/4317' 2>/dev/null; then
            print_success "OpenTelemetry Collector gRPC endpoint is ready"
            break
        fi
        if [ $i -eq 30 ]; then
            print_warning "OpenTelemetry Collector gRPC endpoint may not be ready"
        fi
        sleep 2
    done
}

# Run demonstration tests
run_demo() {
    print_header "Running OpenTelemetry Logger Demonstrations"

    # Demo 1: Basic JSON logs via gRPC
    print_info "Demo 1: Sending JSON logs via gRPC..."
    echo '{"timestamp": "2024-01-15T10:30:45Z", "level": "info", "message": "Hello from otel-logger!", "service": "demo-app", "user_id": 12345}' | \
        ./otel-logger --endpoint localhost:4317 --service-name quickstart-demo --service-version 1.0.0
    print_success "JSON logs sent via gRPC"

    sleep 2

    # Demo 2: JSON logs via HTTP
    print_info "Demo 2: Sending JSON logs via HTTP..."
    echo '{"timestamp": "2024-01-15T10:30:46Z", "level": "warn", "message": "HTTP protocol test", "component": "web-server", "response_time_ms": 250}' | \
        ./otel-logger --endpoint http://localhost:4318 --protocol http --service-name http-demo
    print_success "JSON logs sent via HTTP"

    sleep 2

    # Demo 3: Multiple log levels
    print_info "Demo 3: Sending logs with different severity levels..."
    cat << 'EOF' | ./otel-logger --endpoint localhost:4317 --service-name level-demo --batch-size 10
{"timestamp": "2024-01-15T10:30:47Z", "level": "trace", "message": "Trace level message", "function": "calculateTotal"}
{"timestamp": "2024-01-15T10:30:48Z", "level": "debug", "message": "Debug information", "variable": "userCount", "value": 42}
{"timestamp": "2024-01-15T10:30:49Z", "level": "info", "message": "Application started successfully", "port": 8080}
{"timestamp": "2024-01-15T10:30:50Z", "level": "warn", "message": "High memory usage detected", "memory_usage": "85%"}
{"timestamp": "2024-01-15T10:30:51Z", "level": "error", "message": "Database connection failed", "retry_count": 3}
{"timestamp": "2024-01-15T10:30:52Z", "level": "fatal", "message": "Critical system failure", "exit_code": 1}
EOF
    print_success "Multi-level logs sent"

    sleep 2

    # Demo 4: Prefixed logs
    print_info "Demo 4: Sending logs with timestamp prefixes..."
    cat << 'EOF' | ./otel-logger --endpoint localhost:4317 --service-name prefixed-demo --json-prefix "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}[.\\d]*Z?\\s*"
2024-01-15T10:30:53.123Z {"level": "info", "message": "Prefixed log entry", "request_id": "req-abc123"}
2024-01-15T10:30:54.456Z {"level": "debug", "message": "Processing user request", "user_id": 789, "action": "login"}
2024-01-15T10:30:55.789Z {"level": "error", "message": "Authentication failed", "reason": "invalid_token", "ip": "192.168.1.100"}
EOF
    print_success "Prefixed logs sent"

    sleep 2

    # Demo 5: Configurable field mappings - Logstash format
    print_info "Demo 5: Sending Logstash format logs with custom field mappings..."
    cat << 'EOF' | ./otel-logger --endpoint localhost:4317 --service-name logstash-demo --timestamp-fields "@timestamp" --level-fields "level" --message-fields "message"
{"@timestamp": "2024-01-15T10:30:47Z", "level": "INFO", "message": "Application startup completed", "service": "web-api", "host": "server-001"}
{"@timestamp": "2024-01-15T10:30:48Z", "level": "WARN", "message": "High memory usage detected", "memory_usage_percent": 87.5, "threshold": 85.0}
{"@timestamp": "2024-01-15T10:30:49Z", "level": "ERROR", "message": "Database connection failed", "error_code": "DB_TIMEOUT", "retry_count": 3}
EOF
    print_success "Logstash format logs sent"

    sleep 2

    # Demo 6: Custom application format with multiple field mappings
    print_info "Demo 6: Sending custom format logs with multiple field mappings..."
    cat << 'EOF' | ./otel-logger --endpoint localhost:4317 --service-name custom-demo --timestamp-fields "created_at,event_time" --level-fields "severity,priority" --message-fields "description,content"
{"created_at": "2024-01-15T10:30:50Z", "severity": "high", "description": "Payment processing failed", "transaction_id": "txn_abc123", "amount": 99.99}
{"event_time": "2024-01-15T10:30:51Z", "priority": "medium", "content": "User profile updated successfully", "user_id": "usr_456", "changes": ["email", "phone"]}
{"created_at": "2024-01-15T10:30:52Z", "severity": "low", "description": "Cache warming completed", "keys_loaded": 50000, "time_ms": 2500}
EOF
    print_success "Custom format logs sent"

    sleep 2

    # Demo 7: Example log files
    if [ -f "examples/json-logs.txt" ]; then
        print_info "Demo 7: Sending example JSON logs from file..."
        cat examples/json-logs.txt | ./otel-logger --endpoint localhost:4317 --service-name file-demo --batch-size 5
        print_success "Example JSON logs sent"
    fi

    if [ -f "examples/mixed-logs.txt" ]; then
        print_info "Demo 8: Sending mixed format logs from file..."
        cat examples/mixed-logs.txt | ./otel-logger --endpoint localhost:4317 --service-name mixed-demo --batch-size 3
        print_success "Mixed format logs sent"
    fi

    # Demo 9: Example format files with custom mappings
    if [ -f "examples/logstash-format.txt" ]; then
        print_info "Demo 9: Sending Logstash format example logs..."
        cat examples/logstash-format.txt | ./otel-logger --endpoint localhost:4317 --service-name logstash-file-demo --timestamp-fields "@timestamp" --level-fields "level" --message-fields "message" --batch-size 3
        print_success "Logstash format example logs sent"
    fi

    if [ -f "examples/custom-format.txt" ]; then
        print_info "Demo 10: Sending custom format example logs..."
        cat examples/custom-format.txt | ./otel-logger --endpoint localhost:4317 --service-name custom-file-demo --timestamp-fields "created_at,event_time,occurred_at" --level-fields "severity,priority,log_level" --message-fields "description,content,log_text" --batch-size 3
        print_success "Custom format example logs sent"
    fi

    sleep 2

    # Demo 11: High throughput test
    print_info "Demo 11: High throughput test (100 log entries)..."
    for i in {1..100}; do
        echo "{\"timestamp\": \"$(date -Iseconds)\", \"level\": \"info\", \"message\": \"High throughput test message $i\", \"iteration\": $i, \"batch\": \"throughput-test\"}"
    done | ./otel-logger --endpoint localhost:4317 --service-name throughput-demo --batch-size 50 --flush-interval 1s
    print_success "High throughput test completed"
}

# Show access URLs
show_access_info() {
    print_header "Service Access Information"

    echo -e "${GREEN}OpenTelemetry Collector:${NC}"
    echo "  - gRPC endpoint: localhost:4317"
    echo "  - HTTP endpoint: http://localhost:4318"
    echo "  - Health check: http://localhost:13133"
    echo "  - Component status (zpages): http://localhost:55679"
    echo

    echo -e "${GREEN}Jaeger (Distributed Tracing):${NC}"
    echo "  - UI: http://localhost:16686"
    echo "  - Search for traces and view distributed tracing data"
    echo

    echo -e "${GREEN}Prometheus (Metrics):${NC}"
    echo "  - UI: http://localhost:9090"
    echo "  - Query metrics and view time series data"
    echo

    echo -e "${GREEN}Grafana (Visualization):${NC}"
    echo "  - UI: http://localhost:3000"
    echo "  - Login: admin/admin"
    echo "  - Create dashboards for metrics and logs"
    echo

    echo -e "${GREEN}Elasticsearch (Log Storage):${NC}"
    echo "  - API: http://localhost:9200"
    echo "  - Index: otel-logs"
    echo "  - Query logs: curl 'http://localhost:9200/otel-logs/_search?pretty'"
    echo

    echo -e "${GREEN}Kibana (Log Visualization):${NC}"
    echo "  - UI: http://localhost:5601"
    echo "  - Create index pattern for 'otel-logs'"
    echo

    echo -e "${GREEN}Loki (Alternative Log Storage):${NC}"
    echo "  - API: http://localhost:3100"
    echo "  - Query via Grafana or LogQL"
    echo
}

# Show example queries
show_examples() {
    print_header "Example Queries and Commands"

    echo -e "${YELLOW}Send a single log entry:${NC}"
    echo "echo '{\"timestamp\": \"$(date -Iseconds)\", \"level\": \"info\", \"message\": \"Test message\"}' | ./otel-logger --endpoint localhost:4317"
    echo

    echo -e "${YELLOW}Monitor a log file in real-time:${NC}"
    echo "tail -f /var/log/myapp.log | ./otel-logger --endpoint localhost:4317 --service-name myapp"
    echo

    echo -e "${YELLOW}Send Docker container logs:${NC}"
    echo "docker logs -f container-name 2>&1 | ./otel-logger --endpoint localhost:4317 --service-name container-name"
    echo

    echo -e "${YELLOW}Send Logstash format logs:${NC}"
    echo "cat logstash.log | ./otel-logger --endpoint localhost:4317 --timestamp-fields '@timestamp' --level-fields 'level' --message-fields 'message'"
    echo

    echo -e "${YELLOW}Send custom format logs:${NC}"
    echo "cat custom.log | ./otel-logger --endpoint localhost:4317 --timestamp-fields 'created_at,event_time' --level-fields 'severity,priority' --message-fields 'description,content'"
    echo

    echo -e "${YELLOW}Query logs in Elasticsearch:${NC}"
    echo "curl 'http://localhost:9200/otel-logs/_search?q=level:error&pretty'"
    echo

    echo -e "${YELLOW}View collector health:${NC}"
    echo "curl http://localhost:13133"
    echo

    echo -e "${YELLOW}Stop all services:${NC}"
    echo "./quickstart.sh stop"
    echo
}

# Stop services
stop_services() {
    print_info "Stopping all services..."

    if docker compose version &> /dev/null; then
        COMPOSE_CMD="docker compose"
    else
        COMPOSE_CMD="docker-compose"
    fi

    $COMPOSE_CMD down
    print_success "All services stopped"
}

# Show logs
show_logs() {
    print_info "Showing OpenTelemetry Collector logs..."
    docker logs otel-collector --tail 50 -f
}

# Main script logic
case "${1:-start}" in
    "start")
        print_header "OpenTelemetry Logger Quick Start"
        check_docker
        build_logger
        start_services
        wait_for_services
        run_demo
        show_access_info
        show_examples
        ;;
    "stop")
        stop_services
        ;;
    "logs")
        show_logs
        ;;
    "demo")
        build_logger
        run_demo
        ;;
    "info")
        show_access_info
        show_examples
        ;;
    *)
        echo "Usage: $0 {start|stop|logs|demo|info}"
        echo
        echo "Commands:"
        echo "  start  - Start all services and run demonstrations (default)"
        echo "  stop   - Stop all services"
        echo "  logs   - Show OpenTelemetry Collector logs"
        echo "  demo   - Run demonstrations only (services must be running)"
        echo "  info   - Show service access information and examples"
        exit 1
        ;;
esac

#!/bin/bash

echo "2024-01-15T10:30:00Z INFO Starting application"
echo "  Environment: production"
echo "  Version: 2.1.0"

echo "2024-01-15T10:30:05Z ERROR Failed to process request"
echo "  Exception: NullPointerException"
echo "    at com.example.Service.process(Service.java:42)"
echo "    at com.example.Controller.handle(Controller.java:15)"

echo "2024-01-15T10:30:10Z DEBUG Processing user request"
echo "  User ID: 12345"
echo "  Request path: /api/users/profile"

echo "2024-01-15T10:30:15Z INFO Request completed successfully"

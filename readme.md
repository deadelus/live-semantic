# 🎥 LiveSemantic

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](https://opensource.org/licenses/MIT)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen)](https://github.com/your-org/livesemantic)

**Real-time semantic video analysis with natural language AI filters**

LiveSemantic analyzes video streams and files using AI-powered semantic understanding. Define any filter in natural language ("person walking", "red car entering", "crowd gathering") and get instant matches with sub-50ms latency.

## 🚀 **Quick Start**

### Prerequisites
- Go 1.24.5+
- Python 3.9+ (for model export)
- OpenCV 4.x
- ONNX Runtime

### Installation
```bash
# Clone repository
git clone https://github.com/your-org/livesemantic.git
cd livesemantic

# Install Go dependencies
go mod tidy

# Build the application
go build -o livesemantic src/main.go
```

### Basic Usage

#### Interactive CLI (Default)
```bash
# Interactive mode with Survey prompts
./livesemantic

# Explicit interactive mode
./livesemantic interactive
./livesemantic -i
```

#### Classic CLI Commands
```bash
# Create an example (current working feature)
./livesemantic example create john@example.com "John Doe"

# Show help
./livesemantic help

# Show version
./livesemantic version
```

#### Future Video Analysis Features
```bash
# Real-time webcam surveillance (planned)
./livesemantic realtime \
  --source="cam0" \
  --filter="person walking,vehicle entering" \
  --threshold=0.7

# Batch video file analysis (planned)
./livesemantic batch \
  --file="video.mp4" \
  --filters="celebration,applause,dancing" \
  --export-clips
```

#### Web API Mode
```bash
# Start web server on port 8080
./livesemantic web

# Or specify custom port
./livesemantic web 3000

# Test API endpoint
curl -X POST http://localhost:8080/api/v1/example \
  -H "Content-Type: application/json" \
  -d '{"email":"john@example.com","name":"John Doe"}'
```

#### WebSocket Mode
```bash
# Start WebSocket server on port 8081
./livesemantic ws

# Or specify custom port
./livesemantic ws 9000

# Connect to ws://localhost:8081/ws
# Send message: {"type":"example","data":{"email":"john@example.com","name":"John Doe"}}
```

## 🏗️ **Architecture**

LiveSemantic follows Clean Architecture principles with transport-agnostic design:

```
┌─────────────────────┐
│ Interactive CLI     │  Survey-based prompts & menus
├─────────────────────┤
│   Classic CLI       │  Cobra CLI with commands
├─────────────────────┤
│   Web Transport     │  Gin HTTP server
├─────────────────────┤
│   WS Transport      │  Gorilla WebSocket server
├─────────────────────┤
│  Base Handler       │  Shared business logic
├─────────────────────┤
│   Use Cases         │  Domain business rules
├─────────────────────┤
│   Domain DTOs       │  Data transfer objects
└─────────────────────┘
```

### Key Features

- **🎯 Interactive by Default**: User-friendly Survey prompts for common tasks
- **🔄 Transport Agnostic**: Same business logic works across Interactive CLI, Classic CLI, Web API, and WebSocket
- **📦 Clean Architecture**: Clear separation of concerns with dependency injection
- **⚡ Type-Safe**: Go generics for compile-time safety
- **🪵 Structured Logging**: Zap logger with graceful shutdown
- **🔧 Configuration**: Cobra + Viper for professional CLI experience
- **🐳 Container Ready**: Docker and Kubernetes deployment examples

## 📖 **Project Structure**

```
live-semantic/
├── .env                          # Environment variables
├── go.mod                        # Go module dependencies
├── src/
│   ├── main.go                   # Application entry point
│   ├── domain/                   # Business logic layer
│   │   ├── use_cases.go         # Use case interfaces
│   │   ├── uc_example.go        # Example use case implementation
│   │   ├── dto.go               # Result pattern
│   │   └── dto_example.go       # Example DTOs
│   └── transport/               # Transport layer
│       ├── transport.go         # Transport interfaces
│       ├── handler.go           # Base handler
│       ├── handle_example.go    # Example handler logic
│       ├── cli/                 # CLI transport (Interactive + Classic)
│       │   ├── interactive.go   # Survey controller base
│       │   ├── menu.go          # Interactive main menu
│       │   ├── cli_example.go   # Interactive example flows
│       │   ├── cli_settings.go  # Interactive settings
│       │   └── cmd/             # Classic Cobra commands
│       │       ├── root.go      # Cobra root command
│       │       └── cmd_example.go # Cobra example commands
│       ├── api/                 # HTTP transport
│       │   ├── server.go        # Gin server
│       │   ├── routes.go        # Route definitions
│       │   └── api_example.go   # API handlers
│       └── websocket/           # WebSocket transport
│           ├── server.go        # WS server
│           └── handler.go       # WS message handlers
├── pkg/app/                     # Application framework
│   ├── application/             # App context & lifecycle
│   ├── logger/                  # Logging interfaces
│   └── lifecycle/               # Graceful shutdown
└── docs/                        # Documentation
```

## 🔧 **Development**

### Build Commands
```bash
# Install dependencies
go mod tidy

# Development build
go build -o livesemantic src/main.go

# Production build with optimizations
go build -ldflags="-s -w" -o livesemantic src/main.go

# Cross-compilation for Linux
GOOS=linux GOARCH=amd64 go build -o livesemantic-linux src/main.go

# Run tests
go test ./...

# Format code
go fmt ./...

# Lint code
golangci-lint run
```

### Environment Setup
Create a `.env` file in the project root:
```env
APP_NAME="live semantic"
APP_VERSION="0.1.0"
APP_ENV="development"
APP_DEBUG="true"
```

### Adding New Use Cases

1. **Define DTOs** in `src/domain/dto_*.go`:
```go
type CreateTaskRequest struct {
    Title       string `json:"title"`
    Description string `json:"description"`
}

type TaskResponse struct {
    ID          string    `json:"id"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    CreatedAt   time.Time `json:"created_at"`
}
```

2. **Add use case** to interface in `src/domain/use_cases.go`:
```go
type UseCases interface {
    CreateTask(context.Context, CreateTaskRequest) (Result[TaskResponse], error) // New
}
```

3. **Implement use case** in `src/domain/uc_*.go`:
```go
func (uc *UseCase) CreateTask(ctx context.Context, req CreateTaskRequest) (Result[TaskResponse], error) {
    // Implementation here
}
```

4. **Add transport handler** in `src/transport/handle_*.go`:
```go
func (h *BaseHandler) HandleCreateTask(req TransportRequest[CreateTaskRequest]) TransportResponse[TaskResponse] {
    // Handler implementation
}
```

5. **Add CLI interfaces** in `src/transport/cli/`:

**Interactive CLI flows** in `cli_*.go`:
```go
func (s *SurveyController) createTaskFlow() error {
    var qs = []*survey.Question{
        {Name: "title", Prompt: &survey.Input{Message: "📝 Task Title:"}},
        {Name: "description", Prompt: &survey.Input{Message: "📄 Description:"}},
    }
    // Interactive implementation with confirmation
}
```

**Interactive menu** in `menu.go`:
```go
func (s *SurveyController) Run() error {
    for {
        var action string
        prompt := &survey.Select{
            Message: "What would you like to do?",
            Options: []string{"📝 Create Task", "📋 List Tasks", "❌ Exit"},
        }
        // Menu handling logic
    }
}
```

**Classic CLI commands** in `cmd/cmd_*.go`:
```go
var createTaskCmd = &cobra.Command{
    Use:   "create-task [title] [description]",
    Short: "Create a new task",
    Args:  cobra.ExactArgs(2),
    Run: func(cmd *cobra.Command, args []string) {
        // CLI implementation
    },
}
```

6. **Add API endpoint** in `src/transport/api/api_*.go`:
```go
func (s *Server) createTask(c *gin.Context) {
    // API implementation
}
```

## 🚀 **Deployment**

### Docker
```bash
# Build image
docker build -t livesemantic:latest .

# Run CLI mode
docker run --rm livesemantic:latest example create john@example.com "John"

# Run web server
docker run -d -p 8080:8080 livesemantic:latest web

# Run WebSocket server
docker run -d -p 8081:8081 livesemantic:latest ws
```

### Docker Compose
```yaml
# docker-compose.yml
version: '3.8'
services:
  api:
    build: .
    command: ["./livesemantic", "web", "8080"]
    ports:
      - "8080:8080"
    environment:
      - APP_ENV=production
  
  websocket:
    build: .
    command: ["./livesemantic", "ws", "8081"]
    ports:
      - "8081:8081"
    environment:
      - APP_ENV=production
```

### Kubernetes
```yaml
# k8s-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: livesemantic-api
spec:
  replicas: 3
  selector:
    matchLabels:
      app: livesemantic-api
  template:
    metadata:
      labels:
        app: livesemantic-api
    spec:
      containers:
      - name: livesemantic
        image: livesemantic:latest
        command: ["./livesemantic", "web", "8080"]
        ports:
        - containerPort: 8080
        env:
        - name: APP_ENV
          value: "production"
---
apiVersion: v1
kind: Service
metadata:
  name: livesemantic-service
spec:
  selector:
    app: livesemantic-api
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
```

## 🧪 **Testing**

### Manual Testing

#### Interactive CLI Testing
```bash
# Test interactive mode (default)
./livesemantic

# Test explicit interactive mode  
./livesemantic interactive
./livesemantic -i

# Interactive flow example:
# 🚀 Welcome to Live Semantic Interactive CLI!
# ? What would you like to do? 
#   ▶ 📝 Create Example
#     📋 List Examples
#     ⚙️ Settings
#     ❌ Exit
# 
# ? 📧 Email: test@example.com
# ? 👤 Name: Test User
# ? Create example for Test User (test@example.com)? Yes
# ✅ Example created successfully!
```

#### Classic CLI Testing
```bash
# Test example creation
./livesemantic example create test@example.com "Test User"

# Test with verbose output
./livesemantic example create test@example.com "Test User" --verbose

# Test help system
./livesemantic help
./livesemantic example help
```

#### API Testing
```bash
# Start server
./livesemantic web &

# Test health endpoint
curl http://localhost:8080/health

# Test example creation
curl -X POST http://localhost:8080/api/v1/example \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","name":"Test User"}'

# Stop server
pkill livesemantic
```

#### WebSocket Testing
```bash
# Start WebSocket server
./livesemantic ws &

# Test with wscat (install: npm install -g wscat)
wscat -c ws://localhost:8081/ws

# Send test message
{"type":"example","data":{"email":"test@example.com","name":"Test User"}}
```

### Unit Testing
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./src/domain/
go test ./src/transport/
```

## 📊 **Monitoring & Observability**

### Built-in Logging
The application uses structured logging with Zap:

```bash
# Enable debug logging
APP_DEBUG=true ./livesemantic example create test@example.com "Test"

# Different log levels based on APP_ENV
APP_ENV=development  # Debug logging with caller info
APP_ENV=production   # JSON structured logging
```

### Health Checks
```bash
# API health check
curl http://localhost:8080/health

# WebSocket health check
curl http://localhost:8081/health
```

### Graceful Shutdown
The application handles SIGTERM and SIGINT signals gracefully:
```bash
# Start application
./livesemantic web &

# Graceful shutdown
kill -TERM $!
```

## 🤝 **Contributing**

We welcome contributions! Here's how to get started:

### Development Workflow
1. **Fork** the repository
2. **Create** a feature branch: `git checkout -b feature/amazing-feature`
3. **Make** your changes following the project structure
4. **Test** your changes: `go test ./...`
5. **Commit** your changes: `git commit -m 'Add amazing feature'`
6. **Push** to the branch: `git push origin feature/amazing-feature`
7. **Open** a Pull Request

### Coding Standards
- Follow Go conventions and `gofmt` formatting
- Use Clean Architecture principles
- Add tests for new features
- Update documentation for API changes
- Use structured logging with appropriate context
- Maintain both interactive and classic CLI interfaces for consistency

### Project Principles
- **User-Friendly by Default**: Interactive CLI for ease of use, classic CLI for automation
- **Transport Agnostic**: Business logic should work across all transports
- **Type Safety**: Use Go generics for compile-time safety
- **Clean Architecture**: Maintain clear separation of concerns
- **Testability**: Write testable code with dependency injection
- **Documentation**: Keep README and code comments up to date

## 📄 **License**

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 **Acknowledgments**

- [Survey](https://github.com/AlecAivazis/survey) for interactive CLI prompts
- [Cobra](https://github.com/spf13/cobra) for the powerful CLI framework
- [Viper](https://github.com/spf13/viper) for configuration management
- [Gin](https://github.com/gin-gonic/gin) for the HTTP web framework
- [Gorilla WebSocket](https://github.com/gorilla/websocket) for WebSocket support
- [Zap](https://github.com/uber-go/zap) for structured logging
- The Go community for excellent tooling and libraries

---

**Built with ❤️ using Clean Architecture, Interactive CLI, and Go best practices**
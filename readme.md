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

## 🚀 Installation GoCV et OpenCV sur macOS

Pour utiliser la caméra avec GoCV sur macOS :

1. **Installer OpenCV avec Homebrew**
   ```sh
   brew install opencv
   ```

2. **Définir la variable d’environnement pour GoCV**
   ```sh
   export PKG_CONFIG_PATH=/opt/homebrew/lib/pkgconfig
   ```
   Pour la rendre permanente :
   ```sh
   echo 'export PKG_CONFIG_PATH=/opt/homebrew/lib/pkgconfig' >> ~/.zshrc
   source ~/.zshrc
   ```

3. **Vérifier l’installation**
   ```sh
   go run src/main.go
   ```

Si tu rencontres une erreur liée à `opencv4.pc` ou à la variable d’environnement, vérifie bien les étapes ci-dessus.

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
# Create a new task
./livesemantic create-task "My First Task" "A description of the task"

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
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"My API Task","description":"Task created via API"}'
```

#### WebSocket Mode
```bash
# Start WebSocket server on port 8081
./livesemantic ws

# Or specify custom port
./livesemantic ws 9000

# Connect to ws://localhost:8081/ws
# Send message: {"type":"create_task","data":{"title":"My WebSocket Task","description":"Task via WS"}}
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
├── .gitignore                    # Git ignore file
├── go.mod                        # Go module dependencies
├── go.sum                        # Go module checksums
├── readme.md                     # This file
├── src/
│   ├── main.go                   # Application entry point
│   ├── domain/                   # Core business logic and models
│   │   ├── dto/                  # Data Transfer Objects
│   │   ├── models/               # Domain models (entities)
│   │   └── uc/                   # Use Case implementations
│   ├── infrastructure/           # External concerns (AI, DB, etc.)
│   └── transport/                # Adapters for delivery mechanisms
│       ├── handler.go            # Base handler for all transports
│       ├── api/                  # REST API (Gin)
│       ├── cli/                  # Interactive CLI (Survey)
│       ├── cmd/                  # Classic CLI (Cobra)
│       └── websocket/            # WebSocket transport (Gorilla)
├── internal/                     # Internal application code/scripts
│   └── scripts/
└── docs/                         # Project documentation
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

1. **Define DTOs** in `src/domain/dto/dto_task.go`:
```go
// src/domain/dto/dto_task.go
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

2. **Add use case** to interface in `src/domain/uc/use_case.go`:
```go
// src/domain/uc/use_case.go
type UseCases interface {
    CreateTask(context.Context, dto.CreateTaskRequest) (dto.Result[dto.TaskResponse], error) // New
}
```

3. **Implement use case** in `src/domain/uc/uc_task.go`:
```go
// src/domain/uc/uc_task.go
func (uc *UseCase) CreateTask(ctx context.Context, req dto.CreateTaskRequest) (dto.Result[dto.TaskResponse], error) {
    // Implementation here
}
```

4. **Add transport handler** in `src/transport/handle_task.go`:
```go
// src/transport/handle_task.go
func (h *BaseHandler) HandleCreateTask(req TransportRequest[dto.CreateTaskRequest]) TransportResponse[dto.TaskResponse] {
    // Handler implementation
}
```

5. **Add transport adapters**:

**Interactive CLI** in `src/transport/cli/cli_task.go`:
```go
// src/transport/cli/cli_task.go
func (s *SurveyController) createTaskFlow() error {
    // Interactive implementation
}
```

**Classic CLI command** in `src/transport/cmd/cmd_task.go`:
```go
// src/transport/cmd/cmd_task.go
var createTaskCmd = &cobra.Command{
    Use:   "create-task [title] [description]",
    // ...
}
```

**API endpoint** in `src/transport/api/api_task.go`:
```go
// src/transport/api/api_task.go
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
docker run --rm livesemantic:latest create-task "Docker Task" "A task from Docker"

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
#   ▶ 📝 Create Task
#     📋 List Tasks
#     ⚙️ Settings
#     ❌ Exit
#
# ? 📝 Task Title: My Interactive Task
# ? 📄 Description: A new task created interactively
# ? Create task "My Interactive Task"? Yes
# ✅ Task created successfully!
```

#### Classic CLI Testing
```bash
# Test task creation
./livesemantic create-task "My CLI Task" "A new task from CLI"

# Test with verbose output
./livesemantic create-task "My Verbose Task" "A verbose task" --verbose

# Test help system
./livesemantic help
./livesemantic create-task --help
```

#### API Testing
```bash
# Start server
./livesemantic web &

# Test health endpoint
curl http://localhost:8080/health

# Test task creation
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"My API Test Task","description":"A task for API testing"}'

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
{"type":"create_task","data":{"title":"My WS Test Task","description":"A task for WS testing"}}
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
APP_DEBUG=true ./livesemantic create-task "My Debug Task" "A task for debugging"

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
# 🎥 LiveSemantic

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](https://opensource.org/licenses/MIT)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen)](https://github.com/deadelus/live-semantic)

**Real-time semantic video analysis with natural language AI filters** *(vision — voir l'état d'avancement ci-dessous pour ce qui existe réellement aujourd'hui)*

LiveSemantic vise à analyser des flux vidéo en langage naturel ("person walking", "red car entering", "crowd gathering"). **Aujourd'hui, le projet détecte des objets en webcam temps réel avec YOLO11s (80 classes COCO fixes) — le matching sémantique en langage naturel n'est pas encore implémenté.** Détails complets : [AUDIT.md](AUDIT.md).

---

## 📍 État d'avancement du projet

> Dernière mise à jour : 2026-08-04, branche `main`/`feat/displayer`. Pour l'audit complet, le backlog et le plan de migration : [AUDIT.md](AUDIT.md), [TODO.md](TODO.md), [MIGRATION.md](MIGRATION.md).

### Vision vs réalité

| | Vision | Réalité actuelle |
|---|---|---|
| Filtres | Langage naturel libre (CLIP, embeddings) | Vocabulaire fermé : 80 classes COCO (YOLO), comparaison de chaîne |
| Détection | Cascade YOLO → crop → CLIP | YOLO11s seul, ONNX natif Go ✅ |
| Tracking | Tracker visuel (KCF/CSRT/MOSSE) + agrégat `Track` | Absent |
| Modes | Realtime + Batch fichiers | Realtime webcam ✅ — Batch absent |
| Transport | CLI + Web API + WebSocket, même logique métier partout | CLI ✅ branchée sur le use case — Web API et WebSocket : squelettes non branchés |
| Observabilité | Métriques (latence, throughput, taux de match) | Logs structurés zap uniquement, pas de métriques |

### Architecture réelle

```
┌─────────────────────┐
│     TRANSPORT       │  CLI (cobra) ✅, mode interactif ✅ — Web API (gin) et WebSocket : squelettes non branchés
├─────────────────────┤
│  APPLICATION (uc)   │  UseCases.RecognitionUseCase ✅ — un seul use case, orchestre les ports
├─────────────────────┤
│     DOMAIN          │  entités pures (Frame, Class...) — zéro dépendance externe
├─────────────────────┤
│  INFRASTRUCTURE     │  ports (interfaces) ai.AI, streamer.InputStream/OutputStream, notifier.Notifier ✅
├─────────────────────┤
│  IMPLEMENTATION     │  yolo11s (ONNX natif) ✅, camera gocv ✅, window gocv ✅, log-notifier ✅
└─────────────────────┘
```

L'inversion de dépendance est réelle : `application/uc` ne dépend que d'interfaces (`internal/infrastructure/*`), les implémentations concrètes (`internal/implementation/*`) sont injectées dans `cmd/livesemantic/main.go`.

### Roadmap

- [x] Architecture Clean + ports/adapters
- [x] ONNX Go natif intégré (YOLO11s, vocabulaire fermé)
- [x] Pipeline webcam basique (gocv)
- [x] CLI realtime surveillance
- [ ] Métriques console
- [ ] Cache embeddings LRU
- [ ] Mode batch fichiers vidéo
- [ ] Tracking + agrégat `Track`
- [ ] Cascade YOLO → crop → CLIP (matching sémantique en langage naturel)
- [ ] API REST + WebSocket branchées sur la logique métier
- [ ] Persistance, monitoring, conteneurisation

~2000 lignes de Go (hors vendor), couverture de tests quasi nulle (2 fichiers de test, dont 1 sans test réel exécuté).

---

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
   go run ./cmd/livesemantic
   ```

Si tu rencontres une erreur liée à `opencv4.pc` ou à la variable d’environnement, vérifie bien les étapes ci-dessus.

### Installation
```bash
# Clone repository
git clone https://github.com/deadelus/live-semantic.git
cd live-semantic

# Install Go dependencies
go mod tidy

# Build the application
go build -o livesemantic ./cmd/livesemantic
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

#### Classic CLI Commands (réellement implémenté)
```bash
# Run realtime webcam recognition (YOLO11s, vocabulaire fermé COCO)
./livesemantic recognition --filter="person" --similarity-threshold=0.7

# Show help
./livesemantic help

# Show version
./livesemantic version
```

#### Future Video Analysis Features (non implémenté)
```bash
# Batch video file analysis (planned — voir TODO.md décision A)
./livesemantic batch \
  --file="video.mp4" \
  --filters="celebration,applause,dancing" \
  --export-clips
```

#### Web API / WebSocket Mode ⚠️ squelettes non fonctionnels
```bash
# Ces serveurs démarrent (-s / -w) mais ne routent vers AUCUN use case métier.
# Les exemples curl/wscat ci-dessous ne fonctionneront pas tant que TODO.md
# (branchement transport/api et transport/websocket sur RecognitionUseCase)
# n'est pas fait.
./livesemantic -s -p 8080   # web
./livesemantic -w -p 8081   # websocket
```

## 🏗️ **Architecture (vision cible)**

> ⚠️ Ceci décrit l'architecture visée pour l'ensemble des transports. L'architecture *réellement en place aujourd'hui* est décrite plus haut dans [État d'avancement](#-état-davancement-du-projet) — seule la CLI est branchée sur la logique métier, Web API et WebSocket sont des squelettes.

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
├── .env.example                  # Template des variables d'environnement (.env réel non versionné)
├── .gitignore
├── go.mod / go.sum
├── LICENSE
├── readme.md / overview.md / AUDIT.md / TODO.md / MIGRATION.md
├── assets/                       # Assets binaires (non compilés) : modèles ONNX, libs natives, fonts
│   ├── fonts/
│   ├── models/
│   └── libraries/{linux,osx,win}/
├── cmd/
│   └── livesemantic/
│       └── main.go                # Point d'entrée, wiring des dépendances
└── internal/
    ├── domain/                    # Pur : entités, zéro dépendance externe
    │   └── entities/
    ├── application/                # Orchestre domain + ports
    │   ├── uc/                     # Use cases (RecognitionUseCase)
    │   └── dto/                    # Contrats input/output des use cases
    ├── infrastructure/             # Interfaces / ports (ai, notifier, streamer)
    ├── implementation/             # Adapters concrets (yolo11s, gocv camera/window, drawer, log-notifier)
    └── transport/                  # CLI (cobra + interactif) ✅, API (gin) et WebSocket ❌ squelettes
        ├── handler/                # BaseHandler partagé par les transports
        ├── envelope/                # TransportRequest/Response — enveloppe agnostique (Source, Context)
        ├── api/
        ├── cli/
        ├── cmd/
        └── websocket/
```

## 🔧 **Development**

### Build Commands
```bash
# Install dependencies
go mod tidy

# Development build
go build -o livesemantic ./cmd/livesemantic

# Production build with optimizations
go build -ldflags="-s -w" -o livesemantic ./cmd/livesemantic

# Cross-compilation for Linux
GOOS=linux GOARCH=amd64 go build -o livesemantic-linux ./cmd/livesemantic

# Run tests
go test ./...

# Format code
go fmt ./...

# Lint code
golangci-lint run
```

### Environment Setup
Copie `.env.example` en `.env` à la racine (le fichier `.env` réel n'est pas versionné) :
```bash
cp .env.example .env
```

### Adding New Use Cases

1. **Define DTOs** in `internal/application/dto/dto_task.go`:
```go
// internal/application/dto/dto_task.go
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

2. **Add use case** to interface in `internal/application/uc/use_case.go`:
```go
// internal/application/uc/use_case.go
type UseCases interface {
    CreateTask(context.Context, dto.CreateTaskRequest) (dto.Result[dto.TaskResponse], error) // New
}
```

3. **Implement use case** in `internal/application/uc/uc_task.go`:
```go
// internal/application/uc/uc_task.go
func (uc *UseCase) CreateTask(ctx context.Context, req dto.CreateTaskRequest) (dto.Result[dto.TaskResponse], error) {
    // Implementation here
}
```

4. **Add transport handler** in `internal/transport/handle_task.go`:
```go
// internal/transport/handle_task.go
func (h *BaseHandler) HandleCreateTask(req TransportRequest[dto.CreateTaskRequest]) TransportResponse[dto.TaskResponse] {
    // Handler implementation
}
```

5. **Add transport adapters**:

**Interactive CLI** in `internal/transport/cli/cli_task.go`:
```go
// internal/transport/cli/cli_task.go
func (s *SurveyController) createTaskFlow() error {
    // Interactive implementation
}
```

**Classic CLI command** in `internal/transport/cmd/cmd_task.go`:
```go
// internal/transport/cmd/cmd_task.go
var createTaskCmd = &cobra.Command{
    Use:   "create-task [title] [description]",
    // ...
}
```

**API endpoint** in `internal/transport/api/api_task.go`:
```go
// internal/transport/api/api_task.go
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
go test ./internal/domain/... ./internal/application/...
go test ./internal/transport/...
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
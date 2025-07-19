#!/bin/bash

# LiveSemantic - GitHub Issues Creation Script
# Usage: ./create_issues.sh
# Prerequisites: gh auth login

set -e

echo "🎯 Creating LiveSemantic GitHub Issues..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if gh is authenticated
if ! gh auth status &>/dev/null; then
    echo -e "${RED}❌ GitHub CLI not authenticated. Run: gh auth login${NC}"
    exit 1
fi

echo -e "${BLUE}📋 Creating labels first...${NC}"

# Create labels if they don't exist
gh label create "epic:foundation" --description "Foundation Architecture" --color "8B5CF6" || true
gh label create "epic:ai" --description "AI Integration" --color "F59E0B" || true  
gh label create "epic:video" --description "Video Pipeline" --color "10B981" || true
gh label create "epic:cli" --description "CLI Interface" --color "3B82F6" || true
gh label create "epic:alerts" --description "Alerting System" --color "EF4444" || true
gh label create "epic:observability" --description "Observability" --color "6366F1" || true
gh label create "epic:production" --description "Production Ready" --color "8B5A2B" || true
gh label create "epic:advanced" --description "Advanced Features" --color "EC4899" || true

gh label create "priority:highest" --description "Critical Priority" --color "DC2626" || true
gh label create "priority:high" --description "High Priority" --color "EA580C" || true  
gh label create "priority:medium" --description "Medium Priority" --color "D97706" || true
gh label create "priority:low" --description "Low Priority" --color "65A30D" || true

gh label create "effort:3" --description "3 Story Points" --color "DBEAFE" || true
gh label create "effort:5" --description "5 Story Points" --color "BFDBFE" || true
gh label create "effort:8" --description "8 Story Points" --color "93C5FD" || true  
gh label create "effort:13" --description "13 Story Points" --color "60A5FA" || true
gh label create "effort:21" --description "21 Story Points" --color "3B82F6" || true

echo -e "${GREEN}✅ Labels created successfully${NC}"
echo -e "${BLUE}🎫 Creating issues...${NC}"

# =============================================================================
# EPIC 1: Foundation Architecture  
# =============================================================================

gh issue create \
  --title "LIVE-001: Core Domain Setup" \
  --body "## 📝 Description
Setup domain entities with business rules for the Clean Architecture foundation.

## 🎯 User Story
**As a** developer  
**I want** a clean domain layer with business entities  
**So that** I can build on solid architectural foundations

## ✅ Acceptance Criteria
- [ ] Video, Match, Filter entities with business rules and validation
- [ ] Domain events (MatchDetected, ProcessingStarted, ProcessingCompleted)
- [ ] Ports interfaces defined (AIProvider, VideoSource, AlertSender, MetricsCollector)
- [ ] Value objects (Confidence, Embedding, VideoTimestamp, BoundingBox)
- [ ] Domain layer unit tests with 90%+ coverage

## 🏗️ Technical Details
- Domain entities in \`internal/domain/\`
- No external dependencies in domain layer
- Rich domain model with business logic
- Immutable value objects where appropriate

## 🔗 Related Issues
- Blocks: #2 (Application Use Cases)

## 📋 Definition of Done
- [ ] Code reviewed and approved
- [ ] Unit tests passing
- [ ] Documentation updated
- [ ] No external dependencies in domain" \
  --label "epic:foundation,priority:highest,effort:5" \
  --milestone "Sprint 1"

gh issue create \
  --title "LIVE-002: Application Use Cases" \
  --body "## 📝 Description
Implement business logic use cases for realtime surveillance and batch video analysis.

## 🎯 User Story
**As a** developer  
**I want** use cases for realtime and batch processing  
**So that** I can implement business logic cleanly separated from infrastructure

## ✅ Acceptance Criteria
- [ ] RealtimeProcessingUseCase with streaming logic
- [ ] BatchProcessingUseCase with file analysis logic
- [ ] ProcessingStrategy interface (realtime vs batch optimization)
- [ ] Event handlers for domain events (MatchDetected, ProcessingStarted)
- [ ] Use case integration tests

## 🏗️ Technical Details
- Use cases in \`internal/application/usecases/\`
- Strategy pattern for processing modes
- Event-driven architecture with handlers
- Clean separation of concerns

## 🔗 Related Issues
- Depends on: #1 (Core Domain Setup)
- Blocks: #4 (ONNX Go Provider)

## 📋 Definition of Done
- [ ] Use cases implemented with proper error handling
- [ ] Strategy pattern correctly implemented
- [ ] Event handlers working
- [ ] Integration tests passing" \
  --label "epic:foundation,priority:highest,effort:8" \
  --milestone "Sprint 1"

gh issue create \
  --title "LIVE-003: ONNX Model Export" \
  --body "## 📝 Description
Export CLIP models to ONNX format for ultra-fast Go native inference.

## 🎯 User Story
**As a** developer  
**I want** CLIP models exported to ONNX format  
**So that** I can achieve < 20ms inference latency in Go

## ✅ Acceptance Criteria
- [ ] Python script exports CLIP text encoder → ONNX
- [ ] Python script exports CLIP image encoder → ONNX  
- [ ] Models validation (input/output shapes, inference test)
- [ ] Setup documentation and requirements.txt
- [ ] Models stored in \`models/\` directory

## 🏗️ Technical Details
- Use \`torch.onnx.export()\` for model conversion
- Target ONNX opset version 12+ for compatibility
- Optimize for inference speed
- Include model metadata and documentation

## 🧠 AI Models
- CLIP ViT-B/32 (150MB, balanced speed/accuracy)
- Text encoder: input_ids → text_features  
- Image encoder: pixel_values → image_features

## 📋 Definition of Done
- [ ] ONNX models exported successfully
- [ ] Models validate with onnxruntime
- [ ] Documentation includes setup steps
- [ ] Models ready for Go integration" \
  --label "epic:ai,priority:highest,effort:3" \
  --milestone "Sprint 1"

# =============================================================================
# EPIC 2: ONNX AI Integration
# =============================================================================

gh issue create \
  --title "LIVE-004: ONNX Go Provider" \
  --body "## 📝 Description
Implement ultra-fast ONNX inference in Go native for semantic video analysis.

## 🎯 User Story
**As a** developer  
**I want** ONNX runtime integration in Go  
**So that** I can achieve < 20ms inference latency for real-time processing

## ✅ Acceptance Criteria
- [ ] ONNXClip struct with text/image encoding methods
- [ ] Image preprocessing (resize, normalize, tensor conversion)
- [ ] Text tokenization for CLIP vocabulary
- [ ] Error handling and graceful model loading
- [ ] Benchmarks showing < 20ms inference time
- [ ] Memory efficient implementation

## 🏗️ Technical Details
- Use \`github.com/yalue/onnxruntime_go\`
- Implement preprocessing pipeline with gocv
- CLIP-specific tokenization and normalization
- Efficient memory management

## ⚡ Performance Targets
- Text encoding: < 5ms
- Image encoding: < 15ms  
- Memory usage: < 200MB
- Concurrent inference support

## 🔗 Related Issues
- Depends on: #3 (ONNX Model Export)
- Blocks: #5 (Multi-Provider Orchestrator)

## 📋 Definition of Done
- [ ] ONNX inference working correctly
- [ ] Performance benchmarks meet targets
- [ ] Error handling robust
- [ ] Unit tests with mock models" \
  --label "epic:ai,priority:highest,effort:13" \
  --milestone "Sprint 2"

gh issue create \
  --title "LIVE-005: Multi-Provider AI Orchestrator" \
  --body "## 📝 Description
Implement resilient AI system with multiple providers, fallbacks, and circuit breaker pattern.

## 🎯 User Story
**As a** developer  
**I want** multiple AI providers with automatic fallback  
**So that** the system remains resilient to individual provider failures

## ✅ Acceptance Criteria
- [ ] AIOrchestrator with configurable provider selection
- [ ] Circuit breaker pattern preventing cascade failures
- [ ] Health checking for all providers (ONNX, Python, REST)
- [ ] Performance metrics collection per provider
- [ ] Automatic failover and recovery logic

## 🏗️ Technical Details
- Implement Circuit Breaker pattern
- Provider selection strategies (FASTEST, MOST_ACCURATE, ROUND_ROBIN)
- Health check endpoints with timeout handling
- Metrics collection for decision making

## 🔧 Resilience Features
- Circuit breaker with configurable thresholds
- Graceful degradation under load
- Provider health monitoring
- Automatic recovery mechanisms

## 🔗 Related Issues
- Depends on: #4 (ONNX Go Provider)
- Blocks: #6 (Video Sources)

## 📋 Definition of Done
- [ ] Multi-provider system working
- [ ] Circuit breaker tested under failure
- [ ] Health checks implemented
- [ ] Metrics collection functional" \
  --label "epic:ai,priority:high,effort:8" \
  --milestone "Sprint 2"

# =============================================================================
# EPIC 3: Video Pipeline
# =============================================================================

gh issue create \
  --title "LIVE-006: Video Sources Implementation" \
  --body "## 📝 Description
Implement video source adapters for webcam, files, and streams using gocv.

## 🎯 User Story
**As a** user  
**I want** to process webcam feeds and video files  
**So that** I can analyze different types of video sources

## ✅ Acceptance Criteria
- [ ] WebcamSource with gocv integration for live capture
- [ ] VideoFileSource supporting MP4, AVI, MOV, WebM formats
- [ ] StreamSource for RTMP/HLS streams (future)
- [ ] Configurable frame extraction (FPS, resolution)
- [ ] Robust error handling for invalid/unavailable sources
- [ ] Source auto-detection and validation

## 🏗️ Technical Details
- Use gocv (OpenCV Go bindings)
- Implement VideoSource interface
- Support multiple concurrent sources
- Efficient frame buffering

## 📹 Supported Formats
- **Webcam**: /dev/video0, DirectShow, V4L2
- **Files**: MP4, AVI, MOV, WebM, MKV
- **Streams**: RTMP, HLS (future enhancement)

## 🔗 Related Issues
- Depends on: #5 (AI Orchestrator)
- Blocks: #7 (Frame Processing Pipeline)

## 📋 Definition of Done
- [ ] All video sources working reliably
- [ ] Error handling comprehensive
- [ ] Performance acceptable for real-time
- [ ] Unit tests with mock sources" \
  --label "epic:video,priority:high,effort:8" \
  --milestone "Sprint 3"

gh issue create \
  --title "LIVE-007: Frame Processing Pipeline" \
  --body "## 📝 Description
Build high-throughput frame processing pipeline with worker pools and backpressure handling.

## 🎯 User Story
**As a** developer  
**I want** an efficient frame processing pipeline  
**So that** I can handle high-throughput video streams without dropping frames

## ✅ Acceptance Criteria
- [ ] Worker pool for parallel frame processing
- [ ] Frame buffering with configurable backpressure strategies
- [ ] Pipeline middleware pattern for extensibility
- [ ] Memory optimization to prevent leaks
- [ ] Graceful handling of processing bottlenecks
- [ ] Performance monitoring and metrics

## 🏗️ Technical Details
- Implement pipeline pattern with middleware
- Worker pools with configurable concurrency
- Backpressure strategies (drop oldest, drop newest, block)
- Memory pooling for frame buffers

## ⚡ Performance Targets
- **Realtime**: 30+ FPS processing
- **Batch**: 2x video speed processing
- **Memory**: < 500MB peak usage
- **Latency**: < 50ms end-to-end

## 🔗 Related Issues
- Depends on: #6 (Video Sources)
- Blocks: #8 (Realtime CLI)

## 📋 Definition of Done
- [ ] Pipeline handling target throughput
- [ ] Backpressure working correctly
- [ ] Memory usage optimized
- [ ] Performance tests passing" \
  --label "epic:video,priority:high,effort:13" \
  --milestone "Sprint 3"

# =============================================================================
# EPIC 4: CLI Interface
# =============================================================================

gh issue create \
  --title "LIVE-008: Realtime CLI Command" \
  --body "## 📝 Description
Implement CLI command for real-time video surveillance with semantic filtering.

## 🎯 User Story
**As a** security operator  
**I want** a CLI command for real-time surveillance  
**So that** I can monitor live video feeds with natural language filters

## ✅ Acceptance Criteria
- [ ] \`livesemantic realtime\` command with full argument parsing
- [ ] Source selection (webcam ID, device path, stream URL)
- [ ] Natural language filter specification (multiple filters supported)
- [ ] Threshold configuration and alert settings
- [ ] Real-time output with timestamps and confidence scores
- [ ] Graceful shutdown with Ctrl+C handling

## 🏗️ Technical Details
- Use cobra/cli for command structure
- Integrate with video pipeline and AI orchestrator
- Real-time console output with color coding
- Configuration validation and helpful error messages

## 💻 CLI Examples
\`\`\`bash
livesemantic realtime --source=\"cam0\" --filter=\"person walking\" --threshold=0.7
livesemantic realtime --source=\"rtmp://cam.local\" --filter=\"vehicle,person\" --alert=\"console\"
\`\`\`

## 🔗 Related Issues
- Depends on: #7 (Frame Processing Pipeline)
- Blocks: #10 (Console Alerting)

## 📋 Definition of Done
- [ ] CLI command working end-to-end
- [ ] All arguments properly validated
- [ ] Real-time output functional
- [ ] Integration tests passing" \
  --label "epic:cli,priority:high,effort:8" \
  --milestone "Sprint 4"

gh issue create \
  --title "LIVE-009: Batch CLI Command" \
  --body "## 📝 Description
Implement CLI command for batch video file analysis with progress reporting.

## 🎯 User Story
**As a** video analyst  
**I want** a CLI command for batch video analysis  
**So that** I can process video files and extract semantic insights

## ✅ Acceptance Criteria
- [ ] \`livesemantic batch\` command with file input support
- [ ] Video file format validation (MP4, AVI, MOV, etc.)
- [ ] Multiple semantic filters with confidence thresholds
- [ ] Progress reporting with ETA and processing speed
- [ ] Results export in multiple formats (JSON, CSV, text)
- [ ] Directory batch processing support

## 🏗️ Technical Details
- File validation and format detection
- Progress bar with detailed statistics
- Concurrent processing for multiple files
- Structured output formats

## 💻 CLI Examples
\`\`\`bash
livesemantic batch --file=\"video.mp4\" --filters=\"celebration,applause\" 
livesemantic batch --dir=\"/videos/\" --filters=\"security-event\" --output=\"results.json\"
\`\`\`

## 🔗 Related Issues
- Depends on: #7 (Frame Processing Pipeline)
- Related: #8 (Realtime CLI Command)

## 📋 Definition of Done
- [ ] Batch processing working correctly
- [ ] Progress reporting accurate
- [ ] Multiple output formats supported
- [ ] Directory processing functional" \
  --label "epic:cli,priority:medium,effort:5" \
  --milestone "Sprint 4"

# =============================================================================
# EPIC 5: Alerting System
# =============================================================================

gh issue create \
  --title "LIVE-010: Console Alerting" \
  --body "## 📝 Description
Implement console-based alerting system for development and debugging.

## 🎯 User Story
**As a** developer  
**I want** console-based alerts with rich formatting  
**So that** I can see semantic matches in real-time during development

## ✅ Acceptance Criteria
- [ ] ConsoleAlertSender implementing AlertSender interface
- [ ] Formatted output with timestamps, confidence, and match details
- [ ] Color coding by severity level (critical=red, high=yellow, medium=blue)
- [ ] Configurable verbosity levels (quiet, normal, verbose, debug)
- [ ] JSON output option for structured logging

## 🏗️ Technical Details
- Rich console formatting with color support
- Structured logging compatibility
- Performance optimized for high-frequency alerts
- Cross-platform color support

## 🎨 Output Format Examples
\`\`\`
[2024-07-19 14:30:25] 🚨 CRITICAL: person walking (confidence: 0.89) at frame 1423
[2024-07-19 14:30:26] ⚠️  HIGH: vehicle entering (confidence: 0.76) at frame 1445
\`\`\`

## 🔗 Related Issues
- Depends on: #8 (Realtime CLI Command)
- Blocks: #11 (Webhook Alerting)

## 📋 Definition of Done
- [ ] Console alerting working correctly
- [ ] Color coding implemented
- [ ] Verbosity levels functional
- [ ] JSON output option working" \
  --label "epic:alerts,priority:medium,effort:3" \
  --milestone "Sprint 4"

gh issue create \
  --title "LIVE-011: Webhook Alerting" \
  --body "## 📝 Description
Implement webhook-based alerting for production system integration.

## 🎯 User Story
**As a** system integrator  
**I want** webhook-based alerts with reliable delivery  
**So that** I can integrate LiveSemantic with external monitoring systems

## ✅ Acceptance Criteria
- [ ] WebhookAlertSender with HTTP client implementation
- [ ] Configurable JSON payload format with customizable fields
- [ ] Retry mechanism with exponential backoff for failed deliveries
- [ ] Authentication support (API keys, Bearer tokens, Basic Auth)
- [ ] SSL/TLS verification with certificate validation
- [ ] Request/response logging for debugging

## 🏗️ Technical Details
- HTTP client with timeout configuration
- Retry logic with configurable max attempts
- Authentication header injection
- JSON payload templating system

## 🔒 Security Features
- TLS certificate verification
- API key rotation support
- Request signing (HMAC)
- Rate limiting protection

## 🔗 Related Issues
- Depends on: #10 (Console Alerting)
- Related: #12 (Performance Metrics)

## 📋 Definition of Done
- [ ] Webhook delivery working reliably
- [ ] Retry mechanism tested under failure
- [ ] Authentication methods implemented
- [ ] Security features validated" \
  --label "epic:alerts,priority:medium,effort:5" \
  --milestone "Sprint 5"

# =============================================================================
# EPIC 6: Observability
# =============================================================================

gh issue create \
  --title "LIVE-012: Performance Metrics" \
  --body "## 📝 Description
Implement comprehensive performance monitoring for system health tracking.

## 🎯 User Story
**As an** operator  
**I want** detailed performance metrics  
**So that** I can monitor system health and optimize performance

## ✅ Acceptance Criteria
- [ ] Latency tracking per component (AI, video processing, alerts)
- [ ] Throughput metrics (FPS processed, matches per second)
- [ ] Error rate monitoring with categorization
- [ ] Memory and CPU usage tracking
- [ ] Console metrics output with real-time dashboard
- [ ] Metrics export interface for external systems

## 🏗️ Technical Details
- Metrics collection with minimal performance overhead
- In-memory aggregation with periodic reporting
- Configurable metrics retention and sampling
- Thread-safe metrics collectors

## 📊 Key Metrics
- **Latency**: AI inference, frame processing, end-to-end
- **Throughput**: Frames/sec, matches/sec, alerts/sec  
- **Errors**: AI failures, video source errors, alert failures
- **Resources**: Memory usage, CPU utilization, disk I/O

## 🔗 Related Issues
- Related: #8 (Realtime CLI Command)
- Blocks: #13 (Prometheus Integration)

## 📋 Definition of Done
- [ ] All key metrics collected accurately
- [ ] Console dashboard functional
- [ ] Performance overhead minimal
- [ ] Metrics export interface ready" \
  --label "epic:observability,priority:medium,effort:5" \
  --milestone "Sprint 5"

gh issue create \
  --title "LIVE-013: Prometheus Integration" \
  --body "## 📝 Description
Implement Prometheus metrics export for production monitoring and alerting.

## 🎯 User Story
**As a** DevOps engineer  
**I want** Prometheus metrics export  
**So that** I can use standard monitoring tools and create alerting rules

## ✅ Acceptance Criteria
- [ ] Prometheus metrics HTTP endpoint (/metrics)
- [ ] Standard metric types (counter, gauge, histogram, summary)
- [ ] Custom metrics for AI performance and video processing
- [ ] Proper metric naming following Prometheus conventions
- [ ] Grafana dashboard template with key visualizations
- [ ] Documentation for metric meanings and alerting rules

## 🏗️ Technical Details
- Use prometheus/client_golang library
- HTTP server for metrics endpoint
- Metric registration and lifecycle management
- Performance optimized metric collection

## 📊 Prometheus Metrics
\`\`\`
livesemantic_frames_processed_total
livesemantic_ai_inference_duration_seconds
livesemantic_matches_detected_total
livesemantic_alerts_sent_total
livesemantic_memory_usage_bytes
\`\`\`

## 🔗 Related Issues
- Depends on: #12 (Performance Metrics)
- Related: #14 (Configuration Management)

## 📋 Definition of Done
- [ ] Prometheus endpoint working
- [ ] All metrics properly exposed
- [ ] Grafana dashboard imported successfully
- [ ] Documentation complete" \
  --label "epic:observability,priority:low,effort:8" \
  --milestone "Sprint 6"

# =============================================================================
# EPIC 7: Production Ready
# =============================================================================

gh issue create \
  --title "LIVE-014: Configuration Management" \
  --body "## 📝 Description
Implement flexible configuration system supporting multiple environments and validation.

## 🎯 User Story
**As an** operator  
**I want** flexible configuration management  
**So that** I can adapt the system to different environments easily

## ✅ Acceptance Criteria
- [ ] YAML configuration files with hierarchical structure
- [ ] Environment variable overrides with clear precedence
- [ ] Configuration validation with helpful error messages
- [ ] Hot reloading support for non-critical settings
- [ ] Configuration schema documentation
- [ ] Default configuration for quick setup

## 🏗️ Technical Details
- Use viper for configuration management
- Schema validation with struct tags
- Environment-specific config files
- Configuration change detection

## ⚙️ Configuration Areas
- **Video**: FPS, resolution, buffer sizes
- **AI**: Provider selection, model paths, thresholds
- **Alerts**: Channel configuration, formatting
- **Performance**: Worker pools, timeouts, limits

## 🔗 Related Issues
- Related: #15 (Docker Containerization)
- Blocks: Production deployment

## 📋 Definition of Done
- [ ] Configuration system working correctly
- [ ] Environment overrides functional
- [ ] Validation providing clear errors
- [ ] Hot reloading implemented" \
  --label "epic:production,priority:medium,effort:5" \
  --milestone "Sprint 6"

gh issue create \
  --title "LIVE-015: Docker Containerization" \
  --body "## 📝 Description
Create production-ready Docker containers with optimized builds and health checks.

## 🎯 User Story
**As a** DevOps engineer  
**I want** optimized Docker containers  
**So that** I can deploy consistently across environments

## ✅ Acceptance Criteria
- [ ] Multi-stage Dockerfile with size optimization
- [ ] ONNX models included in container image
- [ ] Environment-specific docker-compose files
- [ ] Health check endpoints with proper status codes
- [ ] Security scanning and vulnerability assessment
- [ ] Container registry automation

## 🏗️ Technical Details
- Alpine-based images for minimal size
- Non-root user for security
- Proper signal handling for graceful shutdown
- Volume mounts for configuration and data

## 🔒 Security Features
- Non-root container execution
- Minimal attack surface
- Security scanning integration
- Secret management support

## 🔗 Related Issues
- Depends on: #14 (Configuration Management)
- Blocks: Kubernetes deployment

## 📋 Definition of Done
- [ ] Docker images building successfully
- [ ] Containers running reliably
- [ ] Health checks working
- [ ] Security scan passing" \
  --label "epic:production,priority:low,effort:8" \
  --milestone "Sprint 7"

# =============================================================================
# EPIC 8: Advanced Features
# =============================================================================

gh issue create \
  --title "LIVE-016: Video Clip Export" \
  --body "## 📝 Description
Implement automatic video clip generation around detected semantic matches.

## 🎯 User Story
**As a** content creator  
**I want** automatic video clip generation around matches  
**So that** I can quickly access relevant video segments for editing

## ✅ Acceptance Criteria
- [ ] Clip extraction with configurable duration (before/after match)
- [ ] Multiple output formats (MP4, AVI, WebM)
- [ ] Batch clip generation for multiple matches
- [ ] Clip metadata with match details and timestamps
- [ ] Quality settings and compression options
- [ ] Thumbnail generation for quick preview

## 🏗️ Technical Details
- Use ffmpeg for video processing
- Efficient seeking and extraction
- Parallel clip generation
- Metadata embedding in video files

## 🎬 Features
- Configurable clip duration (5s-60s around match)
- Multiple quality presets
- Automatic thumbnail generation
- Clip concatenation for highlight reels

## 🔗 Related Issues
- Depends on: Batch processing functionality
- Related: #17 (Web UI Interface)

## 📋 Definition of Done
- [ ] Clip extraction working correctly
- [ ] Multiple formats supported
- [ ] Batch processing functional
- [ ] Quality settings implemented" \
  --label "epic:advanced,priority:low,effort:13" \
  --milestone "Future"

gh issue create \
  --title "LIVE-017: Web UI Interface" \
  --body "## 📝 Description
Build browser-based interface for real-time monitoring and video analysis.

## 🎯 User Story
**As a** user  
**I want** a web interface for video monitoring  
**So that** I can use the system without command-line knowledge

## ✅ Acceptance Criteria
- [ ] Real-time video stream display with match overlays
- [ ] Filter configuration UI with natural language input
- [ ] Match timeline visualization with playback controls
- [ ] WebSocket integration for live updates
- [ ] Dashboard with system metrics and health status
- [ ] Responsive design for mobile and desktop

## 🏗️ Technical Details
- React/Vue.js frontend with WebSocket connection
- REST API for configuration and historical data
- Video streaming with WebRTC or HLS
- Real-time match visualization

## 🌐 UI Components
- Live video player with match annotations
- Filter management interface
- Timeline scrubber with match markers
- Metrics dashboard with charts
- Alert management panel

## 🔗 Related Issues
- Depends on: WebSocket implementation
- Related: #16 (Video Clip Export)

## 📋 Definition of Done
- [ ] Web UI fully functional
- [ ] Real-time updates working
- [ ] Video streaming implemented
- [ ] Responsive design validated" \
  --label "epic:advanced,priority:low,effort:21" \
  --milestone "Future"

echo -e "${GREEN}✅ All GitHub issues created successfully!${NC}"
echo -e "${YELLOW}📋 Summary:${NC}"
echo -e "  • Epic 1 (Foundation): 3 issues"
echo -e "  • Epic 2 (AI Integration): 2 issues" 
echo -e "  • Epic 3 (Video Pipeline): 2 issues"
echo -e "  • Epic 4 (CLI Interface): 2 issues"
echo -e "  • Epic 5 (Alerting): 2 issues"
echo -e "  • Epic 6 (Observability): 2 issues"
echo -e "  • Epic 7 (Production): 2 issues"
echo -e "  • Epic 8 (Advanced): 2 issues"
echo -e "${BLUE}🎯 Total: 17 issues created${NC}"

echo -e "${GREEN}🚀 Ready to start Sprint 1 with issues #1, #2, #3!${NC}"

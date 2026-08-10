package main

import (
	"context"
	"fmt"
	"live-semantic/internal/application/uc"
	"live-semantic/internal/implementation/inference/onnx/clip"
	"live-semantic/internal/implementation/inference/onnx/yolo11s"
	lognotifier "live-semantic/internal/implementation/notifier/log-notifier"
	"live-semantic/internal/implementation/streamer/input"
	"live-semantic/internal/implementation/streamer/output"
	gocvtracker "live-semantic/internal/implementation/tracking/gocv-tracker"
	"live-semantic/internal/infrastructure/inference"
	"live-semantic/internal/infrastructure/notifier"
	"live-semantic/internal/infrastructure/streamer"
	"live-semantic/internal/infrastructure/tracking"
	"live-semantic/internal/transport/adapters/api"
	"live-semantic/internal/transport/adapters/cli"
	"live-semantic/internal/transport/adapters/cmd"
	"live-semantic/internal/transport/adapters/websocket"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/deadelus/go-clean-app/v2/application"
	"github.com/joho/godotenv"
	"github.com/spf13/pflag"
)

const (
	defaultWebPort       = 8080
	defaultWebsocketPort = 8081
)

func main() {
	if runtime.GOOS == "darwin" {
		runtime.LockOSThread()
	}

	println("Live Semantic - Starting Application")
	// os.Exit(onnx_poc.Run()) // Run the ONNX Proof of concept model and handle any errors

	// go-clean-app v2 no longer auto-loads .env (explicit over magic), so we load it ourselves.
	if err := godotenv.Load(); err != nil {
		fmt.Println("Error loading .env file:", err)
	}

	// Define and parse flags first to determine the mode. Subcommand-owned
	// flags (e.g. `recognition --filter`) aren't known here — cobra parses
	// those itself later from the raw os.Args, in startCLIMode. Without
	// UnknownFlags, this top-level Parse() would hard-exit(2) on any flag
	// it doesn't recognize, before cobra ever gets a chance to run (found
	// 2026-08-10 while testing the CLIP semantic gate end-to-end: `recognition
	// --filter person` never even reached RecognitionUseCase).
	pflag.CommandLine.ParseErrorsWhitelist.UnknownFlags = true
	web := pflag.BoolP("web", "s", false, "Start the web server (API mode)")
	ws := pflag.BoolP("websocket", "w", false, "Start the WebSocket server")
	interactive := pflag.BoolP("interactive", "i", false, "Start in interactive mode")
	port := pflag.IntP("port", "p", 0, "Port to use for the server")
	pflag.Parse()

	appVersion := os.Getenv(application.AppVersionEnvName)
	if appVersion == "" {
		fmt.Printf("Error creating application: environment variable %s not set\n", application.AppVersionEnvName)
		return
	}

	// Build application options
	var options = []application.Option{
		application.Version(appVersion),
	}
	if appName := os.Getenv(application.AppNameEnvName); appName != "" {
		options = append(options, application.AppName(appName))
	}

	isCliMode := !*web && !*ws && !*interactive
	if isCliMode {
		// Console-friendly logger for CLI mode, also teed to logs/livesemantic.log
		// (withFileLogging, cmd/livesemantic/logging.go — not the vendored
		// zaplogger.SetZapLoggerForCLI, which only writes to the console).
		options = append(options, withFileLogging(true), application.WithCLIMode())
	} else {
		// Web-friendly (JSON) logger for web/websocket mode, same file teeing.
		options = append(options, withFileLogging(false))
	}

	// Create the engine with the appropriate options
	engine, err := application.New(options...)

	if err != nil {
		fmt.Println("Error creating application:", err)
		return
	}

	engine.Logger().Info(
		"Application started",
		map[string]interface{}{
			"appName":    engine.Name(),
			"appVersion": engine.Version(),
		},
	)

	streamingInput, windowOutput, notifier, objectDetector, semanticEncoder, trackerFactory, err := initDependencies()
	if err != nil {
		engine.Logger().Error("Failed to initialize dependencies", err)
		return
	}

	engine.Gracefull().Register("Stopping application gracefully", func() error {
		fmt.Println("🔒 Stopping application gracefully...")
		if notifier != nil {
			fmt.Println("Cleaning up notifier...")
		}
		if objectDetector != nil {
			fmt.Println("Cleaning up object detector resources...")
		}
		if semanticEncoder != nil {
			fmt.Println("Cleaning up semantic encoder resources...")
		}
		// Cleanup resources
		if notifier != nil {
			notifier.Cleanup()
		}
		if objectDetector != nil {
			objectDetector.Cleanup()
		}
		if semanticEncoder != nil {
			semanticEncoder.Cleanup()
		}
		fmt.Println("Application stopped gracefully.")
		return nil
	})

	useCases, err := uc.NewUseCase(engine.Context(), engine.Logger(), streamingInput, windowOutput, notifier, objectDetector, semanticEncoder, trackerFactory)
	if err != nil {
		engine.Logger().Error("Failed to create use cases", err)
		return
	}

	engine.Logger().Info("✅ Use cases initialized")

	// Decide which mode to start based on flags
	switch {
	case *web:
		serverPort := determinePort(*port, defaultWebPort)
		startWebServer(engine, useCases, serverPort)
	case *ws:
		serverPort := determinePort(*port, defaultWebsocketPort)
		startWebsocketServer(engine, useCases, serverPort)
	case *interactive:
		startInteractiveMode(engine, useCases)
	default:
		startCLIMode(engine, useCases)
	}
}

func initDependencies() (streamer.InputStream, streamer.OutputStream, notifier.AlertSender, inference.ObjectDetector, inference.SemanticEncoder, tracking.TrackerFactory, error) {
	cameraInput := input.NewCameraInput()
	windowOutput := output.NewWindowOutput()

	logNotifier := lognotifier.NewLogNotifier()
	objectDetector, err := yolo11s.New()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	semanticEncoder, err := clip.New()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}

	// KCF: reverted from CSRT on 2026-08-09 — CSRT's own drift-test numbers
	// were better (avg IoU 0.729 vs KCF 0.353 at maxTrackingDimension=320),
	// but in real usage CSRT's near-total refusal to self-report a lost
	// track (0 tracker_failures measured, see maxMissesBeforeLost's doc
	// comment in entities/track.go) made "object left frame" take several
	// seconds to resolve even after lowering maxMissesBeforeLost and
	// reanchorInterval — still not acceptable in practice. KCF fails more
	// honestly, which this project's loss-detection logic actually depends
	// on more than raw IoU accuracy. Revisit per TODO.md § B/F if KCF's
	// weaker accuracy at 320px becomes the bigger problem instead.
	trackerFactory := func() (tracking.ObjectTracker, error) {
		return gocvtracker.New(gocvtracker.KCF)
	}

	return cameraInput, windowOutput, logNotifier, objectDetector, semanticEncoder, trackerFactory, nil
}

func determinePort(flagPort, defaultPort int) int {
	if flagPort != 0 {
		return flagPort
	}
	return defaultPort
}

// startInteractiveMode starts the interactive mode
func startInteractiveMode(engine *application.Engine, useCases uc.UseCases) {
	engine.Logger().Info("💡 Starting in interactive mode")
	controller := cli.NewSurveyController(useCases, engine.Logger())
	if err := controller.Run(engine.Context()); err != nil {
		if err == context.Canceled {
			fmt.Println("Graceful shutdown triggered by interrupt.")
			return
		}
		// Send SIGTERM to self for graceful shutdown and wait briefly
		p, err := os.FindProcess(os.Getpid())
		if err == nil {
			_ = p.Signal(syscall.SIGTERM)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// startCLIMode starts the CLI mode
func startCLIMode(engine *application.Engine, useCases uc.UseCases) {
	engine.Logger().Info("💻 Starting in CLI mode")
	cmd.Execute(useCases, engine.Logger())
}

// startWebServer starts the web server in API mode
func startWebServer(engine *application.Engine, useCases uc.UseCases, port int) {
	engine.Logger().Info("🌐 Starting in Web API mode", map[string]interface{}{
		"port": port,
	})

	server := api.NewServer(useCases, engine.Logger(), port)
	if err := server.Start(); err != nil {
		engine.Logger().Error("Web server failed", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}
}

// startWebsocketServer starts the WebSocket server
func startWebsocketServer(engine *application.Engine, useCases uc.UseCases, port int) {
	engine.Logger().Info("🔗 Starting in WebSocket mode", map[string]interface{}{
		"port": port,
	})

	server := websocket.NewServer(useCases, engine.Logger(), port)
	if err := server.Start(); err != nil {
		engine.Logger().Error("WebSocket server failed", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}
}

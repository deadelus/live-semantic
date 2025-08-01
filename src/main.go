package main

import (
	"fmt"
	"live-semantic/src/domain"
	"live-semantic/src/domain/uc"
	"live-semantic/src/implementation/ai/yolo11s"
	"live-semantic/src/implementation/displayhandler/window"
	lognotifier "live-semantic/src/implementation/notifier/log-notifier"
	"live-semantic/src/implementation/sourceHandler/macOsCamera"
	"live-semantic/src/infrastructure/ai"
	displayhandler "live-semantic/src/infrastructure/displayHandler"
	"live-semantic/src/infrastructure/notifier"
	sourcehandler "live-semantic/src/infrastructure/sourceHandler"
	"live-semantic/src/transport/api"
	"live-semantic/src/transport/cli"
	"live-semantic/src/transport/cmd"
	"live-semantic/src/transport/websocket"
	"os"
	"runtime"

	"github.com/deadelus/go-clean-app/src/application"
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

	// Define and parse flags first to determine the mode
	web := pflag.BoolP("web", "s", false, "Start the web server (API mode)")
	ws := pflag.BoolP("websocket", "w", false, "Start the WebSocket server")
	interactive := pflag.BoolP("interactive", "i", false, "Start in interactive mode")
	port := pflag.IntP("port", "p", 0, "Port to use for the server")
	pflag.Parse()

	// Build application options
	var options = []application.Option{}

	isCliMode := !*web && !*ws && !*interactive
	if isCliMode {
		// Use a console-friendly logger for CLI mode
		options = append(options, application.SetZapLoggerForCLI(), application.WithCLIMode())
	} else {
		// Use a web-friendly logger for web or websocket mode
		options = append(options, application.SetZapLogger())
	}

	// Create the engine with the appropriate options
	engine, err := application.New(
		application.AppNameEnvName,
		application.SetVersionFromEnv(),
		options...,
	)
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

	videoHandler, displayHandler, notifier, ai, err := initDependencies(engine)
	if err != nil {
		engine.Logger().Error("Failed to initialize dependencies", err)
		return
	}

	useCases, err := uc.NewUseCase(engine.Context(), engine.Logger(), videoHandler, displayHandler, notifier, ai)
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

func initDependencies(engine *application.Engine) (sourcehandler.VideoHandler, displayhandler.DisplayHandler, notifier.Notifier, ai.AI, error) {
	videoSource, err := macOsCamera.NewMacOsCameraSource()
	if err != nil {
		engine.Logger().Error("Failed to create video source", err)
		return nil, nil, nil, nil, err
	}

	displayHandler := window.NewDisplayHandler()
	go func() {
		displayHandler.ProcessCommands()
	}()

	if displayHandler == nil {
		engine.Logger().Error("Display handler not initialized", domain.ErrNilDisplayHandler)
		return nil, nil, nil, nil, domain.ErrNilDisplayHandler
	}

	notifier := lognotifier.NewLogNotifier()
	if notifier == nil {
		engine.Logger().Error("Notifier not initialized", domain.ErrNilNotifier)
		return videoSource, displayHandler, nil, nil, domain.ErrNilNotifier
	}

	ai, err := yolo11s.NewNeuralNetwork()
	if err != nil {
		engine.Logger().Error("Failed to initialize Yolo11s AI model", err)
		return videoSource, displayHandler, nil, nil, err
	}

	return videoSource, displayHandler, notifier, ai, nil
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
	if err := controller.Run(); err != nil {
		engine.Logger().Error("Interactive CLI failed", err)
		os.Exit(1)
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

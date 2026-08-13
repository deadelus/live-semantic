package main

import (
	"context"
	"fmt"
	"live-semantic/internal/application/session"
	"live-semantic/internal/application/uc"
	"live-semantic/internal/implementation/inference/onnx/clip"
	"live-semantic/internal/implementation/inference/onnx/yolo11s"
	lognotifier "live-semantic/internal/implementation/notifier/log-notifier"
	"live-semantic/internal/implementation/storage/diskgallery"
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
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/deadelus/go-clean-app/v2/application"
	"github.com/joho/godotenv"
	"github.com/spf13/pflag"
)

const (
	defaultWebPort = 8080
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
	// -s/--web starts the unified backend (H1, TODO.md § H1): REST control
	// + WebSocket video on one server/one port — see
	// internal/transport/adapters/api. The previous standalone
	// -w/--websocket flag/mode is gone: it ran on a separate port with no
	// way to share a session with the REST side, which the GUI needs.
	web := pflag.BoolP("web", "s", false, "Start the unified web backend (REST control + WebSocket video)")
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

	isCliMode := !*web && !*interactive
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

	// Output adapter is mode-dependent: a GoCV window makes no sense
	// headless (web mode), and a WebSocket broadcaster has no window to
	// draw for CLI/interactive mode. See initDependencies's doc comment.
	// browserInput is likewise mode-dependent (nil outside web mode — see
	// its own doc comment).
	localInput, browserInput, streamingOutput, notifier, objectDetector, semanticEncoder, trackerFactoryBuilder, err := initDependencies(*web)
	if err != nil {
		engine.Logger().Error("Failed to initialize dependencies", err)
		return
	}

	// Shared across the single-session UseCase above and (web mode only)
	// every session.Manager-owned UseCase — TODO.md § H1 Multi-flux: a
	// reference gallery entry added from one flux must be usable as a
	// filter term on every other flux, not siloed per-session. Concrete
	// implementation/storage/diskgallery.Gallery constructed here, the
	// composition root — application/uc only ever sees it through the
	// storage.GalleryStorage port (dependency inversion, 2026-08-12).
	//
	// diskgallery, not inmemory — switched 2026-08-13: a user-curated
	// gallery of reference photos/embeddings is meant to survive a
	// restart, which a pure-RAM store never could (implementation/storage/
	// inmemory's doc comment explains why that adapter still exists, just
	// not wired in here anymore). "data/gallery" is a plain relative path,
	// same convention as "logs/livesemantic.log" elsewhere in this
	// composition root — not yet a flag/env var (nothing has needed one
	// so far), revisit if a real deployment needs the location
	// configurable.
	galleryRepo, err := diskgallery.New("data/gallery")
	if err != nil {
		engine.Logger().Error("Failed to initialize gallery storage", err)
		return
	}

	// collectionRepo: same sharing rationale as galleryRepo just above —
	// a Collection groups Terms across every flux, not siloed per-session
	// (docs/gui/design-brief.md § Bibliothèque, TODO.md § H1 "Modèle
	// Bibliothèque"). Stored under the same "data/gallery" root as the
	// Terms it references (diskgallery.NewCollections writes its own
	// collections.json there, doesn't touch per-Term directories) —
	// deliberately one root, not two, since a restore/backup of "the
	// Bibliothèque" should be a single directory copy.
	collectionRepo, err := diskgallery.NewCollections("data/gallery")
	if err != nil {
		engine.Logger().Error("Failed to initialize collection storage", err)
		return
	}

	// trackerFactoryBuilder() called fresh here — a dedicated
	// gocvtracker.Factory (and therefore frame cache) for this UseCase,
	// not shared with session.Manager's own sessions below. See
	// session.NewManager's doc comment for why sharing one across
	// concurrently-running sessions used to SIGSEGV.
	useCases, err := uc.NewUseCase(engine.Context(), engine.Logger(), localInput, browserInput, streamingOutput, notifier, objectDetector, semanticEncoder, trackerFactoryBuilder(), galleryRepo, collectionRepo)
	if err != nil {
		engine.Logger().Error("Failed to create use cases", err)
		return
	}

	engine.Logger().Info("✅ Use cases initialized")

	// Registered after useCases exists (moved 2026-08-11, real crash found
	// while testing — docs/adr/clip-backend.md § 15, TODO.md § H1): a
	// SIGTERM used to destroy objectDetector/semanticEncoder's shared ONNX
	// sessions immediately, even while Recognize's detection goroutine was
	// still mid-EncodeImage/AnalyzeFrame on those exact sessions (SIGSEGV
	// inside the CGo call — worse the slower a reanchor cycle is, e.g.
	// with CLIP semantic filter terms in play). useCases.
	// StopRecognition() unsticks any session still blocked reading
	// frames; useCases.WaitRecognition() blocks until it has genuinely
	// returned — only then is it safe to destroy the sessions it was
	// using.
	engine.Gracefull().Register("Stopping application gracefully", func() error {
		fmt.Println("🔒 Stopping application gracefully...")
		useCases.StopRecognition()
		useCases.WaitRecognition()
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

	// Decide which mode to start based on flags
	switch {
	case *web:
		// initDependencies(true) always returns a *output.WebSocketOutput
		// in this branch — asserted here (rather than threading the
		// concrete type through NewUseCase's interface-typed parameter)
		// so the api.Server can register WebSocket clients against the
		// exact instance Recognize renders to.
		wsOutput, ok := streamingOutput.(*output.WebSocketOutput)
		if !ok {
			engine.Logger().Error("Web mode requires a WebSocketOutput, got something else", nil)
			return
		}
		serverPort := determinePort(*port, defaultWebPort)
		sessionManager := session.NewManager(
			sessionInputFactory,
			func() streamer.OutputStream { return output.NewWebSocketOutput() },
			engine.Logger(),
			notifier,
			objectDetector,
			semanticEncoder,
			trackerFactoryBuilder,
			galleryRepo,
			collectionRepo,
		)
		startWebServer(engine, useCases, wsOutput, browserInput, sessionManager, serverPort)
	case *interactive:
		startInteractiveMode(engine, useCases)
	default:
		startCLIMode(engine, useCases)
	}
}

// initDependencies wires the concrete adapters. webMode picks the output
// adapter: a GoCV window (CLI/interactive, a local process with a display)
// or a WebSocketOutput broadcaster (web, headless server — see
// implementation/streamer/output/websocket_output.go). It also decides
// whether a *input.BrowserInput exists at all (TODO.md § H2 "capture
// caméra navigateur") — only web mode has an /ws/ingest endpoint for one
// to receive frames from, so it stays nil in CLI/interactive mode rather
// than being built unused. Everything else is shared regardless of mode.
func initDependencies(webMode bool) (localInput streamer.InputStream, browserInput *input.BrowserInput, streamingOutput streamer.OutputStream, alertSender notifier.AlertSender, objectDetector inference.ObjectDetector, semanticEncoder inference.SemanticEncoder, trackerFactoryBuilder func() tracking.TrackerFactory, err error) {
	// Device 0 hardcoded for now — configurable at the adapter level
	// (TODO.md § H1), not yet exposed via CLI/REST (source selection is
	// part of the GUI "Ajouter une source" flow, § H1 Multi-flux/H2).
	localInput = input.NewCameraInput(0)

	if webMode {
		streamingOutput = output.NewWebSocketOutput()
		browserInput = input.NewBrowserInput()
	} else {
		streamingOutput = output.NewWindowOutput()
	}

	alertSender = lognotifier.NewLogNotifier()
	objectDetector, err = yolo11s.New()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	semanticEncoder, err = clip.New()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
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
	// gocvtracker.Factory implements tracking.TrackerFactory (a real
	// interface, 2026-08-12 — see its own doc comment) for the fixed KCF
	// choice above. A *builder*, not a shared instance, since 2026-08-13:
	// each call must return a brand new Factory (own frame-conversion
	// cache) — see session.NewManager's doc comment for the SIGSEGV that
	// happened when every session shared one Factory/cache.
	trackerFactoryBuilder = func() tracking.TrackerFactory {
		return gocvtracker.NewFactory(gocvtracker.KCF)
	}

	return localInput, browserInput, streamingOutput, alertSender, objectDetector, semanticEncoder, trackerFactoryBuilder, nil
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

// sessionInputFactory builds the concrete streamer.InputStream for a
// session.Manager session, one Source.Kind at a time — the only place
// (besides initDependencies) allowed to know about the concrete
// implementation/streamer/input adapters, per session.go's dependency-
// inversion rule. Package-level rather than a closure literal so it's
// reusable/testable in isolation if session.Manager ever needs more than
// one Manager instance.
func sessionInputFactory(src session.Source) (streamer.InputStream, error) {
	switch src.Kind {
	case "local", "":
		return input.NewCameraInput(src.Device), nil
	case "file":
		return input.NewFileInput(src.URI), nil
	case "browser":
		return input.NewBrowserInput(), nil
	case "webrtc":
		return input.NewWebRTCInput(), nil
	default:
		return nil, fmt.Errorf("unknown session source kind %q", src.Kind)
	}
}

// startWebServer starts the unified REST + WebSocket backend (H1,
// TODO.md § H1). wsOutput is registered as the server's FrameBroadcaster
// so /ws clients receive the frames RecognitionUseCase renders to it.
// sessionManager backs the parallel multi-flux REST/WS surface (§ H1
// "Multi-flux") — see api.NewServer's doc comment.
func startWebServer(engine *application.Engine, useCases uc.UseCases, wsOutput *output.WebSocketOutput, browserInput *input.BrowserInput, sessionManager *session.Manager, port int) {
	engine.Logger().Info("🌐 Starting in Web mode (REST + WebSocket)", map[string]interface{}{
		"port": port,
	})

	server := api.NewServer(useCases, wsOutput, browserInput, sessionManager, engine.Logger(), port)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Found 2026-08-11 testing the SIGSEGV fix above (docs/adr/clip-backend.md
	// § 15): the graceful shutdown hooks (camera/ORT cleanup) used to run
	// to completion — logged "Shutdown is over." — but the process never
	// actually exited: gin's router.Run() blocks the goroutine it's called
	// from forever (it's a thin wrapper over http.ListenAndServe, which
	// only returns on a listener error) and doesn't expose the underlying
	// *http.Server to call Shutdown(ctx) on, so nothing ever unblocked it.
	// A SIGKILL was the only way out. Exiting explicitly here once
	// Gracefull().Done() fires is the straightforward fix — there's no
	// cleaner handle to the listener available through gin's API as used
	// here.
	select {
	case err := <-errCh:
		if err != nil {
			engine.Logger().Error("Web server failed", map[string]interface{}{
				"error": err.Error(),
			})
			os.Exit(1)
		}
	case <-engine.Gracefull().Done():
		os.Exit(0)
	}
}

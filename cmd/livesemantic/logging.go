package main

import (
	"fmt"
	"os"

	"github.com/deadelus/go-clean-app/v2/application"
	"github.com/deadelus/go-clean-app/v2/logger/zaplogger"
	"go.uber.org/zap"
)

// logFilePath is where every run writes its structured logs (JSON, one
// object per line — same format zap already writes to the console), so
// that debugging a real run (webcam/window, no way to pipe/copy from here)
// doesn't depend on the user pasting terminal output by hand. Truncated
// at the start of each run, not appended across runs — see
// withFileLogging's own doc comment.
const logFilePath = "logs/livesemantic.log"

// withFileLogging mirrors zaplogger.SetZapLoggerForCLI/SetZapLogger (both
// vendored, not modified here) but tees every log line to logFilePath in
// addition to the console. devMode selects zap.NewDevelopmentConfig
// (human-readable, CLI) vs zap.NewProductionConfig (JSON, web/websocket) —
// same split the two vendored options make.
//
// Truncated at the start of every run (2026-08-14) — zap's own file sink
// (config.OutputPaths below) always opens in append mode, no truncate
// option exists on it, so a long-lived local dev loop (start/stop the
// server dozens of times a day) used to grow this file forever (a 40k+
// line log was reported in exactly this environment) with nothing but
// old, already-debugged runs in it. Local dev only needs the *current*
// run's log to grep/tail — truncating here, once, before zap opens its
// own append-mode handle onto the now-empty file, gets that without
// needing a rotation library (lumberjack etc., not a dependency this
// project has any other use for) for what's still a single-file, single-
// machine log.
func withFileLogging(devMode bool) application.Option {
	return func(e *application.Engine) {
		if err := os.MkdirAll("logs", 0o755); err != nil {
			fmt.Println("Could not create logs/ directory, file logging disabled:", err)
		}
		if f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644); err != nil {
			fmt.Println("Could not truncate", logFilePath, "— logging will append to the previous run:", err)
		} else {
			f.Close()
		}

		var config zap.Config
		var opts []zap.Option
		if devMode {
			config = zap.NewDevelopmentConfig()
			opts = append(opts, zap.WithCaller(false))
		} else {
			config = zap.NewProductionConfig()
		}
		opts = append(opts, zap.AddStacktrace(zap.PanicLevel))

		config.OutputPaths = append(config.OutputPaths, logFilePath)

		l, err := config.Build(opts...)
		if err != nil {
			panic(fmt.Errorf("failed to create zap logger with file output: %w", err))
		}

		zlogger, closeLogger, _ := zaplogger.GetFromExternalLogger(l)
		e.SetLogger(zlogger)

		if err := e.Gracefull().Register("zaplogger-file", closeLogger); err != nil {
			panic(fmt.Errorf("failed to register zap logger for graceful shutdown: %w", err))
		}
	}
}

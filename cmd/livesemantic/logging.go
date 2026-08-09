package main

import (
	"fmt"
	"os"

	"github.com/deadelus/go-clean-app/v2/application"
	"github.com/deadelus/go-clean-app/v2/logger/zaplogger"
	"go.uber.org/zap"
)

// logFilePath is where every run appends its structured logs (JSON, one
// object per line — same format zap already writes to the console), so
// that debugging a real run (webcam/window, no way to pipe/copy from here)
// doesn't depend on the user pasting terminal output by hand.
const logFilePath = "logs/livesemantic.log"

// withFileLogging mirrors zaplogger.SetZapLoggerForCLI/SetZapLogger (both
// vendored, not modified here) but tees every log line to logFilePath in
// addition to the console. devMode selects zap.NewDevelopmentConfig
// (human-readable, CLI) vs zap.NewProductionConfig (JSON, web/websocket) —
// same split the two vendored options make.
func withFileLogging(devMode bool) application.Option {
	return func(e *application.Engine) {
		if err := os.MkdirAll("logs", 0o755); err != nil {
			fmt.Println("Could not create logs/ directory, file logging disabled:", err)
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

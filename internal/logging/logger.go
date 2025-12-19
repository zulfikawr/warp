package logging

import (
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger  *zap.Logger
	sugar   *zap.SugaredLogger
	once    sync.Once
	initErr error
	level   = zap.NewAtomicLevelAt(zapcore.WarnLevel) // Default to warn level
)

// initLogger performs lazy initialization of the logger
func initLogger() {
	once.Do(func() {
		// Create production logger with console encoding for better readability
		config := zap.NewProductionConfig()
		config.Encoding = "console"
		config.DisableStacktrace = true
		config.DisableCaller = true
		config.Level = level

		var err error
		logger, err = config.Build()
		if err != nil {
			// Fallback to no-op logger instead of panicking
			logger = zap.NewNop()
			initErr = err
			fmt.Fprintf(os.Stderr, "Warning: failed to initialize logger: %v\n", err)
		}
		sugar = logger.Sugar()
	})
}

// SetLevel sets the logging level
// verbosity: 0 = warn, 1 = info (-v), 2 = debug (-vv), 3+ = debug with caller (-vvv)
func SetLevel(verbosity int) {
	var lvl zapcore.Level
	switch verbosity {
	case 0:
		lvl = zapcore.WarnLevel
	case 1:
		lvl = zapcore.InfoLevel
	case 2:
		lvl = zapcore.DebugLevel
	default:
		lvl = zapcore.DebugLevel
	}
	level.SetLevel(lvl)
}

// GetLogger returns the structured logger
func GetLogger() *zap.Logger {
	initLogger()
	return logger
}

// GetSugar returns the sugared logger for easier use
func GetSugar() *zap.SugaredLogger {
	initLogger()
	return sugar
}

// Sync flushes any buffered log entries
func Sync() {
	initLogger()
	_ = logger.Sync()
	_ = sugar.Sync()
}

// InitError returns any error that occurred during logger initialization
func InitError() error {
	initLogger()
	return initErr
}

// Info logs an informational message
func Info(msg string, fields ...zap.Field) {
	initLogger()
	logger.Info(msg, fields...)
}

// Warn logs a warning message
func Warn(msg string, fields ...zap.Field) {
	initLogger()
	logger.Warn(msg, fields...)
}

// Error logs an error message
func Error(msg string, fields ...zap.Field) {
	initLogger()
	logger.Error(msg, fields...)
}

// Debug logs a debug message
func Debug(msg string, fields ...zap.Field) {
	initLogger()
	logger.Debug(msg, fields...)
}

// Infof logs a formatted informational message (sugared)
func Infof(template string, args ...interface{}) {
	initLogger()
	sugar.Infof(template, args...)
}

// Warnf logs a formatted warning message (sugared)
func Warnf(template string, args ...interface{}) {
	initLogger()
	sugar.Warnf(template, args...)
}

// Errorf logs a formatted error message (sugared)
func Errorf(template string, args ...interface{}) {
	initLogger()
	sugar.Errorf(template, args...)
}

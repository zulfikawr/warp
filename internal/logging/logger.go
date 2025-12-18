package logging

import (
	"go.uber.org/zap"
)

var logger *zap.Logger
var sugar *zap.SugaredLogger

func init() {
	// Create production logger with console encoding for better readability
	config := zap.NewProductionConfig()
	config.Encoding = "console"
	config.DisableStacktrace = true
	config.DisableCaller = true
	
	var err error
	logger, err = config.Build()
	if err != nil {
		panic(err)
	}
	sugar = logger.Sugar()
}

// GetLogger returns the structured logger
func GetLogger() *zap.Logger {
	return logger
}

// GetSugar returns the sugared logger for easier use
func GetSugar() *zap.SugaredLogger {
	return sugar
}

// Sync flushes any buffered log entries
func Sync() {
	_ = logger.Sync()
	_ = sugar.Sync()
}

// Info logs an informational message
func Info(msg string, fields ...zap.Field) {
	logger.Info(msg, fields...)
}

// Warn logs a warning message
func Warn(msg string, fields ...zap.Field) {
	logger.Warn(msg, fields...)
}

// Error logs an error message
func Error(msg string, fields ...zap.Field) {
	logger.Error(msg, fields...)
}

// Debug logs a debug message
func Debug(msg string, fields ...zap.Field) {
	logger.Debug(msg, fields...)
}

// Infof logs a formatted informational message (sugared)
func Infof(template string, args ...interface{}) {
	sugar.Infof(template, args...)
}

// Warnf logs a formatted warning message (sugared)
func Warnf(template string, args ...interface{}) {
	sugar.Warnf(template, args...)
}

// Errorf logs a formatted error message (sugared)
func Errorf(template string, args ...interface{}) {
	sugar.Errorf(template, args...)
}

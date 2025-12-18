package logging

import (
	"testing"
	
	"go.uber.org/zap"
)

func TestLoggerInitialization(t *testing.T) {
	if logger == nil {
		t.Fatal("Logger is not initialized")
	}
}

func TestInfoLogging(t *testing.T) {
	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Info logging panicked: %v", r)
		}
	}()
	
	Info("test message", zap.String("key", "value"))
	Infof("test formatted: %s", "value")
}

func TestWarnLogging(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Warn logging panicked: %v", r)
		}
	}()
	
	Warn("test warning", zap.String("key", "value"))
	Warnf("test formatted warning: %s", "value")
}

func TestErrorLogging(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Error logging panicked: %v", r)
		}
	}()
	
	Error("test error", zap.String("key", "value"))
	Errorf("test formatted error: %s", "value")
}

func TestDebugLogging(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Debug logging panicked: %v", r)
		}
	}()
	
	Debug("test debug", zap.String("key", "value"))
}

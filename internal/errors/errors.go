package errors

import (
	"errors"
	"fmt"
	"strings"
)

// UserError represents an error with user-friendly message and suggestions
type UserError struct {
	Message     string   // User-friendly error message
	Suggestions []string // Possible solutions
	Err         error    // Underlying error (can be nil)
}

// Error implements the error interface
func (e *UserError) Error() string {
	var sb strings.Builder
	sb.WriteString(e.Message)

	if len(e.Suggestions) > 0 {
		sb.WriteString("\n\nPossible solutions:")
		for _, suggestion := range e.Suggestions {
			sb.WriteString("\n  • ")
			sb.WriteString(suggestion)
		}
	}

	if e.Err != nil {
		sb.WriteString("\n\nTechnical details: ")
		sb.WriteString(e.Err.Error())
	}

	return sb.String()
}

// Unwrap returns the underlying error
func (e *UserError) Unwrap() error {
	return e.Err
}

// NewUserError creates a new user-friendly error
func NewUserError(message string, suggestions []string, err error) *UserError {
	return &UserError{
		Message:     message,
		Suggestions: suggestions,
		Err:         err,
	}
}

// IsUserError checks if an error is a UserError
func IsUserError(err error) bool {
	var userErr *UserError
	return errors.As(err, &userErr)
}

// Common error constructors for typical scenarios

// ConnectionError creates an error for connection failures
func ConnectionError(url string, err error) error {
	return NewUserError(
		fmt.Sprintf("Failed to connect to %s", url),
		[]string{
			"Check if the server is running",
			"Verify the URL/token is correct",
			"Ensure both devices are on the same network",
			"Check firewall settings",
		},
		err,
	)
}

// FileNotFoundError creates an error for missing files
func FileNotFoundError(path string, err error) error {
	return NewUserError(
		fmt.Sprintf("File not found: %s", path),
		[]string{
			"Check if the file path is correct",
			"Verify you have read permissions",
			"Ensure the file still exists",
		},
		err,
	)
}

// FileExistsError creates an error for existing files
func FileExistsError(path string) error {
	return NewUserError(
		fmt.Sprintf("File already exists: %s", path),
		[]string{
			"Use --force flag to overwrite",
			"Specify a different output path with --output",
			"Rename or move the existing file",
		},
		nil,
	)
}

// PermissionError creates an error for permission issues
func PermissionError(operation, path string, err error) error {
	return NewUserError(
		fmt.Sprintf("Permission denied: cannot %s %s", operation, path),
		[]string{
			"Check file/directory permissions",
			"Try running with appropriate privileges",
			"Ensure the directory is writable",
		},
		err,
	)
}

// DiskSpaceError creates an error for insufficient disk space
func DiskSpaceError(required, available int64) error {
	return NewUserError(
		fmt.Sprintf("Insufficient disk space (need %d bytes, have %d bytes)", required, available),
		[]string{
			"Free up disk space",
			"Choose a different destination directory",
			"Delete unnecessary files",
		},
		nil,
	)
}

// InvalidURLError creates an error for malformed URLs
func InvalidURLError(url string, err error) error {
	return NewUserError(
		fmt.Sprintf("Invalid URL: %s", url),
		[]string{
			"Check the URL format (should be http://host:port/d/token or http://host:port/u/token)",
			"Verify the token is correct",
			"Try copying the URL again from the server",
		},
		err,
	)
}

// ConfigError creates an error for configuration issues
func ConfigError(message string, err error) error {
	return NewUserError(
		message,
		[]string{
			"Check your config file at ~/.config/warp/warp.yaml",
			"Verify the YAML syntax is correct",
			"Try running 'warp config show' to see current settings",
			"Delete the config file to reset to defaults",
		},
		err,
	)
}

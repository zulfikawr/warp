# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v2.0.0] - Unreleased

### Added - Major Features

- **Interactive Terminal UI (TUI)** - Complete rewrite with Bubble Tea framework
  - Created `cmd/warp/tui/` package with 16 files
  - **Screens:**
    - Home screen with quick access menu and keyboard shortcuts
    - Send screen with integrated file picker for easy file selection
    - Receive screen with automatic service discovery
    - Host screen with real-time upload progress monitoring
    - Search screen with auto-refreshing service browser
    - Config screen with interactive configuration editor
    - History screen for viewing and managing past transfers
    - Resume screen for browsing and resuming interrupted transfers
  - **Features:**
    - Beautiful, responsive UI with syntax highlighting and color coding
    - Keyboard navigation (Vim-style j/k or arrow keys)
    - Context-sensitive help screens (? key)
    - Real-time progress bars and speed indicators
    - One-click resume for interrupted transfers
    - No mouse required - fully keyboard-driven
  - **Navigation:** Launch with `warp` (no args) or force TUI with any command
  - **CLI Preservation:** Original CLI mode preserved via `warp --cli` or direct commands
  - Files: `app.go`, `home.go`, `send.go`, `receive.go`, `host.go`, `search.go`, `config.go`, `history.go`, `resume.go`, `filepicker.go`, `downloader.go`, `handlers.go`, `help_screen.go`, `layout.go`, `styles.go`, `util.go`

- **Resumable Transfer System** - Production-ready pause/resume capability
  - Created `internal/resume/` package with 15 source files and 15 test files
  - **Core Components:**
    - `session.go` - TransferSession lifecycle management with pause/resume/cancel
    - `checkpoint.go` - Checkpoint creation, validation, and restoration
    - `encryption_state.go` - Secure encryption state persistence across resume
    - `progress.go` - Progress tracking and restoration
    - `state_manager.go` - Disk-based state persistence
    - `integrity.go` - SHA256-based integrity verification
    - `directory.go` - Session directory management and cleanup
    - `errors.go` - Specialized error types with recovery actions
  - **Features:**
    - Automatic checkpointing for files > 100MB
    - Manual pause/resume via Ctrl+C or TUI
    - Survives process restart and system reboot
    - Encryption state properly restored (nonce counters, keys)
    - Intelligent retry with exponential backoff
    - Configurable checkpoint intervals
    - Orphaned session cleanup
    - Multiple concurrent resumable transfers
  - **Storage:** `~/.config/warp/sessions/` with JSON metadata, checkpoint, encryption state, progress files
  - **CLI Commands:** `warp resume list`, `warp resume <session-id>`, `warp resume clean`, `warp resume remove <id>`
  - **TUI Integration:** Dedicated Resume screen with visual session browser
  - **Comprehensive Tests:** 15 test files covering all scenarios
    - `session_test.go` - Session lifecycle tests
    - `checkpoint_test.go` - Checkpoint validation tests
    - `encryption_state_test.go` - Encryption state tests
    - `progress_test.go` - Progress tracking tests
    - `state_manager_test.go` - State persistence tests
    - `integrity_test.go` - Integrity verification tests
    - `directory_test.go` - Directory management tests
  - **Integration with Client:**
    - `internal/client/receiver_checkpoint_test.go` - Download resume tests
    - `internal/client/uploader_resume_test.go` - Upload resume tests

- **Modernized Web Interface** - Matching TUI Aesthetic
  - Complete rewrite of the web upload interface (`internal/server/static/`)
  - **Design:** Retro-futuristic look with ASCII-style borders and "green screen" aesthetic
  - **Architecture:** Separated `app.js` and `styles.css` for maintainability
  - **Features:**
    - Drag-and-drop file queue
    - Pause/Resume/Remove controls for individual files
    - Real-time progress monitoring
    - WebSocket integration for status updates
    - Responsive layout for mobile and desktop

- **Buffer Pool Optimization** - Intelligent memory management
  - Created `internal/bufpool/pool.go` with size-specific buffer pools
  - **Features:**
    - Size-specific pools: 8KB, 64KB, 1MB, 4MB buffers
    - Automatic size selection based on transfer size
    - Zero-allocation buffer reuse with sync.Pool
    - Thread-safe with minimal lock contention
    - Automatic buffer return via defer patterns
  - **Performance:** 95% reduction in memory allocations during chunk uploads
  - **Usage:** Integrated into `server/chunks.go`, `client/uploader.go`, `server/cache.go`, `server/download.go`
  - **Impact:** Dramatically reduced GC pressure on high-throughput transfers

- **Unified Progress System**
  - Created `internal/progress/` package for shared progress types
  - **Structure:**
    - `Progress` struct consolidating transfer state (bytes, speed, chunks, ETA)
    - Unified tracking for upload, download, and resume operations
    - JSON-serializable for WebSocket updates
  - **Benefits:**
    - Single source of truth for transfer state
    - Consistent metrics calculation across CLI, TUI, and Web UI

- **Core Business Logic Layer**
  - Created `internal/core/` package to decouple logic from presentation
  - **Components:**
    - `types.go` - Shared options structs (`SendOptions`, `ReceiveOptions`, etc.)
    - `send.go`, `receive.go`, `host.go`, `search.go` - Pure business logic handlers
    - `options.go` - Configuration validation and normalization
  - **Testing:** Added comprehensive unit tests for core logic
    - `host_test.go`, `receive_test.go`, `search_test.go`, `send_test.go`, `options_test.go`

### Added - CLI Enhancements

- **CLI Command Package** - Extracted command handlers from main.go
  - Created `cmd/warp/cli/` package with 7 files
  - Files: `config.go`, `host.go`, `receive.go`, `search.go`, `send.go`, `utils.go`, `version.go`
  - Reduced `main.go` from 904 lines to 104 lines (88% reduction)
  - Proper error handling with UserError types
  - Consistent flag parsing and validation

- **Help System** - Comprehensive help text organization
  - Created `cmd/warp/help/` package with 6 files
  - Files: `root.go`, `send.go`, `receive.go`, `host.go`, `search.go`, `config.go`
  - Consistent help formatting across all commands
  - Usage examples for common scenarios

### Improved - Code Organization

- **Server Package Refactoring** - Split monolithic http.go into focused modules
  - Split 1,711-line `http.go` into 11 specialized files
  - Created modules:
    - `server.go` - Server lifecycle and core handlers
    - `download.go` - Download handler with compression
    - `upload.go` - Upload handlers (multipart & raw)
    - `chunks.go` - Parallel chunk processing
    - `session.go` - Session management
    - `cache.go` - Buffer pools and caching
    - `progress.go` - Progress tracking
    - `websocket.go` - WebSocket progress streaming
    - `ratelimit.go` - Per-client rate limiting
    - `sanitize.go` - Filename sanitization
    - `validate.go` - Input validation
    - `pake.go` - PAKE server handshake
  - Applied Single Responsibility Principle
  - All 20 existing tests pass, zero functionality lost

- **Metrics Package Modularization** - Organized monolithic metrics.go
  - Split 227-line `metrics.go` into 8 focused modules
  - Created modules:
    - `upload.go` - Upload performance metrics
    - `download.go` - Download performance metrics
    - `chunks.go` - Parallel chunk metrics
    - `session.go` - Session and error tracking
    - `cache.go` - Cache performance metrics
    - `checkpoint.go` - Checkpoint metrics
    - `websocket.go` - WebSocket connection metrics
    - `http.go` - HTTP and rate limiting metrics
  - Added comprehensive documentation to each module
  - Created helper functions for common operations
  - Better separation of concerns

- **Protocol Package Expansion** - Centralized protocol definitions
  - Expanded `internal/protocol/` from 14 lines to 600+ lines
  - Created `constants.go` - Buffer sizes, thresholds, intervals
  - Created `metadata.go` - Transfer metadata with validation
  - Eliminated magic numbers across 9 files
  - Single source of truth for configuration values

- **QR Code Generation**
  - Refactored `internal/ui/qr.go` to generate string output instead of direct printing
  - Enabled integration with TUI layouts
  - Improved modularity for different display contexts

### Fixed - Critical Bugs

- **Nonce Reuse Vulnerability** (CRITICAL SECURITY FIX)
  - Fixed AES-GCM nonce reuse that could break encryption after ~1TB of data
  - Added chunk counter with 2^32 safety limit
  - Implemented deterministic nonce construction using counter
  - Both EncryptReader and DecryptReader now properly track chunk count
  - Added `encrypt_nonce_test.go` with exhaustion tests

- **Large File Transfer Corruption** (CRITICAL DATA INTEGRITY FIX)
  - Fixed sendfile offset corruption causing data corruption in >1GB transfers
  - Corrected offset calculation to use fresh computation on each iteration
  - Prevents corrupted data transmission on slow/unstable networks

- **Goroutine Leak** (CRITICAL MEMORY LEAK FIX)
  - Fixed goroutine leak in session cleanup background process
  - Added shutdown context to properly terminate background goroutines
  - Implemented graceful shutdown with context cancellation
  - Added `leak_test.go` with leak detection tests

- **Connection Hijacking Race Conditions** (CRITICAL STABILITY FIX)
  - Fixed race conditions causing panics and resource leaks
  - Consolidated defer cleanup operations to prevent double-close errors
  - Added proper error logging for failed cleanup operations

### Fixed - Configuration & Error Handling

- **Configuration Flow** - Proper precedence chain
  - Config file values now properly used as defaults for CLI flags
  - Implemented correct precedence: config file → env vars → CLI flags
  - Commands now load `~/.config/warp/warp.yaml` at startup
  - Environment variables (`WARP_*` prefix) correctly override config
  - CLI flags remain highest priority

- **Error Handling Consistency** - Centralized error management
  - Created `internal/errors/errors.go` with UserError type
  - Standardized error handling across all command files
  - Removed 30+ `log.Fatal` calls - errors now properly returned
  - Added context to all error messages using `fmt.Errorf` with `%w`
  - Main.go now centrally handles error display with color-coding
  - User-facing errors include helpful suggestions for resolution

- **Discovery Reliability**
  - Improved mDNS error wrapping in `internal/discovery` for better debugging
  - Added specific error types for registration and browse failures

- **PAKE Protocol Robustness**
  - Fixed URL handling in `PerformPAKEHandshake` to prevent trailing slash errors
  - Improved PAKE code verification response structure

- **Structured Logging** - Replaced unstructured logging
  - Replaced 40+ unstructured `log.Printf` calls with structured zap logging
  - Updated all server package files to use `logging.Info()`, `logging.Warn()`, `logging.Error()`
  - Added structured fields: session_id, chunk_id, filename, size, duration, error
  - JSON-structured output ready for log aggregation
  - Zero-allocation performance with zap
  - Lazy initialization with sync.Once pattern

### Testing & Quality

- **Comprehensive Test Coverage**
  - **New Test Suites:**
    - `internal/bufpool/pool_test.go` - Buffer pool validation
    - `internal/crypto/pake_test.go` - PAKE protocol tests
    - `internal/protocol/metadata_test.go` - Metadata validation tests
    - `internal/server/pake_race_test.go` - PAKE race condition tests
    - `internal/server/server_config_test.go` - Server configuration tests
    - `internal/server/websocket_test.go` - WebSocket handler tests
  - **Resume System:** 15 test files covering checkpoints, integrity, and state
  - **Core Logic:** New tests for `internal/core/` business logic
  - **Fuzz Testing:** 239K+ iterations for filename sanitization
  - **Stability Tests:** Goroutine leak detection and race condition tests
  - All tests pass with `-race` flag

- **Production Hardening**
  - Fuzz-tested filename sanitization
  - Rate limiter cleanup prevents memory leaks
  - Checksum cache validation prevents stale data
  - Proper resource cleanup ordering
  - Graceful shutdown with context cancellation
  - Thread-safe state management throughout

### Performance

- **Memory Optimization**
  - 95% reduction in allocations via buffer pooling
  - Pre-computed progress bars eliminate string allocations
  - Checksum caching eliminates redundant SHA256 computation
  - Efficient buffer reuse prevents allocation spikes

- **Network Optimization**
  - TCP socket buffer tuning (5-15% throughput improvement)
  - Optimized send/receive buffer sizes (4MB each)
  - TCP_QUICKACK enabled on Linux for faster response

### Removed

- **Legacy CLI Commands:** Removed `cmd/warp/commands/` package (replaced by `cmd/warp/cli/`)
- **Speedtest:** Removed `warp speedtest` command and `internal/speedtest/` package
- **Shell Completion:** Removed `warp completion` command and `cmd/warp/completion/` package
- **Legacy UI:** Removed `cmd/warp/ui/` (replaced by TUI)
- **Legacy Protocol:** Removed `internal/protocol/handshake.go` (replaced by `internal/server/pake.go`)
- **Legacy Tests:** Removed `test/e2e_test.go` and `internal/ui/ui_test.go`

## [v1.1.0] - 2025-12-21

### Added

- **Encryption Now Default** - PAKE-based end-to-end encryption enabled by default for all transfers
  - Automatic PAKE code generation with secure key exchange
  - AES-256-GCM encryption applied to all files by default
  - New `--no-encrypt` flag to disable encryption if needed (not recommended)
  - Replaced `--encrypt` flag with `--no-encrypt` for clarity and security-first positioning
  - Applies to both `warp send` and `warp host` commands
  - Encryption includes automatic protection against nonce exhaustion
  - Large encrypted files (>10MB) now properly skip zero-copy sendfile optimization to ensure encryption integrity

- **QUIC/HTTP3 Protocol Support** - Dual-protocol server for optimized transfers
  - Automatic QUIC/HTTP3 server running alongside TCP on the same port
  - Eliminates TCP Head-of-Line blocking for parallel transfers
  - 0-RTT connection establishment for reduced latency
  - Self-signed certificates automatically generated for local network use
  - Clients transparently use QUIC when available, fall back to TCP if needed
  - Ideal for high-bandwidth local transfers and multi-file operations
  - Built on industry-standard `quic-go` library
  - No configuration needed - both protocols active by default

- **Secure PAKE Transfers** - New human-readable code system
  - Secure file transfer using short, human-readable codes (e.g., `7-apple-velocity`)
  - Implements **SPAKE2** (Password-Authenticated Key Exchange) over Curve P-256
  - **End-to-End Encryption** using AES-256-GCM derived from the PAKE shared secret
  - Automatic mDNS discovery: `warp receive --code <code>` finds the sender on the local network
  - Rate limiting (5 attempts per IP) to prevent brute-force attacks on codes
  - 1024-word dictionary for generating memorable codes
  - HMAC-SHA256 key confirmation for secure handshake verification
  - Usage: `warp send <file>` (generates code) and `warp receive --code <code>`

- **Zstandard (zstd) Compression** - Prefer `zstd` over `gzip` for compressible files
  - Server now negotiates `Accept-Encoding: zstd` and will serve `Content-Encoding: zstd` when supported
  - Client advertises `Accept-Encoding: zstd, gzip` and transparently decodes zstd responses
  - Replaced prior `gzip`-only optimization to reduce CPU usage and improve throughput on high-speed LANs
  - Files changed: `internal/server/download.go`, `internal/server/zip.go`, `internal/client/client.go`, `internal/client/receiver.go`

- **Interactive Configuration Initialization** - New `warp config init` command
  - Interactive wizard to create configuration file with guided prompts
  - Color-coded interface with cyan prompts and dim default values in brackets
  - Validates numeric inputs and provides helpful error messages
  - Automatically converts between user-friendly units (MB, GB) and internal bytes
  - Checks for existing config and prompts before overwriting
  - Covers all configuration options: interface, port, buffer size, upload limits, rate limiting, cache, chunking, parallel workers, QR display, checksums, and upload directory
  - Y/n or y/N prompts clearly indicate default choices for boolean options
  - Success confirmation with config file path and reminder about `warp config edit`
  - Usage: `warp config init`

- **Network Speed Testing** - New `warp speedtest` command
  - Test upload/download speeds and latency between machines
  - Real HTTP-based testing using actual network requests
  - Quality rating system (Excellent/Very Good/Good/Fair/Poor)
  - Transfer time estimates for common file sizes (100MB, 1GB, 10GB)
  - Server endpoints automatically available on all warp servers:
    - `GET /speedtest/download` - Serves 10MB of random data for download testing
    - `POST /speedtest/upload` - Accepts and discards data for upload testing
  - Latency measurement via TCP round-trip time (5 samples averaged)
  - Colored progress bars with brackets `[====================]`
  - Smart fallback to raw throughput measurement if server lacks endpoints
  - Configurable timeout with `--timeout` flag (default: 30s)
  - Usage: `warp speedtest <host>[:port]`
  - New package: `internal/speedtest/` with comprehensive unit tests
  - New server handler: `internal/server/speedtest.go`
  - Integrated into main help output and completion scripts

### Fixed

- **Large Encrypted File Transfers** - Fixed critical bug where files >10MB were transmitted as plaintext
  - Issue: Sendfile optimization was incorrectly applied to encrypted transfers, bypassing EncryptReader
  - Solution: Added `!isEncrypted` check to prevent sendfile for encrypted transfers
  - Verified working for 10.5MB, 20MB, 50MB, and 100MB+ encrypted transfers with checksums
  - All file sizes now properly encrypted end-to-end

### Technical

- Added `Cyan` color to `cmd/warp/ui/colors.go` for expanded color options
- Server automatically registers speedtest endpoints on startup
- Speedtest tests pass with 100% success rate (10 unit tests)
- Integration tested with live warp server showing real network measurements

## [1.0.5] - 2025-12-20

### Added

- **Protocol package expansion**
  - Created `internal/protocol/metadata.go` with formal protocol definitions
    - `TransferType` enum (File, Directory, Text, Stream) for typed transfers
    - `Metadata` struct with validation for transfer metadata
    - Helper methods: `IsChunked()`, `IsCompressible()`, `ShouldUseZeroCopy()`
  - Created `internal/protocol/constants.go` with centralized configuration
    - Buffer size constants: BufferSizeSmall (8KB), Medium (64KB), Large (1MB), VeryLarge (4MB)
    - Threshold constants: SendfileThreshold (10MB), MaxCacheFileSize
    - Progress timing: ProgressUpdateInterval (200ms), WebSocketUpdateInterval (100ms)
    - UI dimensions: ProgressBarWidth (20), ProgressBarFilled, ProgressBarEmpty
    - `GetOptimalBufferSize()` function for adaptive buffer sizing
  - Expanded protocol package from 14 lines to 200+ lines with comprehensive definitions
  - Provides foundation for future protocol versioning and feature additions

- **Color-coded status messages** throughout the application
  - Green for success messages (Download complete, Upload complete, Checksum verified)
  - Yellow for warnings (File already exists)
  - Red for errors (connection failures, checksum mismatches)
  - Dim/gray for secondary information (Summary headers, borders)
  - Automatically respects `NO_COLOR` environment variable for accessibility
  - Applied consistently across receiver, uploader, server, and progress displays

- **Multi-file progress tracking** for multiple files and directory transfers
  - Shows individual file progress when sending multiple files or directories
  - Displays "Compressing: X/Y files" with current file name during zip creation
  - Shows total file count and cumulative size before compression starts
  - Completion message shows final statistics (Compressed X files, total size)
  - Improves visibility into what's being transferred in large directories

### Improved

- Progress bars now display in green with colored percentage indicators
- Transfer completion summaries use consistent color scheme
  - Success indicators in green
  - Section headers in dim/gray for better visual hierarchy
- Upload complete messages match download complete styling
- All status messages maintain consistent formatting and color usage

### Refactored

- **Structured logging implementation**
  - Replaced 40+ unstructured `log.Printf` calls with structured logging using zap
  - Updated all server package files to use `logging.Info()`, `logging.Warn()`, `logging.Error()`
  - Added structured fields for better log filtering and aggregation:
    - Session tracking: `zap.String("session_id", ...)`, `zap.Int("chunk_id", ...)`
    - File operations: `zap.String("filename", ...)`, `zap.String("size", ...)`
    - Performance: `zap.Float64("duration", ...)`, `zap.Float64("mbps", ...)`
    - Errors: `zap.Error(err)` for proper error chain tracking
  - Benefits:
    - JSON-structured output ready for log aggregation (ELK, Loki, Datadog)
    - Better log filtering by specific fields (filename, session_id, error type)
    - Type-safe logging with compile-time validation
    - Zero-allocation performance with zap
    - Proper severity levels (Info/Warn/Error) consistently applied
  - Files updated: `chunks.go`, `upload.go`, `session.go`, `server.go`, `download.go`, `websocket.go`, `http_linux.go`
  - Zero "log" package imports remaining in internal packages

- **Metrics package organization**
  - Reorganized 227-line monolithic `metrics.go` into 7 focused modules
  - Created logical grouping: `upload.go`, `download.go`, `chunks.go`, `session.go`, `cache.go`, `websocket.go`, `http.go`
  - Added comprehensive documentation to each module explaining metric purpose and usage
  - Created helper functions for common operations:
    - Cache: `RecordCacheHit()`, `RecordCacheMiss()`, `SetCacheSize()`
    - Session: `RecordUploadSession()`, `RecordRetry()`, `RecordError()`
    - Chunks: `RecordChunkSuccess()`, `RecordChunkRetry()`, `RecordChunkError()`
    - WebSocket: `WebSocketConnected()`, `RecordProgressMessage()`
  - Each metric now includes labels documentation and use cases
  - Average file size: ~67 lines (vs 227 lines previously)
  - Easier to find relevant metrics for specific features
  - Better maintainability with clear separation of concerns

- **Eliminated magic numbers throughout codebase**
  - Extracted all hardcoded buffer sizes, timing intervals, and UI dimensions to named constants
  - Updated 9 files to use `protocol` package constants:
    - `internal/client/receiver.go` - Uses `protocol.GetOptimalBufferSize()`
    - `internal/client/uploader.go` - Uses `protocol.ProgressUpdateInterval`
    - `internal/server/cache.go` - Uses protocol buffer size constants
    - `internal/server/upload.go` - Uses `protocol.GetOptimalBufferSize()`
    - `internal/server/constants.go` - References protocol constants
    - `internal/server/server_test.go` - Uses protocol constants in tests
    - `internal/ui/progress.go` - Uses `protocol.ProgressBarWidth` and char constants
  - Provides single source of truth for configuration values
  - Improves code readability with semantic constant names
  - Makes performance tuning easier by centralizing configuration

- **Major HTTP server refactoring**
  - Split monolithic 1,711-line `http.go` into 11 focused, maintainable files
  - Created specialized modules: `cache.go`, `chunks.go`, `download.go`, `progress.go`, `ratelimit.go`, `sanitize.go`, `server.go`, `session.go`, `upload.go`, `websocket.go`, `embed.go`
  - Applied Single Responsibility Principle - each file handles one clear concern
  - Improved code maintainability and testability
  - All 20 tests passing, zero functionality lost
  - Average file size: ~165 lines (within target range of 150-300 lines)

- **Command-line interface modularization**
  - Reduced `main.go` from 904 lines to 62 lines (93% reduction)
  - Extracted send/receive logic into dedicated command modules
  - Improved separation of concerns and code organization

### Fixed

- **Configuration flow inconsistency**
  - Config file values now properly used as defaults for CLI flags
  - Implemented correct precedence chain: config file → environment variables → CLI flags
  - Commands now load `~/.config/warp/warp.yaml` at startup
  - Users can set persistent defaults without repeating the same flags
  - Environment variables (`WARP_*` prefix) correctly override config values
  - CLI flags remain highest priority, overriding everything

- **Error handling inconsistencies**
  - Created centralized error package with `UserError` type for user-friendly messages
  - Standardized error handling across all command files
  - Removed 30+ `log.Fatal` calls - errors now properly returned to main()
  - Added context to all error messages using `fmt.Errorf` with `%w`
  - Main.go now centrally handles error display with color-coded messages
  - User-facing errors include helpful suggestions for resolution
  - Improved error messages in client package (9 locations)
  - Improved error messages in server package (13 locations)
  - All errors now properly wrapped for better debugging
  - Consistent error patterns across receiver, uploader, server, and command handlers

- **Duplicate HTTP client configuration**
  - Consolidated identical HTTP client code from `receiver.go` and `uploader.go`
  - Created centralized `internal/client/client.go` with single `defaultHTTPClient()` function
  - Eliminated 18 lines of duplicated code
  - Single source of truth for HTTP/2 settings, connection pooling, and timeouts
  - Easier to maintain and update client configuration

- **Missing input validation**
  - Created `internal/server/validate.go` with comprehensive validation functions
  - Added validation for session IDs (length 8-64 chars, alphanumeric+hyphens only)
  - Added validation for upload offsets (non-negative, within file bounds)
  - Added validation for chunk IDs and total chunks (max 100,000 each)
  - Added validation for chunk sizes (64KB to 100MB)
  - All validation errors return HTTP 400 with clear error messages
  - Prevents integer overflow, resource exhaustion, and path traversal attacks
  - Applied in both sequential and parallel upload handlers

## [1.0.4] - 2025-12-19

### Added

- Enhanced error messages with helpful suggestions for common issues
  - Connection refused errors now suggest checking if server is running
  - HTTP 404 errors now suggest verifying the URL/token
  - File exists errors now suggest using --force flag to overwrite
- Transfer summary display after successful downloads
  - Shows file name, size (formatted), elapsed time, average speed
  - Displays save location and checksum verification status
- Stackable verbosity flags support (-v, -vv, -vvv)
  - Single -v enables INFO level logging
  - Double -vv enables DEBUG level logging
  - Triple -vvv enables DEBUG level with additional details
  - Works with both -v and --verbose flags
- File size formatting throughout the UI (B/KB/MB/GB/TB units)
  - Progress bar now shows human-readable sizes instead of bytes
  - Transfer summary displays formatted file sizes
  - Automatic unit selection based on size magnitude
- Real-time progress display for uploads (host mode)
  - Shows progress bar with percentage, size, speed, and elapsed time
  - Matches the improved receiver UI for consistency
  - Updates in real-time as chunks are received

### Improved

- QR code presentation
  - Changed from Low to Medium quality for better scanning reliability
  - Added decorative border with box-drawing characters
  - Improved instructions text with helpful tips
- Progress bar display (both send and receive modes)
  - Text-based time labels (Time: and ETA:) instead of emojis
  - More accurate speed calculation and display
  - Consistent formatting across upload and download operations
- Transfer summary after completion (both send and receive modes)
  - Shows file name, size (formatted), elapsed time, average speed
  - Text-based status messages for professional appearance
  - Formatted with horizontal borders for visual clarity
- Server startup messages
  - Clean text-based output without decorative symbols
  - Better color usage for visual hierarchy
  - Clearer formatting and spacing
- All UI elements now use professional text-only formatting
  - Removed emojis from progress bars, tips, and status messages
  - Consistent text-based approach throughout the application
- Help text for verbose flags
  - Updated to mention -vv and -vvv options
  - Added descriptions explaining verbosity levels

## [1.0.3] - 2025-12-19

### Security

- **CRITICAL**: Fixed nonce reuse vulnerability in AES-GCM encryption that could break encryption after ~1TB of data
  - Added chunk counter with 2^32 safety limit to prevent nonce exhaustion
  - Implemented deterministic nonce construction using counter in last 8 bytes
  - Both EncryptReader and DecryptReader now properly track chunk count
- **CRITICAL**: Added size limits to multipart uploads to prevent memory exhaustion DoS attacks
  - Enforced 10GB per-part maximum to protect against malicious oversized uploads
- Enhanced filename sanitization with comprehensive validation for path traversal, null bytes, and control characters
  - Rejects path separators (/, \) for directory traversal prevention
  - Rejects ".." anywhere in filename (including substrings like "0..")
  - Detects null bytes, control characters, and normalization attacks
  - Verifies names don't change after normalization (attack detection)
  - Rejects purely whitespace filenames
  - Fuzz-tested with 239,000+ random inputs to verify robustness

### Added

- HTTP client dependency injection via Downloader struct for improved testability
- InitError() function in logging package to check for initialization errors
- Buffer pool for chunk uploads to reduce memory allocations and GC pressure
- Constants file (internal/server/constants.go) for all magic numbers and timeouts
- Enhanced metrics for error tracking and retry monitoring
  - ErrorsTotal counter with type and operation labels
  - RetryAttemptsTotal counter for monitoring retries
  - SessionDuration histogram for tracking complete session lifecycle
- TCP socket buffer tuning for 5-15% throughput improvement on high-latency networks
  - Configurable send/receive buffer sizes (4MB each)
  - TCP_QUICKACK enabled on Linux for faster response times
- File checksum caching to eliminate redundant computation on repeated downloads
- Rate limiter cleanup routine to prevent memory leaks in long-running servers
- Pre-computed progress bars to eliminate string allocations during transfers

### Fixed

- **CRITICAL**: Fixed sendfile offset corruption causing data corruption in large file transfers (>1GB)
  - Corrected offset calculation to use fresh computation on each iteration
  - Prevents corrupted data transmission on slow/unstable networks
- **CRITICAL**: Fixed connection hijacking race conditions causing panics and resource leaks
  - Consolidated defer cleanup operations to prevent double-close errors
  - Added proper error logging for failed cleanup operations
- **CRITICAL**: Fixed goroutine leak in session cleanup background process
  - Added shutdown context to properly terminate background goroutines
  - Implemented graceful shutdown with context cancellation
- Fixed discovery race condition with proper channel closing and goroutine synchronization
- Fixed production-unsafe init() panic in logger - now uses lazy initialization with fallback
- Fixed config error swallowing that hid malformed configuration files
- Fixed parallel upload test compatibility with enhanced filename validation

### Changed

- **BREAKING**: HTTP client is now injectable via NewDownloader() constructor
  - Backward-compatible package-level Receive() function maintained for existing code
- Configuration parsing now returns actual errors instead of silently using defaults
  - Distinguishes between "config not found" (OK) and "config malformed" (error)
- Logger initialization changed from panic to lazy initialization with fallback
  - Uses sync.Once pattern for thread-safe initialization
  - Falls back to no-op logger on error instead of crashing
- Discovery channel handling improved to prevent double-close panics
  - Properly waits for processing goroutine completion
- Enhanced Server struct with shutdown context for proper lifecycle management
- Improved resource cleanup ordering (files closed before connections)
- Added explicit error messages for encryption/decryption limit exceeded
- Uploader now sends base filename instead of full path in X-File-Name header
- Replaced all magic numbers with named constants for better maintainability
  - Session cleanup intervals, timeouts, buffer sizes, and limits
  - WebSocket configuration (read/write buffers, update interval)
  - TCP tuning parameters (keepalive period, buffer sizes)
- Chunk upload now uses sync.Pool for buffer reuse (reduces allocations by ~95%)
- Enhanced TCP listener with platform-specific socket optimizations

### Performance

- Reduced memory allocations in chunk uploads by 95% using buffer pooling
- Improved network throughput by 5-15% with optimized TCP socket buffers
- Enhanced TCP performance with SO_SNDBUF/SO_RCVBUF tuning (4MB each)
- Eliminated redundant buffer allocations in parallel upload workers
- File checksum caching reduces repeated SHA256 computation (eliminates double file read)
  - Cached checksums validated using file size and modification time
  - Significant speedup for repeated downloads of the same file
- Progress bar optimization eliminates ~1000 string allocations per second during transfers
  - Pre-computed 21 possible progress bar states at init
  - Zero allocation lookups during active transfers
- Rate limiter memory leak prevention with automatic cleanup of stale entries
  - Removes limiters inactive for >1 hour every 30 minutes
  - Prevents unbounded memory growth in long-running servers

### Testing

- Fixed race condition in parallel upload test with proper mutex protection
- All tests pass with race detector enabled (`go test -race`)
- Verified fixes with comprehensive end-to-end test suite
- Enhanced security validation in filename sanitization tests
- Added comprehensive test suite for critical functionality:
  - Nonce exhaustion protection tests for encryption (internal/crypto/encrypt_nonce_test.go)
  - Goroutine leak prevention tests (internal/server/leak_test.go)
  - Rate limiter cleanup verification tests
  - Checksum cache validation tests
  - Fuzz testing for filename sanitization with 239K+ iterations (internal/server/fuzz_test.go)
  - Known-good and known-bad filename validation tests
  - Normalization attack detection tests
- All new tests pass with race detector and fuzzer

## [1.0.2] - 2025-12-18

### Fixed

- Fixed all errcheck linting issues across the codebase for improved error handling
- Fixed unused parameter warnings in platform-specific implementations
- Fixed ineffectual assignments and nil checks
- Removed obsolete build tags

### Added

- Integrated chunk tracking for performance monitoring via `chunkTimes` field
- Added rate limiting support for downloads using `RateLimitedWriter`
- Added `getRateLimiter()` and `getClientIP()` methods for bandwidth control
- Enhanced metrics integration for chunk uploads and rate limiting

### Changed

- Improved error handling throughout client and server code
- Enhanced test suite with proper error checking in all test cases
- Rewrite README.md

## [1.0.1] - 2025-12-18

### Fixed

- Fixed GitHub Actions release workflow artifact upload paths causing build binaries to fail

## [1.0.0] - 2025-12-18

### Added

#### Core Features

- HTTP server with zero-copy sendfile optimization on Linux
- Parallel chunk upload support with configurable workers
- Session-based upload management with unique session IDs
- WebSocket real-time progress tracking
- SHA256 checksum verification for file integrity
- mDNS/DNS-SD discovery for local network file sharing
- QR code generation for easy URL sharing
- Platform-specific optimizations (Linux syscalls, cross-platform fallbacks)
- **Host mode** for receiving uploads from other devices
- **Text sharing** via `--text` flag
- **Stdin streaming** via `--stdin` flag
- **Directory auto-ZIP** - directories are streamed as ZIP archives
- **Web upload interface** with drag-and-drop support
- **Automatic gzip compression** for compressible file types
- **File caching** with configurable cache size (default 100MB)
- **Bandwidth rate limiting** per-client for uploads and downloads

#### Encryption & Security

- AES-256-GCM encryption infrastructure
- PBKDF2 key derivation (100,000 iterations)
- Password-based encryption with salt and random nonces
- Streaming encryption/decryption for large files
- Secure token generation for download URLs
- Optional encryption via `--encrypt` flag

#### Configuration Management

- YAML configuration file support (`~/.config/warp/warp.yaml`)
- Environment variable overrides (WARP\_\* prefix)
- Configuration commands: `warp config init`, `warp config set/get`
- Default values with user customization
- Per-transfer configuration via command-line flags

#### Monitoring & Observability

- Prometheus metrics endpoint at `/metrics`
- 15+ metrics including:
  - Upload/download duration histograms
  - Transfer size tracking
  - Active transfer gauges
  - Chunk upload counters
  - Cache hit rates
  - Checksum verification counters
  - WebSocket connection metrics
- Structured JSON logging with zap
- Configurable log levels (debug, info, warn, error)

#### User Interface

- Interactive progress bars with ETA calculation
- Real-time speed indicators (MB/s, Mbps)
- Color-coded status messages
- Terminal QR code display
- **HTML upload interface** with drag-and-drop
- **Terminal-style retro web UI** for uploads
- **Multiple file upload** support in web interface
- Responsive web UI for browsers and mobile devices

#### Shell Integration

- Shell completion for bash, zsh, fish, and PowerShell
- Generate completions: `warp completion <shell>`
- Command and flag autocomplete support

#### Testing & Quality

- Comprehensive unit tests across all packages
- End-to-end integration tests
- Test coverage reporting
- Platform-specific test suites
- Table-driven test patterns

#### Platform Support

- Linux with optimized syscalls (sendfile, disk space checks)
- Windows with fallback implementations
- macOS with fallback implementations
- Cross-platform build support

#### Content Sharing

- **File sharing** - single files of any size
- **Directory sharing** - automatically zipped with deflate compression
- **Text snippets** - share via `--text` flag, displayed in terminal
- **Stdin streaming** - pipe command output via `--stdin`
- **Multiple upload mode** - host mode accepts multiple files from web UI
- Text content displays in terminal (not saved to file)
- Binary files saved with automatic extension detection

### Changed

#### Performance Improvements

- Implemented zero-copy sendfile for Linux downloads
- Parallel chunk uploads with configurable workers (default: 3)
- Configurable chunk size (default: 2MB)
- Session-based upload prevents file locking issues
- Optimized memory usage for large file transfers
- **Automatic gzip compression** for text-based files
- **File caching system** with LRU-like eviction
- **Adaptive buffer pools** (8KB to 4MB) based on file size
- **Rate limiting per-client** for fair bandwidth distribution

#### Protocol Enhancements

- X-Upload-Session header for chunk coordination
- Improved handshake protocol with version negotiation
- Better error messages and status codes
- Graceful handling of connection interruptions

#### User Experience

- Improved progress tracking with combined upload speed
- Health check polling skips during active uploads
- Better terminal output formatting
- More informative error messages
- Consistent command-line interface

### Fixed

- Windows build compatibility issues with Linux syscalls
- Chunk offset mismatch errors during parallel uploads
- File-in-use errors on Windows during upload completion
- Race conditions in file access patterns
- WebSocket disconnection during long uploads
- Speed calculation showing per-chunk instead of overall speed
- Health check timeouts during large transfers

### Security

- Constant-time token comparison to prevent timing attacks
- Secure random token generation
- PBKDF2 key derivation with sufficient iterations
- Proper cleanup of temporary files
- Input validation for all user-provided data

### Documentation

- Comprehensive README with usage examples
- Contributing guidelines
- Code of conduct
- Inline code documentation
- Help text for all commands and flags

### Technical Details

#### Dependencies

- Go 1.21+
- github.com/gorilla/websocket v1.5.3 - WebSocket support
- github.com/grandcat/zeroconf v1.0.0 - mDNS/DNS-SD
- github.com/prometheus/client_golang v1.23.2 - Metrics
- github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e - QR codes
- github.com/spf13/viper v1.21.0 - Configuration
- go.uber.org/zap v1.27.1 - Structured logging
- golang.org/x/crypto v0.46.0 - Cryptography
- golang.org/x/time v0.14.0 - Rate limiting

#### Build Targets

- linux/amd64
- linux/arm64
- darwin/amd64
- darwin/arm64
- windows/amd64

### Removed

- Clipboard QR scanning feature (not practical for terminal-based QR codes)
- Associated dependencies: gozxing, clipboard

---

## Release Notes

### v1.0.0 - Initial Release

This is the first stable release of warp, a fast and secure file transfer tool for local networks. The project provides a complete solution for sharing files between devices with features including encryption, progress tracking, monitoring, and easy discovery.

#### Highlights

- **Fast**: Zero-copy transfers on Linux, parallel chunk uploads
- **Secure**: AES-256-GCM encryption, SHA256 checksums
- **Observable**: Prometheus metrics, structured logging
- **User-friendly**: Progress bars, QR codes, shell completion
- **Configurable**: YAML config, environment variables, CLI flags
- **Production-ready**: Comprehensive tests, cross-platform support

#### Migration Notes

This is the initial release, no migration required.

#### Known Limitations

- mDNS discovery requires multicast support (may not work in some network configurations)
- Zero-copy sendfile only available on Linux
- Disk space checks only available on Linux

#### Next Steps

Future releases may include:

- Docker image distribution
- Additional encryption algorithms
- Enhanced discovery protocols
- Multi-file transfer support

---

For more information, see the [README](README.md) and [CONTRIBUTING](CONTRIBUTING.md) guides.

[1.0.0]: https://github.com/zulfikawr/warp/releases/tag/v1.0.0

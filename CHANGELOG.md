# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
- Environment variable overrides (WARP_* prefix)
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
- CI/CD pipeline integration
- Binary releases for all platforms
- Docker image distribution
- Additional encryption algorithms
- Enhanced discovery protocols
- Rate limiting and bandwidth control
- Multi-file transfer support

---

For more information, see the [README](README.md) and [CONTRIBUTING](CONTRIBUTING.md) guides.

[1.0.0]: https://github.com/zulfikawr/warp/releases/tag/v1.0.0

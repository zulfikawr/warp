package cli

import (
	"fmt"
	"runtime"

	"github.com/zulfikawr/warp/internal/ui"
)

var (
	// These can be set via ldflags at build time
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Version prints version information
func Version() {
	fmt.Printf("%sWarp%s %s\n", ui.Colors.Bold+ui.Colors.Cyan, ui.Colors.Reset, version)
	fmt.Printf("Commit:  %s\n", commit)
	fmt.Printf("Built:   %s\n", date)
	fmt.Printf("Go:      %s\n", runtime.Version())
	fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

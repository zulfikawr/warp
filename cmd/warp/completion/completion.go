package completion

import (
	"fmt"

	"github.com/zulfikawr/warp/cmd/warp/ui"
)

// Generate executes the completion command
func Generate(args []string) error {
	if len(args) == 0 {
		Help()
		return nil
	}

	shell := args[0]
	switch shell {
	case "bash":
		Bash()
	case "zsh":
		Zsh()
	case "fish":
		Fish()
	case "powershell":
		Powershell()
	case "-h", "--help", "help":
		Help()
	default:
		fmt.Printf("Unknown shell: %s\n", shell)
		Help()
		return fmt.Errorf("unknown shell: %s", shell)
	}

	return nil
}

// Help displays completion command help
func Help() {
	fmt.Println(ui.C.Bold + ui.C.Green + "warp completion" + ui.C.Reset + " - Generate shell completion scripts")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Usage:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Green + "warp completion" + ui.C.Reset + " [bash|zsh|fish|powershell]")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Available Shells:" + ui.C.Reset)
	fmt.Println("  " + ui.C.Yellow + "bash" + ui.C.Reset + "              Bash completion script")
	fmt.Println("  " + ui.C.Yellow + "zsh" + ui.C.Reset + "               Zsh completion script")
	fmt.Println("  " + ui.C.Yellow + "fish" + ui.C.Reset + "              Fish completion script")
	fmt.Println("  " + ui.C.Yellow + "powershell" + ui.C.Reset + "        PowerShell completion script")
	fmt.Println()
	fmt.Println(ui.C.Bold + "Installation:" + ui.C.Reset)
	fmt.Println()
	fmt.Println(ui.C.Bold + "  Bash:" + ui.C.Reset)
	fmt.Println("    $ warp completion bash > /etc/bash_completion.d/warp")
	fmt.Println("    $ source /etc/bash_completion.d/warp")
	fmt.Println()
	fmt.Println(ui.C.Bold + "  Zsh:" + ui.C.Reset)
	fmt.Println("    $ warp completion zsh > /usr/local/share/zsh/site-functions/_warp")
	fmt.Println("    $ autoload -U compinit && compinit")
	fmt.Println()
	fmt.Println(ui.C.Bold + "  Fish:" + ui.C.Reset)
	fmt.Println("    $ warp completion fish > ~/.config/fish/completions/warp.fish")
	fmt.Println()
	fmt.Println(ui.C.Bold + "  PowerShell:" + ui.C.Reset)
	fmt.Println("    $ warp completion powershell | Out-String | Invoke-Expression")
	fmt.Println()
}

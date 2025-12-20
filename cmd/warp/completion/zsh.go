package completion

import "fmt"

// Zsh generates zsh completion script
func Zsh() {
	script := `#compdef warp

_warp() {
    local curcontext="$curcontext" state line
    typeset -A opt_args

    _arguments -C \
        '1: :->command' \
        '*:: :->args'

    case $state in
        command)
            local commands=(
                'send:Share a file, directory, or text snippet'
                'host:Receive uploads into a directory'
                'receive:Download from a warp URL'
                'search:Discover nearby warp hosts'
                'config:Manage configuration file'
                'completion:Generate shell completion scripts'
            )
            _describe 'command' commands
            ;;
        args)
            case $line[1] in
                send)
                    _arguments \
                        {-p,--port}'[Port number]' \
                        {-i,--interface}'[Network interface]' \
                        '--text[Send text snippet]' \
                        '--stdin[Read from stdin]' \
                        '--rate-limit[Bandwidth limit in Mbps]' \
                        '--cache-size[Cache size in MB]' \
                        '--no-qr[Skip QR code]' \
                        {-h,--help}'[Show help]' \
                        '*:file:_files'
                    ;;
                host)
                    _arguments \
                        {-i,--interface}'[Network interface]' \
                        {-d,--dest}'[Destination directory]' \
                        '--rate-limit[Bandwidth limit in Mbps]' \
                        '--no-qr[Skip QR code]' \
                        {-h,--help}'[Show help]'
                    ;;
                receive)
                    _arguments \
                        {-o,--output}'[Output file]' \
                        {-f,--force}'[Force overwrite]' \
                        '--workers[Parallel workers]' \
                        '--chunk-size[Chunk size in MB]' \
                        '--no-checksum[Skip checksum]' \
                        {-h,--help}'[Show help]'
                    ;;
                search)
                    _arguments \
                        '--timeout[Discovery timeout]' \
                        {-h,--help}'[Show help]'
                    ;;
                config)
                    local config_commands=(
                        'show:Display current configuration'
                        'edit:Open config file in editor'
                        'path:Show config file path'
                    )
                    _describe 'config command' config_commands
                    ;;
                completion)
                    local shells=(
                        'bash:Bash completion'
                        'zsh:Zsh completion'
                        'fish:Fish completion'
                        'powershell:PowerShell completion'
                    )
                    _describe 'shell' shells
                    ;;
            esac
            ;;
    esac
}

_warp "$@"
`
	fmt.Print(script)
}

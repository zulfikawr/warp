package completion

import "fmt"

// Fish generates fish completion script
func Fish() {
	script := `# fish completion for warp

# Main commands
complete -c warp -f -n '__fish_use_subcommand' -a send -d 'Share a file, directory, or text snippet'
complete -c warp -f -n '__fish_use_subcommand' -a host -d 'Receive uploads into a directory'
complete -c warp -f -n '__fish_use_subcommand' -a receive -d 'Download from a warp URL'
complete -c warp -f -n '__fish_use_subcommand' -a search -d 'Discover nearby warp hosts'
complete -c warp -f -n '__fish_use_subcommand' -a config -d 'Manage configuration file'
complete -c warp -f -n '__fish_use_subcommand' -a completion -d 'Generate shell completion scripts'

# send command
complete -c warp -f -n '__fish_seen_subcommand_from send' -s p -l port -d 'Port number'
complete -c warp -f -n '__fish_seen_subcommand_from send' -s i -l interface -d 'Network interface'
complete -c warp -f -n '__fish_seen_subcommand_from send' -l text -d 'Send text snippet'
complete -c warp -f -n '__fish_seen_subcommand_from send' -l stdin -d 'Read from stdin'
complete -c warp -f -n '__fish_seen_subcommand_from send' -l rate-limit -d 'Bandwidth limit in Mbps'
complete -c warp -f -n '__fish_seen_subcommand_from send' -l cache-size -d 'Cache size in MB'
complete -c warp -f -n '__fish_seen_subcommand_from send' -l no-qr -d 'Skip QR code'
complete -c warp -f -n '__fish_seen_subcommand_from send' -s h -l help -d 'Show help'

# host command
complete -c warp -f -n '__fish_seen_subcommand_from host' -s i -l interface -d 'Network interface'
complete -c warp -f -n '__fish_seen_subcommand_from host' -s d -l dest -d 'Destination directory'
complete -c warp -f -n '__fish_seen_subcommand_from host' -l rate-limit -d 'Bandwidth limit in Mbps'
complete -c warp -f -n '__fish_seen_subcommand_from host' -l no-qr -d 'Skip QR code'
complete -c warp -f -n '__fish_seen_subcommand_from host' -s h -l help -d 'Show help'

# receive command
complete -c warp -f -n '__fish_seen_subcommand_from receive' -s o -l output -d 'Output file'
complete -c warp -f -n '__fish_seen_subcommand_from receive' -s f -l force -d 'Force overwrite'
complete -c warp -f -n '__fish_seen_subcommand_from receive' -l workers -d 'Parallel workers'
complete -c warp -f -n '__fish_seen_subcommand_from receive' -l chunk-size -d 'Chunk size in MB'
complete -c warp -f -n '__fish_seen_subcommand_from receive' -l no-checksum -d 'Skip checksum'
complete -c warp -f -n '__fish_seen_subcommand_from receive' -s h -l help -d 'Show help'

# search command
complete -c warp -f -n '__fish_seen_subcommand_from search' -l timeout -d 'Discovery timeout'
complete -c warp -f -n '__fish_seen_subcommand_from search' -s h -l help -d 'Show help'

# config command
complete -c warp -f -n '__fish_seen_subcommand_from config' -a 'show' -d 'Display current configuration'
complete -c warp -f -n '__fish_seen_subcommand_from config' -a 'edit' -d 'Open config file in editor'
complete -c warp -f -n '__fish_seen_subcommand_from config' -a 'path' -d 'Show config file path'

# completion command
complete -c warp -f -n '__fish_seen_subcommand_from completion' -a 'bash' -d 'Bash completion'
complete -c warp -f -n '__fish_seen_subcommand_from completion' -a 'zsh' -d 'Zsh completion'
complete -c warp -f -n '__fish_seen_subcommand_from completion' -a 'fish' -d 'Fish completion'
complete -c warp -f -n '__fish_seen_subcommand_from completion' -a 'powershell' -d 'PowerShell completion'
`
	fmt.Print(script)
}

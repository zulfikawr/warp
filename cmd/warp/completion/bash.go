package completion

import "fmt"

// Bash generates bash completion script
func Bash() {
	script := `# bash completion for warp
_warp_completion() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    # Main commands
    if [ $COMP_CWORD -eq 1 ]; then
        opts="send host receive search config completion"
        COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
        return 0
    fi
    
    # Subcommand completion
    case "${COMP_WORDS[1]}" in
        send)
            opts="-p --port -i --interface --text --stdin --rate-limit --cache-size --no-qr -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            # File completion for paths
            if [[ ! ${cur} == -* ]]; then
                COMPREPLY=( $(compgen -f -- ${cur}) )
            fi
            ;;
        host)
            opts="-i --interface -d --dest --rate-limit --no-qr -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            ;;
        receive)
            opts="-o --output -f --force --workers --chunk-size --no-checksum -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            ;;
        search)
            opts="--timeout -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            ;;
        config)
            if [ $COMP_CWORD -eq 2 ]; then
                opts="show edit path"
                COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            fi
            ;;
        completion)
            if [ $COMP_CWORD -eq 2 ]; then
                opts="bash zsh fish powershell"
                COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
            fi
            ;;
    esac
}

complete -F _warp_completion warp
`
	fmt.Print(script)
}

package completion

import "fmt"

// Powershell generates PowerShell completion script
func Powershell() {
	script := `# PowerShell completion for warp

Register-ArgumentCompleter -Native -CommandName warp -ScriptBlock {
    param($commandName, $wordToComplete, $commandAst, $fakeBoundParameters)

    $commands = @(
        [System.Management.Automation.CompletionResult]::new('send', 'send', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Share a file')
        [System.Management.Automation.CompletionResult]::new('host', 'host', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Receive uploads')
        [System.Management.Automation.CompletionResult]::new('receive', 'receive', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Download from URL')
        [System.Management.Automation.CompletionResult]::new('search', 'search', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Discover hosts')
        [System.Management.Automation.CompletionResult]::new('config', 'config', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Manage config')
        [System.Management.Automation.CompletionResult]::new('completion', 'completion', [System.Management.Automation.CompletionResultType]::ParameterValue, 'Generate completion')
    )

    $commands | Where-Object { $_.CompletionText -like "$wordToComplete*" }
}
`
	fmt.Print(script)
}

Register-ArgumentCompleter -Native -CommandName git-auto-sync -ScriptBlock {
     param($commandName, $wordToComplete, $cursorPosition)
     $other = "$commandName $wordToComplete --generate-bash-completion"
         Invoke-Expression $other | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
         }
 }

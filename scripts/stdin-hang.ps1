$ErrorActionPreference = 'Continue'

# Reproduce the reported hang: run the CLI while stdin is a pipe that never
# reaches EOF. A working build must print and exit promptly.
$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = 'cmd.exe'
$psi.Arguments = '/c "' + $PWD.Path.Replace('\','\\') + '\\decide.exe" ' + $args[0]
$psi.UseShellExecute = $false
$psi.RedirectStandardInput = $true
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true

$proc = [System.Diagnostics.Process]::Start($psi)
# Deliberately never close stdin: this is the condition that used to hang.
$done = $proc.WaitForExit(8000)

if (-not $done) {
    $proc.Kill()
    Write-Output "FAIL  $($args[0])  still running after 8s with an open stdin pipe"
    exit 1
}

$out = $proc.StandardOutput.ReadToEnd()
$err = $proc.StandardError.ReadToEnd()
Write-Output "exit=$($proc.ExitCode)"
Write-Output "stdout=$($out.Trim())"
if ($err.Trim()) { Write-Output "stderr=$($err.Trim())" }
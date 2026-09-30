package upgrade

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// PlanSystem returns OS vendor update commands for genv targets.
//
//	macos:    sudo softwareupdate -i -a
//	windows:  Windows Update Agent COM via pwsh (else powershell / powershell.exe)
//	arch / wsl-arch: sudo pacman -Syu --noconfirm
//	ubuntu:   sudo apt-get update && sudo apt-get upgrade -y (apt if apt-get is absent)
//
// paru/yay remain tracked-package adapters; they are not the OS vendor updater.
// snap stays tracked-only — genv never refreshes untracked snaps here.
// sudo/elevation is part of the planned argv; missing sudo is an apply failure,
// not a silent skip. On Windows, genv does not auto-elevate: the WUA COM script
// often needs an elevated PowerShell session (see docs/windows-install.md).
func PlanSystem(env Env) CommandPlan {
	switch env.Target {
	case "macos":
		if !env.has("softwareupdate") {
			return skipPlan("softwareupdate not found")
		}
		return cmdPlan([]string{"sudo", "softwareupdate", "-i", "-a"})
	case "windows":
		bin := env.look("pwsh")
		if bin == "" {
			bin = env.look("powershell")
		}
		if bin == "" {
			bin = env.look("powershell.exe")
		}
		if bin == "" {
			return skipPlan("PowerShell not found (Windows Update Agent COM API needs pwsh or powershell)")
		}
		return cmdPlan([]string{bin, "-NoProfile", "-Command", windowsUpdateInstallScript})
	case "arch", "wsl-arch":
		if !env.has("pacman") {
			return skipPlan("pacman not found")
		}
		return cmdPlan([]string{"sudo", "pacman", "-Syu", "--noconfirm"})
	case "ubuntu":
		apt := ""
		if env.has("apt-get") {
			apt = "apt-get"
		} else if env.has("apt") {
			apt = "apt"
		}
		if apt == "" {
			return skipPlan("apt-get not found")
		}
		return cmdPlan(
			[]string{"sudo", apt, "update"},
			[]string{"sudo", apt, "upgrade", "-y"},
		)
	case "linux":
		return skipPlan("no OS vendor updater for target linux (use arch or ubuntu)")
	case "":
		return skipPlan("no genv target for OS vendor updates")
	default:
		return skipPlan("no OS vendor updater for target " + env.Target)
	}
}

// windowsUpdateResultNames maps WUA OperationResultCode values to text.
var windowsUpdateResultNames = map[int]string{
	0: "NotStarted",
	1: "InProgress",
	2: "Succeeded",
	3: "SucceededWithErrors",
	4: "Failed",
	5: "Aborted",
}

// windowsUpdateResultExitBase is the base for exit codes the embedded script
// chooses to report a WUA OperationResultCode with. $ErrorActionPreference =
// 'Stop' makes PowerShell exit 1 on any uncaught COM error (access denied,
// WUA service disabled), which used to be reported as "WUA ResultCode 1
// (InProgress)" with an elevation hint. Offsetting the script's own codes by
// this base lets Go tell "the script decided the result was 1" apart from
// "PowerShell fell over", so only the former is translated as a ResultCode.
const windowsUpdateResultExitBase = 100

// windowsUpdateElevationHint is appended to Windows system-step failures.
const windowsUpdateElevationHint = "try an elevated PowerShell session or open Settings → Windows Update"

// wrapWindowsUpdateError rewrites opaque WUA exit codes into readable errors.
// The script reports a chosen ResultCode as windowsUpdateResultExitBase+code;
// any other non-zero exit is an unexpected PowerShell failure and is left
// alone, because translating it would invent a WUA status that was never
// reported. SucceededWithErrors (3) is soft success.
func wrapWindowsUpdateError(err error) error {
	if err == nil {
		return nil
	}
	code, ok := exitCodeFromError(err)
	if !ok {
		return err
	}
	if code < windowsUpdateResultExitBase {
		// Not a ResultCode the script chose: a PowerShell exception (exit 1),
		// an explicit failure, or anything else. Report it as a script failure
		// so it is not mistaken for an in-progress or aborted update.
		return fmt.Errorf("windows update: PowerShell exited %d (%v); %s", code, err, windowsUpdateElevationHint)
	}
	code -= windowsUpdateResultExitBase
	name, known := windowsUpdateResultNames[code]
	if !known {
		return fmt.Errorf("windows update: PowerShell exited %d (%v); %s", code+windowsUpdateResultExitBase, err, windowsUpdateElevationHint)
	}
	if code == 3 {
		// Soft success / warning path; script should already have warned.
		return nil
	}
	if code == 2 {
		return nil
	}
	return fmt.Errorf("windows update: WUA ResultCode %d (%s); %s", code, name, windowsUpdateElevationHint)
}

func exitCodeFromError(err error) (int, bool) {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), true
	}
	// Fake execs sometimes return errors.New("exit status N") without ExitError.
	msg := err.Error()
	const prefix = "exit status "
	if !strings.HasPrefix(msg, prefix) {
		return 0, false
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(msg[len(prefix):]))
	if convErr != nil {
		return 0, false
	}
	return n, true
}

// windowsUpdateInstallScript installs pending Software updates through the
// built-in Windows Update Agent COM API. It needs no extra modules (not
// PSWindowsUpdate, not winget — winget upgrades packages, not the OS).
// genv does not auto-elevate; callers often need an elevated session.
//
// Every deliberate exit that reports a WUA OperationResultCode is offset by
// 100 so Go can distinguish it from an uncaught PowerShell exception, which
// (with $ErrorActionPreference = 'Stop') exits 1.
const windowsUpdateInstallScript = `$ErrorActionPreference = 'Stop'
$ResultExitBase = 100
function Get-WuaResultName([int]$Code) {
  switch ($Code) {
    0 { 'NotStarted' }
    1 { 'InProgress' }
    2 { 'Succeeded' }
    3 { 'SucceededWithErrors' }
    4 { 'Failed' }
    5 { 'Aborted' }
    default { "Unknown($Code)" }
  }
}
$session = New-Object -ComObject Microsoft.Update.Session
$searcher = $session.CreateUpdateSearcher()
$result = $searcher.Search('IsInstalled=0 and Type=''Software'' and IsHidden=0')
if ($result.Updates.Count -eq 0) { Write-Output 'No pending Windows software updates.'; exit 0 }
$collection = New-Object -ComObject Microsoft.Update.UpdateColl
foreach ($update in $result.Updates) {
  if (-not $update.EulaAccepted) { $update.AcceptEula() }
  [void]$collection.Add($update)
}
$downloader = $session.CreateUpdateDownloader()
$downloader.Updates = $collection
$download = $downloader.Download()
$downloadCode = [int]$download.ResultCode
if ($downloadCode -ne 2 -and $downloadCode -ne 3) {
  $downloadName = Get-WuaResultName $downloadCode
  Write-Output ("Windows Update download ResultCode={0} ({1}). Try an elevated PowerShell session or Settings → Windows Update." -f $downloadCode, $downloadName)
  exit ($ResultExitBase + $downloadCode)
}
if ($downloadCode -eq 3) {
  Write-Output 'Windows Update download completed with SucceededWithErrors (ResultCode=3); continuing to install.'
}
$installer = $session.CreateUpdateInstaller()
$installer.Updates = $collection
$install = $installer.Install()
if ($install.RebootRequired) { Write-Output 'A reboot is required to finish Windows Update.' }
$code = [int]$install.ResultCode
$name = Get-WuaResultName $code
$failed = New-Object System.Collections.Generic.List[string]
for ($i = 0; $i -lt $collection.Count; $i++) {
  $ur = $install.GetUpdateResult($i)
  if ([int]$ur.ResultCode -eq 4) {
    $title = $collection.Item($i).Title
    $hr = [uint32]$ur.HResult
    [void]$failed.Add(('{0} (HRESULT=0x{1:X8})' -f $title, $hr))
  }
}
if ($failed.Count -gt 0) {
  Write-Output ('Failed updates: ' + ($failed -join '; '))
}
if ($code -eq 2) { exit 0 }
if ($code -eq 3 -and $failed.Count -eq 0) {
  Write-Output 'Windows Update completed with SucceededWithErrors (ResultCode=3); no per-update Failed results.'
  exit 0
}
Write-Output ("Windows Update install ResultCode={0} ({1}). Try an elevated PowerShell session or Settings → Windows Update." -f $code, $name)
if ($failed.Count -gt 0) { exit ($ResultExitBase + 4) }
exit ($ResultExitBase + $code)
`

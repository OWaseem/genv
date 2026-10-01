package upgrade

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestPlanSystem_selects_vendor_commands_without_executing(t *testing.T) {
	present := func(names ...string) func(string) (string, error) {
		set := map[string]string{}
		for _, n := range names {
			set[n] = "/bin/" + n
		}
		return func(file string) (string, error) {
			if p, ok := set[file]; ok {
				return p, nil
			}
			return "", exec.ErrNotFound
		}
	}

	t.Run("macos softwareupdate", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "macos", GOOS: "darwin", LookPath: present("softwareupdate")})
		want := [][]string{{"sudo", "softwareupdate", "-i", "-a"}}
		if !commandPlansEqual(plan.Commands, want) {
			t.Fatalf("commands = %v, want %v", plan.Commands, want)
		}
		if plan.SkipReason != "" {
			t.Fatalf("SkipReason = %q, want empty", plan.SkipReason)
		}
	})

	t.Run("macos missing softwareupdate", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "macos", GOOS: "darwin", LookPath: present()})
		if len(plan.Commands) != 0 {
			t.Fatalf("commands = %v, want none", plan.Commands)
		}
		if !strings.Contains(plan.SkipReason, "softwareupdate") {
			t.Fatalf("SkipReason = %q, want softwareupdate absence", plan.SkipReason)
		}
	})

	t.Run("windows uses Windows Update Agent COM via pwsh", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "windows", GOOS: "windows", LookPath: present("pwsh")})
		if len(plan.Commands) != 1 {
			t.Fatalf("commands = %v, want one pwsh invocation", plan.Commands)
		}
		cmd := plan.Commands[0]
		if cmd[0] != "/bin/pwsh" || cmd[1] != "-NoProfile" || cmd[2] != "-Command" {
			t.Fatalf("argv prefix = %v, want pwsh -NoProfile -Command", cmd)
		}
		if !strings.Contains(cmd[3], "Microsoft.Update.Session") {
			t.Fatalf("command script %q, want Windows Update Agent COM", cmd[3])
		}
	})

	t.Run("windows falls back to powershell.exe", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "windows", GOOS: "windows", LookPath: present("powershell.exe")})
		if len(plan.Commands) != 1 || plan.Commands[0][0] != "/bin/powershell.exe" {
			t.Fatalf("commands = %v, want powershell.exe", plan.Commands)
		}
	})

	t.Run("windows missing PowerShell", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "windows", GOOS: "windows", LookPath: present()})
		if len(plan.Commands) != 0 || !strings.Contains(plan.SkipReason, "PowerShell") {
			t.Fatalf("plan = %#v, want PowerShell skip", plan)
		}
	})

	t.Run("arch pacman -Syu", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "arch", GOOS: "linux", LookPath: present("pacman")})
		want := [][]string{{"sudo", "pacman", "-Syu", "--noconfirm"}}
		if !commandPlansEqual(plan.Commands, want) {
			t.Fatalf("commands = %v, want %v", plan.Commands, want)
		}
	})

	t.Run("wsl-arch uses pacman not paru", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "wsl-arch", GOOS: "linux", LookPath: present("pacman", "paru", "yay")})
		if len(plan.Commands) != 1 || plan.Commands[0][1] != "pacman" {
			t.Fatalf("commands = %v, want sudo pacman -Syu", plan.Commands)
		}
	})

	t.Run("ubuntu apt-get", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "ubuntu", GOOS: "linux", LookPath: present("apt-get", "snap")})
		want := [][]string{
			{"sudo", "apt-get", "update"},
			{"sudo", "apt-get", "upgrade", "-y"},
		}
		if !commandPlansEqual(plan.Commands, want) {
			t.Fatalf("commands = %v, want %v (snap stays tracked-only)", plan.Commands, want)
		}
	})

	t.Run("ubuntu falls back to apt", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "ubuntu", GOOS: "linux", LookPath: present("apt")})
		want := [][]string{
			{"sudo", "apt", "update"},
			{"sudo", "apt", "upgrade", "-y"},
		}
		if !commandPlansEqual(plan.Commands, want) {
			t.Fatalf("commands = %v, want %v", plan.Commands, want)
		}
	})

	t.Run("linux target has no vendor updater", func(t *testing.T) {
		plan := PlanSystem(Env{Target: "linux", GOOS: "linux", LookPath: present("apt-get", "pacman")})
		if len(plan.Commands) != 0 || !strings.Contains(plan.SkipReason, "linux") {
			t.Fatalf("plan = %#v, want skip for generic linux target", plan)
		}
	})
}

func commandPlansEqual(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			if got[i][j] != want[i][j] {
				return false
			}
		}
	}
	return true
}

func TestWindowsUpdateInstallScript_hardens_wua_flow(t *testing.T) {
	script := windowsUpdateInstallScript
	wantSubstrings := []string{
		"AcceptEula",
		"Download()",
		"GetUpdateResult",
		"SucceededWithErrors",
		"Failed updates:",
		"elevated PowerShell",
		"Settings → Windows Update",
		"A reboot is required to finish Windows Update.",
		"Get-WuaResultName",
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(script, s) {
			t.Fatalf("windowsUpdateInstallScript missing %q", s)
		}
	}
	// Download failures must exit with the WUA code after messaging (not a silent void Download).
	if !strings.Contains(script, "downloadCode") || !strings.Contains(script, "exit ($ResultExitBase + $downloadCode)") {
		t.Fatal("script should check download ResultCode and exit with it on hard failure")
	}
	// Deliberate ResultCode exits are offset by 100 so an uncaught
	// PowerShell exception (exit 1) cannot be read as "InProgress".
	if !strings.Contains(script, "$ResultExitBase = 100") {
		t.Fatal("script should offset deliberate ResultCode exits by 100")
	}
	if strings.Contains(script, "exit $downloadCode") || strings.Contains(script, "exit $code") {
		t.Fatal("script must not exit with a bare WUA ResultCode")
	}
	// Soft success: SucceededWithErrors with no per-update Failed exits 0.
	if !strings.Contains(script, "exit 0") || !strings.Contains(script, "$failed.Count -eq 0") {
		t.Fatal("script should soft-succeed SucceededWithErrors when no per-update Failed")
	}
}

func TestWrapWindowsUpdateError_maps_result_codes(t *testing.T) {
	// Only exits the script chooses (100 + WUA ResultCode) are translated.
	t.Run("chosen ResultCode 4 becomes human Failed message", func(t *testing.T) {
		err := wrapWindowsUpdateError(errors.New("exit status 104"))
		if err == nil {
			t.Fatal("want error for ResultCode 4")
		}
		msg := err.Error()
		if strings.TrimSpace(msg) == "exit status 104" {
			t.Fatalf("opaque message preserved: %q", msg)
		}
		for _, want := range []string{"ResultCode 4", "Failed", "elevated PowerShell", "Windows Update"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("error %q missing %q", msg, want)
			}
		}
	})

	t.Run("ExitError 104 maps like exit status 104", func(t *testing.T) {
		cmd := exec.Command("sh", "-c", "exit 104")
		runErr := cmd.Run()
		if runErr == nil {
			t.Fatal("expected exit 104 from sh")
		}
		err := wrapWindowsUpdateError(runErr)
		if err == nil || !strings.Contains(err.Error(), "ResultCode 4") || !strings.Contains(err.Error(), "Failed") {
			t.Fatalf("wrap(%v) = %v, want human Failed message", runErr, err)
		}
	})

	t.Run("SucceededWithErrors is soft success", func(t *testing.T) {
		if err := wrapWindowsUpdateError(errors.New("exit status 103")); err != nil {
			t.Fatalf("code 3 soft success: got %v", err)
		}
	})

	t.Run("Succeeded exit is soft success", func(t *testing.T) {
		if err := wrapWindowsUpdateError(errors.New("exit status 102")); err != nil {
			t.Fatalf("code 2: got %v", err)
		}
	})

	t.Run("Aborted maps", func(t *testing.T) {
		err := wrapWindowsUpdateError(errors.New("exit status 105"))
		if err == nil || !strings.Contains(err.Error(), "Aborted") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("InProgress maps", func(t *testing.T) {
		err := wrapWindowsUpdateError(errors.New("exit status 101"))
		if err == nil || !strings.Contains(err.Error(), "InProgress") {
			t.Fatalf("got %v", err)
		}
	})

	// The bug: a COM failure under $ErrorActionPreference = 'Stop' exits 1, and
	// 1 was reported as "WUA ResultCode 1 (InProgress)".
	t.Run("generic PowerShell exit 1 is not InProgress", func(t *testing.T) {
		err := wrapWindowsUpdateError(errors.New("exit status 1"))
		if err == nil {
			t.Fatal("want an error for a PowerShell failure")
		}
		msg := err.Error()
		if strings.Contains(msg, "ResultCode 1") || strings.Contains(msg, "InProgress") {
			t.Fatalf("error %q must not claim a WUA ResultCode for an uncaught exception", msg)
		}
		if !strings.Contains(msg, "PowerShell") {
			t.Fatalf("error %q should name the PowerShell failure", msg)
		}
	})

	t.Run("generic PowerShell exit 5 is not Aborted", func(t *testing.T) {
		err := wrapWindowsUpdateError(errors.New("exit status 5"))
		if err == nil {
			t.Fatal("want an error for a PowerShell failure")
		}
		if strings.Contains(err.Error(), "Aborted") || strings.Contains(err.Error(), "ResultCode 5") {
			t.Fatalf("error %q must not claim a WUA ResultCode", err.Error())
		}
	})

	t.Run("offset out of range is a script failure", func(t *testing.T) {
		err := wrapWindowsUpdateError(errors.New("exit status 199"))
		if err == nil {
			t.Fatal("want an error")
		}
		if strings.Contains(err.Error(), "ResultCode 99") {
			t.Fatalf("error %q must not translate an out-of-range code", err.Error())
		}
	})

	t.Run("non-exit errors left unchanged", func(t *testing.T) {
		in := errors.New("connection reset")
		if err := wrapWindowsUpdateError(in); err != in {
			t.Fatalf("got %v, want original", err)
		}
	})
}

func TestSystemStep_windows_maps_exit_4_via_runner(t *testing.T) {
	present := func(names ...string) func(string) (string, error) {
		set := map[string]string{}
		for _, n := range names {
			set[n] = "/bin/" + n
		}
		return func(file string) (string, error) {
			if p, ok := set[file]; ok {
				return p, nil
			}
			return "", exec.ErrNotFound
		}
	}
	step := SystemStep(Env{Target: "windows", GOOS: "windows", LookPath: present("pwsh")})
	if step.MapError == nil {
		t.Fatal("windows SystemStep should set MapError")
	}
	got := Run(context.Background(), ModeApply, []Step{step}, func(ctx context.Context, cmd []string) error {
		return errors.New("exit status 104")
	})
	if len(got) != 1 || got[0].Status != StatusFailed {
		t.Fatalf("outcome = %#v, want failed", got)
	}
	msg := got[0].Reason
	if msg == "exit status 104" || !strings.Contains(msg, "Failed") || !strings.Contains(msg, "elevated PowerShell") {
		t.Fatalf("reason = %q, want human WUA Failed message", msg)
	}
}

func TestSystemStep_windows_soft_success_on_code_3(t *testing.T) {
	present := func(names ...string) func(string) (string, error) {
		set := map[string]string{}
		for _, n := range names {
			set[n] = "/bin/" + n
		}
		return func(file string) (string, error) {
			if p, ok := set[file]; ok {
				return p, nil
			}
			return "", exec.ErrNotFound
		}
	}
	step := SystemStep(Env{Target: "windows", GOOS: "windows", LookPath: present("pwsh")})
	got := Run(context.Background(), ModeApply, []Step{step}, func(ctx context.Context, cmd []string) error {
		return errors.New("exit status 103")
	})
	if len(got) != 1 || got[0].Status != StatusRan {
		t.Fatalf("outcome = %#v, want ran (soft success for SucceededWithErrors)", got)
	}
}

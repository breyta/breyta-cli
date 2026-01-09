package browseropen

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var startCommand = func(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

func Open(u string) error {
	u = strings.TrimSpace(u)
	if u == "" {
		return errors.New("missing url")
	}

	goos := runtime.GOOS
	wsl := false
	if goos == "linux" {
		wsl = isWSL()
	}
	return openFor(goos, wsl, strings.TrimSpace(os.Getenv("BROWSER")), u)
}

func openFor(goos string, wsl bool, browserEnv string, u string) error {
	switch goos {
	case "darwin":
		return startCommand("open", u)
	case "windows":
		return openWindows(u)
	default: // linux et al
		if goos == "linux" && wsl {
			if err := openWSL(u); err == nil {
				return nil
			}
		}

		// Respect BROWSER on unix-y systems (best effort).
		if err := openViaBrowserEnv(browserEnv, u); err == nil {
			return nil
		}

		return startCommand("xdg-open", u)
	}
}

func openWSL(u string) error {
	var errs []error
	candidates := [][]string{
		{"wslview", u},
		{"cmd.exe", "/c", "start", "", u},
		{"powershell.exe", "-NoProfile", "-Command", "Start-Process", u},
		{"explorer.exe", u},
	}
	for _, args := range candidates {
		if len(args) == 0 {
			continue
		}
		if err := startCommand(args[0], args[1:]...); err == nil {
			return nil
		} else {
			errs = append(errs, err)
		}
	}
	return fmt.Errorf("open browser failed (wsl): %v", errs)
}

func openWindows(u string) error {
	var errs []error
	candidates := [][]string{
		{"rundll32", "url.dll,FileProtocolHandler", u},
		{"cmd", "/c", "start", "", u},
		{"powershell", "-NoProfile", "-Command", "Start-Process", u},
		{"explorer", u},
	}
	for _, args := range candidates {
		if len(args) == 0 {
			continue
		}
		if err := startCommand(args[0], args[1:]...); err == nil {
			return nil
		} else {
			errs = append(errs, err)
		}
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("open browser failed: %v", errs)
}

func openViaBrowserEnv(raw string, u string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("BROWSER not set")
	}

	// Common convention: colon-separated list of browser commands.
	// Also allow a single command containing args.
	parts := strings.Split(raw, ":")
	var errs []error
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		argv := strings.Fields(part)
		if len(argv) == 0 {
			continue
		}
		if strings.Contains(part, "%s") {
			// If the user provided a placeholder, preserve their exact string and run via shell.
			// We avoid /bin/sh -c here to keep it simple and portable; best-effort with Fields.
			replaced := strings.ReplaceAll(part, "%s", u)
			argv = strings.Fields(replaced)
		} else {
			argv = append(argv, u)
		}
		if err := startCommand(argv[0], argv[1:]...); err == nil {
			return nil
		} else {
			errs = append(errs, err)
		}
	}

	return fmt.Errorf("open via BROWSER failed: %v", errs)
}

func isWSL() bool {
	// Fast env-based detection.
	if strings.TrimSpace(os.Getenv("WSL_INTEROP")) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv("WSL_DISTRO_NAME")) != "" {
		return true
	}

	// Heuristic: kernel release contains Microsoft.
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		if strings.Contains(strings.ToLower(string(b)), "microsoft") {
			return true
		}
	}
	if b, err := os.ReadFile("/proc/version"); err == nil {
		if strings.Contains(strings.ToLower(string(b)), "microsoft") {
			return true
		}
	}
	return false
}

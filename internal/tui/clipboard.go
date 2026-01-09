package tui

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func copyToClipboard(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("nothing to copy")
	}

	// Try common clipboard tools. Keep it simple and dependency-free.
	candidates := []struct {
		bin  string
		args []string
	}{
		{bin: "pbcopy", args: nil},
		{bin: "wl-copy", args: nil},
		{bin: "xclip", args: []string{"-selection", "clipboard"}},
	}

	for _, c := range candidates {
		if _, err := exec.LookPath(c.bin); err != nil {
			continue
		}
		cmd := exec.Command(c.bin, c.args...)
		cmd.Stdin = bytes.NewReader([]byte(text))
		if err := cmd.Run(); err != nil {
			return "", err
		}
		return c.bin, nil
	}

	return "", fmt.Errorf("no clipboard tool found (tried pbcopy, wl-copy, xclip)")
}


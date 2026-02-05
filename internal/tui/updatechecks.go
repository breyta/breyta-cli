package tui

import (
	"os"
	"strings"
)

func updateChecksEnabled() bool {
	return strings.TrimSpace(os.Getenv("BREYTA_NO_UPDATE_CHECK")) == ""
}

package cli

import (
	"fmt"
	"strings"
)

func parseCLITrueFalseFlag(name, raw string) (bool, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid --%s %q (use true or false)", name, raw)
	}
}

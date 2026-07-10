package cli

import (
	"fmt"
	"strconv"
	"strings"
)

func parseCLITrueFalseFlag(name, raw string) (bool, error) {
	value := strings.TrimSpace(raw)
	if parsed, err := strconv.ParseBool(value); err == nil {
		return parsed, nil
	} else {
		return false, fmt.Errorf("invalid --%s %q (use true or false)", name, raw)
	}
}

func parseFlowSlugAndCLITrueFalseFlag(name, raw string, args []string, flagChanged bool) (string, bool, error) {
	if len(args) == 0 {
		return "", false, fmt.Errorf("missing flow slug")
	}
	if len(args) > 2 {
		return "", false, fmt.Errorf("accepts 1 arg(s), received %d", len(args))
	}
	flowSlug := strings.TrimSpace(args[0])
	value := raw
	if len(args) == 2 {
		if !flagChanged {
			return "", false, fmt.Errorf("unexpected extra argument %q", args[1])
		}
		firstIsBool := isCLITrueFalseLiteral(args[0])
		secondIsBool := isCLITrueFalseLiteral(args[1])
		switch {
		case firstIsBool && secondIsBool:
			return "", false, fmt.Errorf("ambiguous --%s value and flow slug %q (use --%s=true or --%s=false)", name, args[1], name, name)
		case firstIsBool:
			flowSlug = strings.TrimSpace(args[1])
			value = args[0]
		case secondIsBool:
			value = args[1]
		default:
			return "", false, fmt.Errorf("unexpected extra argument %q", args[1])
		}
	}
	parsed, err := parseCLITrueFalseFlag(name, value)
	if err != nil {
		return "", false, err
	}
	return flowSlug, parsed, nil
}

func isCLITrueFalseLiteral(raw string) bool {
	_, err := strconv.ParseBool(strings.TrimSpace(raw))
	return err == nil
}

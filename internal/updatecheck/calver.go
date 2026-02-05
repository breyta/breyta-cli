package updatecheck

import (
	"fmt"
	"strconv"
	"strings"
)

type CalVer struct {
	Year  int
	Month int
	Patch int
}

func ParseCalVer(v string) (CalVer, error) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	// Allow dirty/dev version suffixes like:
	// v2026.1.2-17-g2376d07-dirty or v2026.1.2+local
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return CalVer{}, fmt.Errorf("invalid calver %q (expected vYYYY.M.PATCH)", v)
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return CalVer{}, fmt.Errorf("invalid year in %q", v)
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return CalVer{}, fmt.Errorf("invalid month in %q", v)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return CalVer{}, fmt.Errorf("invalid patch in %q", v)
	}
	if year < 2000 || year > 3000 {
		return CalVer{}, fmt.Errorf("invalid year %d in %q", year, v)
	}
	if month < 1 || month > 12 {
		return CalVer{}, fmt.Errorf("invalid month %d in %q", month, v)
	}
	if patch < 0 || patch > 10000 {
		return CalVer{}, fmt.Errorf("invalid patch %d in %q", patch, v)
	}
	return CalVer{Year: year, Month: month, Patch: patch}, nil
}

func (c CalVer) Compare(other CalVer) int {
	if c.Year != other.Year {
		if c.Year < other.Year {
			return -1
		}
		return 1
	}
	if c.Month != other.Month {
		if c.Month < other.Month {
			return -1
		}
		return 1
	}
	if c.Patch != other.Patch {
		if c.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

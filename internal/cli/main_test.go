package cli_test

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Keep CLI tests independent of the developer's installed skills and
	// 24-hour skill-sync cache. Tests covering automatic warnings opt back in.
	_ = os.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	os.Exit(m.Run())
}

package cli_test

import (
	"strings"
	"testing"

	"github.com/breyta/breyta-cli/internal/cli"
)

func TestOpenSourceCommandTreeIncludesSkillManagement(t *testing.T) {
	cmd := cli.NewRootCmd()
	for _, args := range [][]string{{"skills"}, {"skills", "install"}, {"skills", "status"}} {
		found, _, err := cmd.Find(args)
		if err != nil || found == nil {
			t.Fatalf("missing command %q: %v", strings.Join(args, " "), err)
		}
	}
}

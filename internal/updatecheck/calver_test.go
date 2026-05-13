package updatecheck

import "testing"

func TestParseCalVer(t *testing.T) {
	c, err := ParseCalVer("v2026.2.5")
	if err != nil {
		t.Fatalf("ParseCalVer: %v", err)
	}
	if c.Year != 2026 || c.Month != 2 || c.Patch != 5 {
		t.Fatalf("unexpected parsed calver: %+v", c)
	}
}

func TestParseCalVer_AllowsSuffix(t *testing.T) {
	c, err := ParseCalVer("v2026.1.2-17-g2376d07-dirty")
	if err != nil {
		t.Fatalf("ParseCalVer: %v", err)
	}
	if c.Year != 2026 || c.Month != 1 || c.Patch != 2 {
		t.Fatalf("unexpected parsed calver: %+v", c)
	}
}

func TestParseCalVer_AllowsGoModuleCompatibilityTag(t *testing.T) {
	c, err := ParseCalVer("v0.202605.6")
	if err != nil {
		t.Fatalf("ParseCalVer: %v", err)
	}
	if c.Year != 2026 || c.Month != 5 || c.Patch != 6 {
		t.Fatalf("unexpected parsed calver: %+v", c)
	}
}

func TestCalVerCompare(t *testing.T) {
	a := CalVer{Year: 2026, Month: 2, Patch: 1}
	b := CalVer{Year: 2026, Month: 2, Patch: 2}
	if a.Compare(b) != -1 {
		t.Fatalf("expected a<b")
	}
	if b.Compare(a) != 1 {
		t.Fatalf("expected b>a")
	}
	if b.Compare(b) != 0 {
		t.Fatalf("expected b==b")
	}
}

func TestUpdateAvailableComparesGoModuleCompatibilityTags(t *testing.T) {
	newer, err := isUpdateAvailable("v0.202605.5", "v2026.5.6")
	if err != nil {
		t.Fatalf("isUpdateAvailable: %v", err)
	}
	if !newer {
		t.Fatalf("expected v0.202605.5 to be older than v2026.5.6")
	}

	newer, err = isUpdateAvailable("v0.202605.6", "v2026.5.6")
	if err != nil {
		t.Fatalf("isUpdateAvailable: %v", err)
	}
	if newer {
		t.Fatalf("expected v0.202605.6 to match v2026.5.6")
	}
}

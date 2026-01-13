package parenrepair

import "testing"

func TestRepair_AppendsMissingClosesAtEOF(t *testing.T) {
	in := "(defn f [x]\n  (+ x 1)\n"
	out, rep, err := Repair(in, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !rep.Changed || rep.AppendedCloses != 1 || rep.UnclosedCount != 1 {
		t.Fatalf("unexpected report: %+v", rep)
	}
	want := "(defn f [x]\n  (+ x 1))\n"
	if out != want {
		t.Fatalf("unexpected out:\n%q\nwant:\n%q", out, want)
	}
}

func TestRepair_DropsUnexpectedClose(t *testing.T) {
	in := ")\n(def x 1)\n"
	out, rep, err := Repair(in, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !rep.Changed || rep.DroppedCloses != 1 {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if out != "\n(def x 1)\n" {
		t.Fatalf("unexpected out:\n%q", out)
	}
}

func TestRepair_ReplacesMismatchedClose(t *testing.T) {
	in := "(let [x 1]\n  x]\n"
	out, rep, err := Repair(in, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !rep.Changed || rep.ReplacedCloses != 1 {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if out != "(let [x 1]\n  x)\n" {
		t.Fatalf("unexpected out:\n%q", out)
	}
}

func TestRepair_DoesNotTouchDelimitersInStringsAndComments(t *testing.T) {
	in := "(def s \"[not a delim]\") ; ) ] }\n(def x 1)\n"
	out, rep, err := Repair(in, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rep.Changed {
		t.Fatalf("expected unchanged, got report: %+v", rep)
	}
	if out != in {
		t.Fatalf("unexpected out:\n%q", out)
	}
}

func TestRepair_UnterminatedStringErrors(t *testing.T) {
	_, rep, err := Repair("(def s \"oops)\n", true)
	if err == nil {
		t.Fatalf("expected err")
	}
	if err != ErrUnterminatedString {
		t.Fatalf("unexpected err: %v", err)
	}
	if !rep.UnterminatedStr {
		t.Fatalf("expected unterminatedStr in report")
	}
}

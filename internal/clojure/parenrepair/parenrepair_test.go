package parenrepair

import (
	"errors"
	"testing"
)

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

func TestRepair_DoesNotTouchDelimitersInCharacterLiterals(t *testing.T) {
	in := "(def open-paren \\()\n(def close-paren \\))\n(def open-bracket \\[)\n(def close-bracket \\])\n(def open-brace \\{)\n(def close-brace \\})\n(def quote \\\")\n(def backslash \\\\)\n(def semi \\;)\n(def named [\\space \\newline \\tab \\return \\backspace \\formfeed])\n(def escaped [\\u0041 \\o101])\n"
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
	if err := Check(in); err != nil {
		t.Fatalf("expected valid char literals to pass Check: %v", err)
	}
}

func TestRepair_DoesNotTouchDelimitersInRegexLiterals(t *testing.T) {
	in := "(def r #\"[\\\\(\\\\)\\\\[\\\\]{}]+\")\n"
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
	if err := Check(in); err != nil {
		t.Fatalf("expected regex delimiters to pass Check: %v", err)
	}
}

func TestCheckReportsDelimiterImbalance(t *testing.T) {
	err := Check("(defn f [x]\n  (+ x 1)\n")
	if !errors.Is(err, ErrUnbalancedDelimiters) {
		t.Fatalf("expected ErrUnbalancedDelimiters, got %v", err)
	}
}

func TestCheckReportsLexicalErrorsSeparately(t *testing.T) {
	err := Check("(def s \"oops)\n")
	if !errors.Is(err, ErrUnterminatedString) {
		t.Fatalf("expected ErrUnterminatedString, got %v", err)
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

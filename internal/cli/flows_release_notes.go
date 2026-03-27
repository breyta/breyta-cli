package cli

import (
	"fmt"
	"os"
	"strings"
)

func resolvePublishDescriptionInput(publishDescription string, publishDescriptionFile string) (string, error) {
	description := publishDescription
	if path := strings.TrimSpace(publishDescriptionFile); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read --publish-description-file: %w", err)
		}
		description = string(b)
	}
	return description, nil
}

func resolveReleaseNoteInput(releaseNote string, legacyNote string, releaseNoteFile string) (string, error) {
	note := strings.TrimSpace(releaseNote)
	if note == "" {
		note = strings.TrimSpace(legacyNote)
	}
	if path := strings.TrimSpace(releaseNoteFile); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read --release-note-file: %w", err)
		}
		note = string(b)
	}
	return note, nil
}

func releaseNoteHintCommands(flowSlug string, version int) []string {
	flowSlug = strings.TrimSpace(flowSlug)
	if flowSlug == "" {
		flowSlug = "<flow-slug>"
	}

	versionArg := "<version>"
	if version > 0 {
		versionArg = fmt.Sprintf("%d", version)
	}

	return []string{
		"breyta flows diff " + flowSlug,
		"breyta flows release " + flowSlug + " --release-note-file ./release-note.md",
		"breyta flows versions update " + flowSlug + " --version " + versionArg + " --release-note-file ./release-note.md",
	}
}

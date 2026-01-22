package cli

import "testing"

func TestPickInstallationUploadTrigger(t *testing.T) {
	t.Run("defaults to only trigger", func(t *testing.T) {
		got, err := pickInstallationUploadTrigger([]installationTrigger{
			{TriggerID: "trig-1", EventName: "upload", EventPath: "webhooks/flow/upload/prof"},
		}, "")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.TriggerID != "trig-1" {
			t.Fatalf("expected trig-1, got %q", got.TriggerID)
		}
	})

	t.Run("requires selector when multiple", func(t *testing.T) {
		_, err := pickInstallationUploadTrigger([]installationTrigger{
			{TriggerID: "trig-1", EventName: "a", EventPath: "webhooks/a"},
			{TriggerID: "trig-2", EventName: "b", EventPath: "webhooks/b"},
		}, "")
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("selects by event name", func(t *testing.T) {
		got, err := pickInstallationUploadTrigger([]installationTrigger{
			{TriggerID: "trig-1", EventName: "upload", EventPath: "webhooks/a"},
			{TriggerID: "trig-2", EventName: "other", EventPath: "webhooks/b"},
		}, "upload")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.TriggerID != "trig-1" {
			t.Fatalf("expected trig-1, got %q", got.TriggerID)
		}
	})
}

package live

import (
	"bytes"
	"context"
	"testing"
)

func TestDecodeSnapshotSSE_IgnoresTypedMessageWithoutSnapshot(t *testing.T) {
	ctx := context.Background()
	snapshots := make(chan Snapshot, 1)

	if err := decodeSnapshotSSE(ctx, bytes.NewBufferString("event: message\ndata: {\"type\":\"ping\"}\n\n"), snapshots); err != nil {
		t.Fatalf("decodeSnapshotSSE returned error: %v", err)
	}
	select {
	case snapshot := <-snapshots:
		t.Fatalf("expected typed control message to be ignored, got %#v", snapshot)
	default:
	}
}

func TestDecodeSnapshotSSE_EmitsWrappedSnapshot(t *testing.T) {
	ctx := context.Background()
	snapshots := make(chan Snapshot, 1)

	if err := decodeSnapshotSSE(ctx, bytes.NewBufferString("event: message\ndata: {\"type\":\"workspace_snapshot\",\"snapshot\":{\"workspace\":{\"workspace_id\":\"ws-acme\"}}}\n\n"), snapshots); err != nil {
		t.Fatalf("decodeSnapshotSSE returned error: %v", err)
	}
	snapshot := <-snapshots
	if snapshot.Workspace.WorkspaceID != "ws-acme" {
		t.Fatalf("expected wrapped snapshot, got %#v", snapshot)
	}
}

func TestDecodeSnapshotSSE_NormalizesCRLFLineEndings(t *testing.T) {
	ctx := context.Background()
	snapshots := make(chan Snapshot, 1)
	stream := "event: message\r\ndata: {\"type\":\"workspace_snapshot\",\"snapshot\":{\"workspace\":{\"workspace_id\":\"ws-crlf\"}}}\r\n\r\n"

	if err := decodeSnapshotSSE(ctx, bytes.NewBufferString(stream), snapshots); err != nil {
		t.Fatalf("decodeSnapshotSSE returned error: %v", err)
	}
	snapshot := <-snapshots
	if snapshot.Workspace.WorkspaceID != "ws-crlf" {
		t.Fatalf("expected CRLF snapshot, got %#v", snapshot)
	}
}

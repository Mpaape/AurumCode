package webhook

import (
	"errors"
	"testing"
)

func TestParsePullRequest_Opened(t *testing.T) {
	parser := NewGitHubEventParser()

	event, err := parser.Parse(
		"pull_request",
		"delivery-123",
		"sha256=abc",
		[]byte(pullRequestOpenedPayload),
	)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if event.Repo != "owner/repo" {
		t.Errorf("expected repo 'owner/repo', got: %s", event.Repo)
	}

	if event.Provider != "github" {
		t.Errorf("expected provider 'github', got: %s", event.Provider)
	}

	if event.EventType != "pull_request.opened" {
		t.Errorf("expected event type 'pull_request.opened', got: %s", event.EventType)
	}

	if event.DeliveryID != "delivery-123" {
		t.Errorf("expected delivery ID 'delivery-123', got: %s", event.DeliveryID)
	}

	if event.Signature != "sha256=abc" {
		t.Errorf("expected signature 'sha256=abc', got: %s", event.Signature)
	}

	if len(event.Payload) == 0 {
		t.Error("expected payload to be preserved")
	}
}

func TestParsePullRequest_Synchronize(t *testing.T) {
	parser := NewGitHubEventParser()

	event, err := parser.Parse(
		"pull_request",
		"delivery-456",
		"sha256=def",
		[]byte(pullRequestSynchronizePayload),
	)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if event.EventType != "pull_request.synchronize" {
		t.Errorf("expected event type 'pull_request.synchronize', got: %s", event.EventType)
	}
}

func TestParsePullRequest_Closed(t *testing.T) {
	parser := NewGitHubEventParser()

	_, err := parser.Parse(
		"pull_request",
		"delivery-789",
		"sha256=ghi",
		[]byte(pullRequestClosedPayload),
	)

	// Closed actions are not supported
	if !errors.Is(err, ErrUnsupportedEvent) {
		t.Errorf("expected ErrUnsupportedEvent for closed PR, got: %v", err)
	}
}

func TestParsePush_Main(t *testing.T) {
	parser := NewGitHubEventParser()

	event, err := parser.Parse(
		"push",
		"delivery-push-123",
		"sha256=push",
		[]byte(pushPayload),
	)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if event.Repo != "owner/repo" {
		t.Errorf("expected repo 'owner/repo', got: %s", event.Repo)
	}

	if event.EventType != "push" {
		t.Errorf("expected event type 'push', got: %s", event.EventType)
	}

	if event.Provider != "github" {
		t.Errorf("expected provider 'github', got: %s", event.Provider)
	}
}

func TestParsePush_NonMain(t *testing.T) {
	parser := NewGitHubEventParser()

	_, err := parser.Parse(
		"push",
		"delivery-push-456",
		"sha256=push",
		[]byte(pushNonMainPayload),
	)

	// Pushes to non-main branches are not supported
	if !errors.Is(err, ErrUnsupportedEvent) {
		t.Errorf("expected ErrUnsupportedEvent for non-main push, got: %v", err)
	}
}

func TestParse_UnsupportedEventType(t *testing.T) {
	parser := NewGitHubEventParser()

	_, err := parser.Parse(
		"issues",
		"delivery-issues",
		"sha256=issues",
		[]byte(`{"action":"opened"}`),
	)

	if !errors.Is(err, ErrUnsupportedEvent) {
		t.Errorf("expected ErrUnsupportedEvent, got: %v", err)
	}
}

func TestParse_MissingEventType(t *testing.T) {
	parser := NewGitHubEventParser()

	_, err := parser.Parse(
		"",
		"delivery-123",
		"sha256=abc",
		[]byte(`{}`),
	)

	if !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("expected ErrInvalidPayload for missing event type, got: %v", err)
	}
}

func TestParse_MissingDeliveryID(t *testing.T) {
	parser := NewGitHubEventParser()

	_, err := parser.Parse(
		"pull_request",
		"",
		"sha256=abc",
		[]byte(pullRequestOpenedPayload),
	)

	if !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("expected ErrInvalidPayload for missing delivery ID, got: %v", err)
	}
}

func TestParse_InvalidJSON(t *testing.T) {
	parser := NewGitHubEventParser()

	_, err := parser.Parse(
		"pull_request",
		"delivery-123",
		"sha256=abc",
		[]byte(`{invalid json`),
	)

	if !errors.Is(err, ErrInvalidPayload) {
		t.Errorf("expected ErrInvalidPayload for invalid JSON, got: %v", err)
	}
}

func TestParse_EmptyPayload(t *testing.T) {
	parser := NewGitHubEventParser()

	_, err := parser.Parse(
		"pull_request",
		"delivery-123",
		"sha256=abc",
		[]byte(`{}`),
	)

	// Empty payload will be parsed but might fail on required fields
	// This should return an error
	if err == nil {
		t.Error("expected error for empty payload")
	}
}

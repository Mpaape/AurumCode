package webhook

import (
	"aurumcode/pkg/types"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrUnsupportedEvent indicates the event type is not supported
	ErrUnsupportedEvent = errors.New("unsupported event type")

	// ErrInvalidPayload indicates the payload format is invalid
	ErrInvalidPayload = errors.New("invalid payload format")
)

// GitHubEventParser parses GitHub webhook events
type GitHubEventParser struct{}

// NewGitHubEventParser creates a new GitHub event parser
func NewGitHubEventParser() *GitHubEventParser {
	return &GitHubEventParser{}
}

// Parse parses a GitHub webhook event into types.Event
// eventType is from X-GitHub-Event header
// deliveryID is from X-GitHub-Delivery header
// signature is from X-Hub-Signature-256 header
// payload is the raw request body
func (p *GitHubEventParser) Parse(eventType, deliveryID, signature string, payload []byte) (*types.Event, error) {
	// Validate inputs
	if eventType == "" {
		return nil, fmt.Errorf("%w: missing event type", ErrInvalidPayload)
	}

	if deliveryID == "" {
		return nil, fmt.Errorf("%w: missing delivery ID", ErrInvalidPayload)
	}

	// Parse based on event type
	switch eventType {
	case "pull_request":
		return p.parsePullRequest(deliveryID, signature, payload)
	case "push":
		return p.parsePush(deliveryID, signature, payload)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedEvent, eventType)
	}
}

// parsePullRequest parses a pull_request event
func (p *GitHubEventParser) parsePullRequest(deliveryID, signature string, payload []byte) (*types.Event, error) {
	var data struct {
		Action string `json:"action"`
		Number int    `json:"number"`
		PullRequest struct {
			Number int `json:"number"`
			Head struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
		} `json:"pull_request"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	// Only process opened and synchronize actions
	if data.Action != "opened" && data.Action != "synchronize" {
		return nil, fmt.Errorf("%w: pull_request action %s", ErrUnsupportedEvent, data.Action)
	}

	event := &types.Event{
		Repo:       data.Repository.FullName,
		Provider:   "github",
		EventType:  fmt.Sprintf("pull_request.%s", data.Action),
		DeliveryID: deliveryID,
		Payload:    payload,
		Signature:  signature,
	}

	return event, nil
}

// parsePush parses a push event
func (p *GitHubEventParser) parsePush(deliveryID, signature string, payload []byte) (*types.Event, error) {
	var data struct {
		Ref string `json:"ref"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	// Only process pushes to main/master
	if !strings.HasSuffix(data.Ref, "/main") && !strings.HasSuffix(data.Ref, "/master") {
		return nil, fmt.Errorf("%w: push to non-main branch %s", ErrUnsupportedEvent, data.Ref)
	}

	event := &types.Event{
		Repo:       data.Repository.FullName,
		Provider:   "github",
		EventType:  "push",
		DeliveryID: deliveryID,
		Payload:    payload,
		Signature:  signature,
	}

	return event, nil
}

// Package dag validates an atomic-card dependency graph and answers which
// cards are immediately releasable. It is side-effect free: callers remain
// responsible for authentication, scheduling, card transitions, and writes.
package dag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
)

const (
	SchemaName    = "aurum.dag-release-query"
	SchemaVersion = 1
	ResultSchema  = "aurum.dag-release-result"

	MaxCards               = 64
	MaxDependenciesPerCard = 64
	MaxAuthorizationEvents = 64
)

type Status string

const (
	StatusBacklog        Status = "backlog"
	StatusReady          Status = "ready"
	StatusDoing          Status = "doing"
	StatusReview         Status = "review"
	StatusValidating     Status = "validating"
	StatusDone           Status = "done"
	StatusBlockedOnOwner Status = "blocked-on-owner"
	StatusCancelled      Status = "cancelled"
)

const (
	ActorHuman = "human"
	ActorAgent = "agent"
)

const (
	CodeBehaviorMissing        = "behavior_missing"
	CodeInvalidQuery           = "invalid_query"
	CodeLimitExceeded          = "limit_exceeded"
	CodeDuplicateCard          = "duplicate_card"
	CodeDuplicateDependency    = "duplicate_dependency"
	CodeMissingDependency      = "missing_dependency"
	CodeCycle                  = "cycle"
	CodeInvalidStatus          = "invalid_status"
	CodeIllegalTransition      = "illegal_transition"
	CodeAgentIdentityForbidden = "agent_identity_forbidden"
	CodeInvalidAuthorization   = "invalid_authorization"
)

// CandidateIdentityV1 binds deterministic output to the caller's already
// computed candidate identity. This package validates the digest shape; it
// does not calculate or authenticate repository identity.
type CandidateIdentityV1 struct {
	Digest string `json:"digest"`
}

// Card is one graph node and the status transition presented to the query.
type Card struct {
	ID             string   `json:"id"`
	PreviousStatus Status   `json:"previous_status"`
	Status         Status   `json:"status"`
	DependsOn      []string `json:"depends_on"`
}

// AuthorizationEvent is a bounded, preverified event from the authorization
// boundary. It can validate a presented transition but can never make a
// dependency done or otherwise alter graph state. Agent identities are always
// refused because a self-asserted agent event is not publication authority.
type AuthorizationEvent struct {
	CardID    string `json:"card_id"`
	ActorType string `json:"actor_type"`
	Decision  string `json:"decision"`
}

// Query is the complete, immutable input to Release.
type Query struct {
	Schema              string               `json:"schema"`
	Version             int                  `json:"version"`
	CandidateIdentity   CandidateIdentityV1  `json:"candidate_identity"`
	Cards               []Card               `json:"cards"`
	AuthorizationEvents []AuthorizationEvent `json:"authorization_events"`
}

// Result is deterministic for a Query. Releasable contains only cards already
// in ready whose dependencies are already done; it is not a future schedule.
type Result struct {
	Schema                  string   `json:"schema"`
	Version                 int      `json:"version"`
	CandidateIdentityDigest string   `json:"candidate_identity_digest"`
	TopologicalOrder        []string `json:"topological_order"`
	Releasable              []string `json:"releasable"`
	Effects                 int      `json:"effects"`
	ArtifactDigest          string   `json:"artifact_digest"`
}

// ValidationError contains only bounded structural labels and never echoes
// untrusted values.
type ValidationError struct {
	Code   string
	Field  string
	CardID string
}

func (e *ValidationError) Error() string {
	if e.CardID == "" {
		return fmt.Sprintf("dag: %s: %s", e.Field, e.Code)
	}
	return fmt.Sprintf("dag: %s: %s: %s", e.Field, e.Code, e.CardID)
}

var (
	cardIDPattern = regexp.MustCompile(`^AUR-[0-9]{3}$`)
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

var validStatuses = map[Status]bool{
	StatusBacklog:        true,
	StatusReady:          true,
	StatusDoing:          true,
	StatusReview:         true,
	StatusValidating:     true,
	StatusDone:           true,
	StatusBlockedOnOwner: true,
	StatusCancelled:      true,
}

// Release validates the complete graph before returning any scheduling
// answer. Validation completes before a Result is constructed, so callers can
// never mistake a partial order for a release decision.
func Release(query Query) (Result, error) {
	if query.Schema != SchemaName {
		return Result{}, validationError(CodeInvalidQuery, "schema", "")
	}
	if query.Version != SchemaVersion {
		return Result{}, validationError(CodeInvalidQuery, "version", "")
	}
	if !digestPattern.MatchString(query.CandidateIdentity.Digest) {
		return Result{}, validationError(CodeInvalidQuery, "candidate_identity.digest", "")
	}
	if len(query.Cards) == 0 {
		return Result{}, validationError(CodeInvalidQuery, "cards", "")
	}
	if len(query.Cards) > MaxCards {
		return Result{}, validationError(CodeLimitExceeded, "cards", "")
	}
	if len(query.AuthorizationEvents) > MaxAuthorizationEvents {
		return Result{}, validationError(CodeLimitExceeded, "authorization_events", "")
	}

	cards := make(map[string]Card, len(query.Cards))
	cardIndexes := make(map[string]int, len(query.Cards))
	for index, card := range query.Cards {
		idField := fmt.Sprintf("cards[%d].id", index)
		if !cardIDPattern.MatchString(card.ID) {
			return Result{}, validationError(CodeInvalidQuery, idField, "")
		}
		if _, exists := cards[card.ID]; exists {
			return Result{}, validationError(CodeDuplicateCard, idField, card.ID)
		}
		if !validStatuses[card.PreviousStatus] {
			return Result{}, validationError(CodeInvalidStatus, fmt.Sprintf("cards[%d].previous_status", index), card.ID)
		}
		if !validStatuses[card.Status] {
			return Result{}, validationError(CodeInvalidStatus, fmt.Sprintf("cards[%d].status", index), card.ID)
		}
		if !legalTransition(card.PreviousStatus, card.Status) {
			return Result{}, validationError(CodeIllegalTransition, fmt.Sprintf("cards[%d].status", index), card.ID)
		}
		if len(card.DependsOn) > MaxDependenciesPerCard {
			return Result{}, validationError(CodeLimitExceeded, fmt.Sprintf("cards[%d].depends_on", index), card.ID)
		}
		dependencies := make(map[string]bool, len(card.DependsOn))
		for dependencyIndex, dependency := range card.DependsOn {
			field := fmt.Sprintf("cards[%d].depends_on[%d]", index, dependencyIndex)
			if !cardIDPattern.MatchString(dependency) {
				return Result{}, validationError(CodeInvalidQuery, field, card.ID)
			}
			if dependencies[dependency] {
				return Result{}, validationError(CodeDuplicateDependency, field, card.ID)
			}
			dependencies[dependency] = true
		}
		cards[card.ID] = Card{
			ID:             card.ID,
			PreviousStatus: card.PreviousStatus,
			Status:         card.Status,
			DependsOn:      append([]string(nil), card.DependsOn...),
		}
		cardIndexes[card.ID] = index
	}

	seenEvents := make(map[string]bool, len(query.AuthorizationEvents))
	for index, event := range query.AuthorizationEvents {
		actorField := fmt.Sprintf("authorization_events[%d].actor_type", index)
		if event.ActorType == ActorAgent {
			return Result{}, validationError(CodeAgentIdentityForbidden, actorField, safeCardID(event.CardID))
		}
		if event.ActorType != ActorHuman || event.Decision != "approve" || !cardIDPattern.MatchString(event.CardID) {
			return Result{}, validationError(CodeInvalidAuthorization, fmt.Sprintf("authorization_events[%d]", index), safeCardID(event.CardID))
		}
		if _, exists := cards[event.CardID]; !exists {
			return Result{}, validationError(CodeInvalidAuthorization, fmt.Sprintf("authorization_events[%d].card_id", index), event.CardID)
		}
		key := event.CardID + "\x00" + event.Decision
		if seenEvents[key] {
			return Result{}, validationError(CodeInvalidAuthorization, fmt.Sprintf("authorization_events[%d]", index), event.CardID)
		}
		seenEvents[key] = true
	}

	for _, card := range query.Cards {
		for dependencyIndex, dependency := range card.DependsOn {
			if _, exists := cards[dependency]; !exists {
				index := cardIndexes[card.ID]
				field := fmt.Sprintf("cards[%d].depends_on[%d]", index, dependencyIndex)
				return Result{}, validationError(CodeMissingDependency, field, card.ID)
			}
		}
	}

	order, ok := topologicalOrder(cards)
	if !ok {
		return Result{}, validationError(CodeCycle, "cards", "")
	}

	releasable := make([]string, 0, len(order))
	for _, id := range order {
		card := cards[id]
		if card.Status != StatusReady {
			continue
		}
		allDone := true
		for _, dependency := range card.DependsOn {
			if cards[dependency].Status != StatusDone {
				allDone = false
				break
			}
		}
		if allDone {
			releasable = append(releasable, id)
		}
	}

	result := Result{
		Schema:                  ResultSchema,
		Version:                 SchemaVersion,
		CandidateIdentityDigest: query.CandidateIdentity.Digest,
		TopologicalOrder:        append([]string(nil), order...),
		Releasable:              append([]string(nil), releasable...),
		Effects:                 0,
	}
	result.ArtifactDigest = resultDigest(result)
	return result, nil
}

func legalTransition(from, to Status) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusBacklog:
		return to == StatusReady || to == StatusBlockedOnOwner || to == StatusCancelled
	case StatusReady:
		return to == StatusDoing || to == StatusReview || to == StatusValidating || to == StatusCancelled
	case StatusDoing:
		return to == StatusReview || to == StatusValidating || to == StatusCancelled
	case StatusReview:
		return to == StatusValidating || to == StatusDone || to == StatusCancelled
	case StatusValidating:
		return to == StatusDone || to == StatusCancelled
	case StatusBlockedOnOwner:
		return to == StatusBacklog || to == StatusReady || to == StatusCancelled
	default:
		return false
	}
}

func topologicalOrder(cards map[string]Card) ([]string, bool) {
	indegree := make(map[string]int, len(cards))
	dependents := make(map[string][]string, len(cards))
	ready := make([]string, 0, len(cards))
	for id, card := range cards {
		indegree[id] = len(card.DependsOn)
		for _, dependency := range card.DependsOn {
			dependents[dependency] = append(dependents[dependency], id)
		}
	}
	for dependency := range dependents {
		sort.Strings(dependents[dependency])
	}
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	order := make([]string, 0, len(cards))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, dependent := range dependents[id] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.Strings(ready)
			}
		}
	}
	return order, len(order) == len(cards)
}

func resultDigest(result Result) string {
	artifact := struct {
		Schema                  string   `json:"schema"`
		Version                 int      `json:"version"`
		CandidateIdentityDigest string   `json:"candidate_identity_digest"`
		TopologicalOrder        []string `json:"topological_order"`
		Releasable              []string `json:"releasable"`
		Effects                 int      `json:"effects"`
	}{
		Schema:                  result.Schema,
		Version:                 result.Version,
		CandidateIdentityDigest: result.CandidateIdentityDigest,
		TopologicalOrder:        result.TopologicalOrder,
		Releasable:              result.Releasable,
		Effects:                 result.Effects,
	}
	encoded, _ := json.Marshal(artifact)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validationError(code, field, cardID string) *ValidationError {
	return &ValidationError{Code: code, Field: field, CardID: cardID}
}

func safeCardID(value string) string {
	if cardIDPattern.MatchString(value) {
		return value
	}
	return ""
}

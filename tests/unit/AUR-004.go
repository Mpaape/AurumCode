// This harness is a standalone program invoked by its acceptance script as
// `go run tests/unit/AUR-004.go`, while every sibling in this directory belongs to the
// shared test package. Go allows one package per directory, so without this
// constraint the two clauses collide and `go build ./...` -- the whole CI
// build and the race suite -- fails before compiling a single line. The
// constraint excludes the file from package resolution; naming it explicitly
// on the command line still runs it.
//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/Mpaape/AurumCode/internal/governance/dag"
)

const candidateDigestAUR004 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

func nominalQueryAUR004() dag.Query {
	return dag.Query{
		Schema:            dag.SchemaName,
		Version:           dag.SchemaVersion,
		CandidateIdentity: dag.CandidateIdentityV1{Digest: candidateDigestAUR004},
		Cards: []dag.Card{
			{ID: "AUR-003", PreviousStatus: dag.StatusReady, Status: dag.StatusReady, DependsOn: []string{"AUR-002"}},
			{ID: "AUR-001", PreviousStatus: dag.StatusReview, Status: dag.StatusDone},
			{ID: "AUR-004", PreviousStatus: dag.StatusBacklog, Status: dag.StatusReady},
			{ID: "AUR-002", PreviousStatus: dag.StatusBacklog, Status: dag.StatusReady, DependsOn: []string{"AUR-001"}},
		},
	}
}

func validationCodeAUR004(err error) string {
	if typed, ok := err.(*dag.ValidationError); ok {
		return typed.Code
	}
	return ""
}

func requireCodeAUR004(err error, code string) error {
	if err == nil {
		return fmt.Errorf("expected %s, got success", code)
	}
	if actual := validationCodeAUR004(err); actual != code {
		return fmt.Errorf("expected %s, got %T/%s", code, err, actual)
	}
	return nil
}

func boundaryQueryAUR004(count int) dag.Query {
	query := dag.Query{
		Schema:            dag.SchemaName,
		Version:           dag.SchemaVersion,
		CandidateIdentity: dag.CandidateIdentityV1{Digest: candidateDigestAUR004},
		Cards:             make([]dag.Card, count),
	}
	for index := range query.Cards {
		query.Cards[index] = dag.Card{
			ID:             fmt.Sprintf("AUR-%03d", 100+index),
			PreviousStatus: dag.StatusReady,
			Status:         dag.StatusReady,
		}
	}
	return query
}

// TestAUR004 is the card's unit selector. The acceptance compiles this file as
// a focused executable and calls this function; a zero-assertion path cannot
// return success.
func TestAUR004() (int, error) {
	assertions := 0
	nominal := nominalQueryAUR004()
	before, err := json.Marshal(nominal)
	if err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/infrastructure: marshal input: %w", err)
	}

	result, err := dag.Release(nominal)
	if validationCodeAUR004(err) == dag.CodeBehaviorMissing {
		return assertions, fmt.Errorf("AUR-004/AC-001/behavior-missing")
	}
	if err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: nominal: %w", err)
	}
	assertions++
	if wanted := []string{"AUR-001", "AUR-002", "AUR-003", "AUR-004"}; !reflect.DeepEqual(result.TopologicalOrder, wanted) {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: order=%v", result.TopologicalOrder)
	}
	assertions++
	if wanted := []string{"AUR-002", "AUR-004"}; !reflect.DeepEqual(result.Releasable, wanted) {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: releasable=%v", result.Releasable)
	}
	assertions++
	if result.Effects != 0 || result.ArtifactDigest == "" || result.CandidateIdentityDigest != candidateDigestAUR004 {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: incomplete result")
	}
	assertions++
	replay, err := dag.Release(nominal)
	if err != nil || !reflect.DeepEqual(result, replay) {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: replay changed")
	}
	assertions++
	after, err := json.Marshal(nominal)
	if err != nil || !reflect.DeepEqual(before, after) {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: input mutated")
	}
	assertions++

	missing := nominalQueryAUR004()
	missing.Cards[0].DependsOn = []string{"AUR-999"}
	if err := requireCodeAUR004(releaseErrorAUR004(missing), dag.CodeMissingDependency); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: missing dependency: %w", err)
	}
	assertions++
	duplicateCard := nominalQueryAUR004()
	duplicateCard.Cards = append(duplicateCard.Cards, duplicateCard.Cards[3])
	if err := requireCodeAUR004(releaseErrorAUR004(duplicateCard), dag.CodeDuplicateCard); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: duplicate card: %w", err)
	}
	assertions++
	duplicateDependency := nominalQueryAUR004()
	duplicateDependency.Cards[0].DependsOn = []string{"AUR-002", "AUR-002"}
	if err := requireCodeAUR004(releaseErrorAUR004(duplicateDependency), dag.CodeDuplicateDependency); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: duplicate dependency: %w", err)
	}
	assertions++
	invalidStatus := nominalQueryAUR004()
	invalidStatus.Cards[0].Status = dag.Status("future")
	if err := requireCodeAUR004(releaseErrorAUR004(invalidStatus), dag.CodeInvalidStatus); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: invalid status: %w", err)
	}
	assertions++

	illegal := nominalQueryAUR004()
	illegal.Cards[1].PreviousStatus = dag.StatusBacklog
	if err := requireCodeAUR004(releaseErrorAUR004(illegal), dag.CodeIllegalTransition); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: transition: %w", err)
	}
	assertions++
	invalidAuthorization := nominalQueryAUR004()
	invalidAuthorization.AuthorizationEvents = []dag.AuthorizationEvent{{CardID: "AUR-002", ActorType: "robot", Decision: "approve"}}
	if err := requireCodeAUR004(releaseErrorAUR004(invalidAuthorization), dag.CodeInvalidAuthorization); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: invalid authorization: %w", err)
	}
	assertions++

	cycle := nominalQueryAUR004()
	cycle.Cards[1].DependsOn = []string{"AUR-003"}
	_, err = dag.Release(cycle)
	if err == nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/MUT-001: cycle accepted")
	}
	if err := requireCodeAUR004(err, dag.CodeCycle); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: cycle: %w", err)
	}
	assertions++

	agent := nominalQueryAUR004()
	agent.AuthorizationEvents = []dag.AuthorizationEvent{{CardID: "AUR-002", ActorType: dag.ActorAgent, Decision: "approve"}}
	_, err = dag.Release(agent)
	if err == nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/MUT-002: agent approval accepted")
	}
	if err := requireCodeAUR004(err, dag.CodeAgentIdentityForbidden); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: agent identity: %w", err)
	}
	assertions++

	human := nominalQueryAUR004()
	human.Cards[1].Status = dag.StatusReview
	human.Cards[1].PreviousStatus = dag.StatusReview
	human.AuthorizationEvents = []dag.AuthorizationEvent{{CardID: "AUR-002", ActorType: dag.ActorHuman, Decision: "approve"}}
	humanResult, err := dag.Release(human)
	if err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: bounded authorization event: %w", err)
	}
	if reflect.DeepEqual(humanResult.Releasable, result.Releasable) || containsAUR004(humanResult.Releasable, "AUR-003") {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: approval granted dependency state")
	}
	assertions++

	boundary := boundaryQueryAUR004(dag.MaxCards)
	boundaryResult, err := dag.Release(boundary)
	if err != nil || len(boundaryResult.Releasable) != dag.MaxCards {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: boundary")
	}
	assertions++
	if err := requireCodeAUR004(releaseErrorAUR004(boundaryQueryAUR004(dag.MaxCards+1)), dag.CodeLimitExceeded); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: boundary overflow: %w", err)
	}
	assertions++
	dependencyOverflow := nominalQueryAUR004()
	dependencyOverflow.Cards[0].DependsOn = make([]string, dag.MaxDependenciesPerCard+1)
	for index := range dependencyOverflow.Cards[0].DependsOn {
		dependencyOverflow.Cards[0].DependsOn[index] = fmt.Sprintf("AUR-%03d", 100+index)
	}
	if err := requireCodeAUR004(releaseErrorAUR004(dependencyOverflow), dag.CodeLimitExceeded); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: dependency overflow: %w", err)
	}
	assertions++
	eventOverflow := nominalQueryAUR004()
	eventOverflow.AuthorizationEvents = make([]dag.AuthorizationEvent, dag.MaxAuthorizationEvents+1)
	if err := requireCodeAUR004(releaseErrorAUR004(eventOverflow), dag.CodeLimitExceeded); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: event overflow: %w", err)
	}
	assertions++
	empty := nominalQueryAUR004()
	empty.Cards = nil
	if err := requireCodeAUR004(releaseErrorAUR004(empty), dag.CodeInvalidQuery); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: empty graph: %w", err)
	}
	assertions++

	canary := nominalQueryAUR004()
	canary.CandidateIdentity.Digest = "AURUM_SECRET_CANARY"
	_, err = dag.Release(canary)
	if err == nil || strings.Contains(err.Error(), "AURUM_SECRET_CANARY") {
		return assertions, fmt.Errorf("AUR-004/AC-001/assertion-failed: canary escaped")
	}
	assertions++
	return assertions, nil
}

func releaseErrorAUR004(query dag.Query) error {
	_, err := dag.Release(query)
	return err
}

func containsAUR004(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func main() {
	if len(os.Args) != 2 || os.Args[1] != "TestAUR004" {
		fmt.Fprintln(os.Stderr, "AUR-004/AC-001/unknown-selector")
		os.Exit(64)
	}
	assertions, err := TestAUR004()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if assertions == 0 {
		fmt.Fprintln(os.Stderr, "AUR-004/AC-001/zero-assertions")
		os.Exit(1)
	}
	fmt.Printf("{\"card\":\"AUR-004\",\"scenario\":\"AC-001\",\"selector\":\"TestAUR004\",\"assertions\":%d,\"result\":\"pass\"}\n", assertions)
}

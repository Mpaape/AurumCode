//go:build aur309_characterization

package integration

import (
	"testing"

	characterization "github.com/Mpaape/AurumCode/tests/characterization/legacy/pipeline"
)

func IntegrationAUR309(t *testing.T) {
	observations := characterization.Characterize(t)
	characterization.AssertFullyObserved(t, observations)
	characterization.PrintObservations(observations)
}

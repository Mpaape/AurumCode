//go:build aur309_characterization

package contracts

import (
	"testing"

	characterization "github.com/Mpaape/AurumCode/tests/characterization/legacy/pipeline"
)

func ContractAUR309(t *testing.T) {
	observations := characterization.Characterize(t)
	characterization.AssertFullyObserved(t, observations)
}

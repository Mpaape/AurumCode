package feedback

import (
	"encoding/json"
	"fmt"
	"sort"
)

// LedgerPath is where the policy repository records the signals a feedback
// pull request already carried. The state lives in GitHub, in the policy
// repository itself: no database of its own.
const LedgerPath = "realimentacao/sinais.json"

// Ledger is the list of signal ids already proposed.
type Ledger struct {
	Signals []string `json:"sinais"`
}

// ParseLedger reads a ledger. Absent content is an empty ledger; content
// that does not parse is an error, never an empty ledger (which would
// propose everything again).
func ParseLedger(data []byte, found bool) (Ledger, error) {
	if !found {
		return Ledger{}, nil
	}
	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return Ledger{}, fmt.Errorf("%s: %w", LedgerPath, err)
	}
	return l, nil
}

// Has reports whether id was already proposed.
func (l Ledger) Has(id string) bool {
	for _, s := range l.Signals {
		if s == id {
			return true
		}
	}
	return false
}

// Fresh returns the signals the ledgers do not know yet.
func Fresh(signals []Signal, ledgers ...Ledger) []Signal {
	var out []Signal
	for _, s := range signals {
		known := false
		for _, l := range ledgers {
			if l.Has(s.ID) {
				known = true
				break
			}
		}
		if !known {
			out = append(out, s)
		}
	}
	return out
}

// With returns the ledger plus the ids of signals, sorted and unique.
func (l Ledger) With(signals []Signal) Ledger {
	set := map[string]bool{}
	for _, id := range l.Signals {
		set[id] = true
	}
	for _, s := range signals {
		set[s.ID] = true
	}
	out := Ledger{Signals: make([]string, 0, len(set))}
	for id := range set {
		out.Signals = append(out.Signals, id)
	}
	sort.Strings(out.Signals)
	return out
}

// JSON renders the ledger deterministically.
func (l Ledger) JSON() []byte {
	if l.Signals == nil {
		l.Signals = []string{}
	}
	data, _ := json.MarshalIndent(l, "", "  ")
	return append(data, '\n')
}

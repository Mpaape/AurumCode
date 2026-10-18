package deliberation

// Merge joins the transcripts of the passes of one review (the batches of
// a diff that did not fit one prompt, the analysts of a selection) into one
// record: every tool some pass was offered, once, the tools some pass requested once each
// and the rest not requested, rounds and tokens summed, the calls in pass
// order, the last pass's outcome, the first limit and the first undecided
// reason kept, the reason no tool was offered only when no pass was
// offered one, and one Analysts entry per attributed pass. nil when no pass
// deliberated.
func Merge(transcripts ...*Transcript) *Transcript {
	var merged *Transcript
	for _, t := range transcripts {
		if t == nil {
			continue
		}
		if merged == nil {
			merged = &Transcript{Offered: []string{}}
		}
		merged.Offered = appendUnique(merged.Offered, t.Offered...)
		merged.Requested = appendUnique(merged.Requested, t.Requested...)
		merged.Rounds += t.Rounds
		merged.TokensIn += t.TokensIn
		merged.TokensOut += t.TokensOut
		merged.BaseTokens += t.BaseTokens
		merged.CostTokens += t.CostTokens
		merged.Calls = append(merged.Calls, t.Calls...)
		merged.Outcome = t.Outcome
		if merged.Limit == "" {
			merged.Limit = t.Limit
		}
		if merged.Undecided == "" {
			merged.Undecided = t.Undecided
		}
		if merged.NotOffered == "" {
			merged.NotOffered = t.NotOffered
		}
		if t.Analyst != "" {
			merged.Analysts = append(merged.Analysts, t.analystEntry())
		}
	}
	if merged != nil {
		if len(merged.Offered) > 0 {
			// Some pass was offered tools: the review was.
			merged.NotOffered = ""
		}
		merged.NotRequested = except(merged.Offered, merged.Requested)
	}
	return merged
}

// appendUnique appends the values not already in list.
func appendUnique(list []string, values ...string) []string {
	seen := make(map[string]bool, len(list))
	for _, v := range list {
		seen[v] = true
	}
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			list = append(list, v)
		}
	}
	return list
}

// except is the values of list absent from drop, never nil.
func except(list, drop []string) []string {
	skip := make(map[string]bool, len(drop))
	for _, v := range drop {
		skip[v] = true
	}
	out := []string{}
	for _, v := range list {
		if !skip[v] {
			out = append(out, v)
		}
	}
	return out
}

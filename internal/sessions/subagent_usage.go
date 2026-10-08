package sessions

import "strconv"

// SubagentTokenUsage preserves missing values: an absent counter is not zero.
// Cached input is a subset of input, and reasoning output a subset of output.
type SubagentTokenUsage struct {
	InputTokens           *int64 `json:"input_tokens"`
	CachedInputTokens     *int64 `json:"cached_input_tokens"`
	OutputTokens          *int64 `json:"output_tokens"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
	TotalTokens           *int64 `json:"total_tokens"`
}

type SubagentUsageRecord struct {
	ThreadID, ResponseID, Timestamp string
	Usage                           *SubagentTokenUsage
}

type SubagentTokenCount struct {
	Timestamp       string
	Baseline, Total *SubagentTokenUsage
}

// SubagentTerminalKey deduplicates identical terminal observations while keeping
// differing durations available for conflict detection.
func SubagentTerminalKey(t SubagentTerminal) string {
	duration := "missing"
	if t.DurationMS != nil {
		duration = strconv.FormatInt(*t.DurationMS, 10)
	}
	if t.InvalidDuration {
		duration = "invalid"
	}

	return t.Timestamp + "\x00" + t.Status + "\x00" + duration
}

// Validated validates the reported inclusive totals without adding their subsets.
func (u *SubagentTokenUsage) Validated() (SubagentTokenUsage, bool) {
	if u == nil || u.hasNegativeUsage() {
		return SubagentTokenUsage{}, false
	}
	v, ok := u.withInclusiveTotal()
	if !ok || !v.hasValidBounds() {
		return SubagentTokenUsage{}, false
	}

	return v, true
}

func (u *SubagentTokenUsage) hasNegativeUsage() bool {
	for _, field := range []*int64{u.InputTokens, u.CachedInputTokens, u.OutputTokens, u.ReasoningOutputTokens, u.TotalTokens} {
		if field != nil && *field < 0 {
			return true
		}
	}

	return false
}

func (u SubagentTokenUsage) withInclusiveTotal() (SubagentTokenUsage, bool) {
	if u.InputTokens == nil || u.OutputTokens == nil {
		return u, true
	}
	if *u.InputTokens > (1<<63-1)-*u.OutputTokens {
		return SubagentTokenUsage{}, false
	}
	total := *u.InputTokens + *u.OutputTokens
	if u.TotalTokens == nil {
		u.TotalTokens = &total
	} else if *u.TotalTokens != total {
		return SubagentTokenUsage{}, false
	}

	return u, true
}

func (u SubagentTokenUsage) hasValidBounds() bool {
	if u.TotalTokens == nil {
		return false
	}
	for _, field := range []*int64{u.InputTokens, u.OutputTokens, u.CachedInputTokens, u.ReasoningOutputTokens} {
		if field != nil && *field > *u.TotalTokens {
			return false
		}
	}

	return tokenSubsetWithin(u.CachedInputTokens, u.InputTokens) && tokenSubsetWithin(u.ReasoningOutputTokens, u.OutputTokens)
}

func tokenSubsetWithin(subset, total *int64) bool {
	return subset == nil || total == nil || *subset <= *total
}

// Delta returns a cumulative-counter difference, rejecting resets and preserving
// unavailable components rather than treating them as zero.
func (u *SubagentTokenUsage) Delta(baseline *SubagentTokenUsage) (SubagentTokenUsage, bool) {
	last, ok := u.Validated()
	if !ok {
		return SubagentTokenUsage{}, false
	}
	first, ok := baseline.Validated()
	if !ok {
		return SubagentTokenUsage{}, false
	}
	var delta SubagentTokenUsage
	pairs := [][2]*int64{
		{last.TotalTokens, first.TotalTokens},
		{last.InputTokens, first.InputTokens},
		{last.CachedInputTokens, first.CachedInputTokens},
		{last.OutputTokens, first.OutputTokens},
		{last.ReasoningOutputTokens, first.ReasoningOutputTokens},
	}
	values := make([]*int64, len(pairs))
	for i, pair := range pairs {
		if pair[0] == nil || pair[1] == nil {
			continue
		}
		if *pair[0] < *pair[1] {
			return SubagentTokenUsage{}, false
		}
		n := *pair[0] - *pair[1]
		values[i] = &n
	}
	delta.TotalTokens, delta.InputTokens, delta.CachedInputTokens, delta.OutputTokens, delta.ReasoningOutputTokens = values[0], values[1], values[2], values[3], values[4]

	return delta.Validated()
}

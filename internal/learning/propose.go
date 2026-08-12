package learning

import (
	"sort"
	"strings"
)

// ProposeHooks detects repeated manual sequences in the work log (LRN-02 / D-18)
// and emits a Proposal for each sequence that appears >=3 times. A "sequence"
// is 2-4 consecutive tool invocations (by Tool name). The detection is PURE —
// no I/O, no time, no globals — so it is fully table-testable.
//
// Overlapping sequences are deduped: at each start index, the longest matching
// repeated sequence wins. The returned Proposals are sorted by Occurrences desc
// then Steps asc (deterministic output — the operator-facing list is stable).
func ProposeHooks(worklog []WorklogEntry) []Proposal {
	// Extract the tool-name sequence (the discriminator — Args shape is not
	// part of the key in v1; a sequence is "the same N tools in a row").
	seq := make([]string, len(worklog))
	for i, w := range worklog {
		seq[i] = w.Tool
	}

	// Count occurrences of every distinct window of length minLen..maxLen.
	const minLen, maxLen = 2, 4

	type counted struct {
		steps []string
		n     int
	}

	counts := map[string]*counted{}

	var order []string // first-seen order for stable iteration

	for windowLen := minLen; windowLen <= maxLen; windowLen++ {
		for i := 0; i+windowLen <= len(seq); i++ {
			key := strings.Join(seq[i:i+windowLen], "|")
			if c, ok := counts[key]; ok {
				c.n++
			} else {
				cp := append([]string(nil), seq[i:i+windowLen]...)
				counts[key] = &counted{steps: cp, n: 1}
				order = append(order, key)
			}
		}
	}

	// Keep only sequences with >= 3 occurrences. Dedupe overlapping: if one
	// sequence's steps are a prefix/substring of a longer one with equal or
	// greater count, prefer the longer (richer hook).
	var proposals []Proposal

	for _, key := range order {
		c := counts[key]
		if c.n < 3 {
			continue
		}

		proposals = append(proposals, Proposal{
			HookName:    "learned-" + Slug(strings.Join(c.steps, "-")),
			Trigger:     "post-implement",
			Steps:       append([]string(nil), c.steps...),
			Occurrences: c.n,
		})
	}

	// Dedupe: drop a Proposal whose Steps are a contiguous sub-slice of another
	// Proposal's Steps when the longer has >= occurrences. (Keeps the richest.)
	proposals = dedupeSubsumed(proposals)

	// Deterministic sort: Occurrences desc, then Steps asc (joined for compare).
	sort.Slice(proposals, func(i, j int) bool {
		if proposals[i].Occurrences != proposals[j].Occurrences {
			return proposals[i].Occurrences > proposals[j].Occurrences
		}

		return strings.Join(proposals[i].Steps, "|") < strings.Join(proposals[j].Steps, "|")
	})

	return proposals
}

// dedupeSubsumed removes Proposals whose Steps are a contiguous sub-slice of a
// longer Proposal's Steps with equal-or-greater Occurrences. This prevents
// emitting both [Read, Grep] and [Read, Grep, Bash] when the longer is the
// richer hook.
func dedupeSubsumed(in []Proposal) []Proposal {
	out := make([]Proposal, 0, len(in))
	for i, sub := range in {
		dropped := false

		for j, other := range in {
			if i == j {
				continue
			}

			if len(other.Steps) <= len(sub.Steps) {
				continue
			}

			if other.Occurrences < sub.Occurrences {
				continue
			}

			if isContiguousSubSlice(sub.Steps, other.Steps) {
				dropped = true

				break
			}
		}

		if !dropped {
			out = append(out, sub)
		}
	}

	return out
}

// isContiguousSubSlice reports whether small appears as a contiguous sub-slice
// of big.
func isContiguousSubSlice(small, big []string) bool {
	if len(small) == 0 || len(small) > len(big) {
		return false
	}

	for i := 0; i+len(small) <= len(big); i++ {
		match := true

		for k := range small {
			if big[i+k] != small[k] {
				match = false

				break
			}
		}

		if match {
			return true
		}
	}

	return false
}

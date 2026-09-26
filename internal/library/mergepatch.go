package library

import (
	"bytes"
	"sort"

	"github.com/Official-Husko/parallax-mod-manager/internal/conflict"
	"github.com/Official-Husko/parallax-mod-manager/internal/definition"
	"github.com/Official-Husko/parallax-mod-manager/internal/script"
)

// MergeModOutcome is one mod's result from a single Tier 1 additive-merge
// pass - see docs/merge-patch.md's "mod-level atomicity". Only mods that had
// at least one candidate contribution this pass appear at all.
type MergeModOutcome struct {
	ModID   string
	ModName string
	// Applied is true when every one of this mod's proposed additions
	// succeeded and was written into the merge state; false means none of
	// them were - mod-level atomicity never applies a partial batch.
	Applied bool
	// EntriesAdded counts entries actually spliced in, meaningful only when
	// Applied.
	EntriesAdded int
	// FailedKey/FailedReason are set only when Applied is false: the first
	// Key that failed validation, and why - the rest of the batch was never
	// attempted once one failed, so naming the first is enough to explain
	// the whole skip.
	FailedKey    conflict.Key
	FailedReason string
}

// MergeReport is a whole Tier 1 pass's own final report - see
// docs/merge-patch.md.
type MergeReport struct {
	Mods           []MergeModOutcome // every mod with a candidate contribution, in load order
	EntriesApplied int
}

// mergeState is one merge-eligible Key's current accumulated content across
// a Tier 1 pass: starts as the automatic winner's own span text/block, and
// is updated in place each time a mod's batch is successfully applied, so
// the next mod processed is validated against what earlier ones already
// added, not just the original winner.
type mergeState struct {
	text  []byte
	block script.Block
}

// computeMerges runs Tier 1's mod-level-atomic additive-merge pass (see
// docs/merge-patch.md) over conflicts, for Types in conflict.MergeSafeTypes
// only, skipping any Key overrides has already manually resolved (a merge
// never second-guesses an explicit per-conflict choice). It returns, per
// Key that ended up with at least one successfully-applied addition, the
// final merged bytes to use in place of the plain winner's own span - and
// the pass's report.
//
// readModFile is the same per-mod file reader GeneratePatch already builds
// (cached, confined to each mod's own ContentPath).
func computeMerges(order conflict.LoadOrder, conflicts []conflict.Conflict, names map[string]string, overrides map[string]string, readModFile func(modID, relPath string) ([]byte, error)) (map[conflict.Key][]byte, MergeReport) {
	if len(conflict.MergeSafeTypes) == 0 {
		return nil, MergeReport{} // the common case today - nothing to do, cheaply
	}

	states := make(map[conflict.Key]*mergeState)
	byKey := make(map[conflict.Key]conflict.Conflict)

	for _, c := range conflicts {
		if !conflict.MergeSafeTypes[c.Key.Type] {
			continue
		}
		winner, manual := effectiveWinner(c, overrides)
		if manual {
			continue
		}
		var winnerDef *definition.Definition
		for i := range c.Candidates {
			if c.Candidates[i].ModID == winner {
				winnerDef = &c.Candidates[i]
				break
			}
		}
		if winnerDef == nil {
			continue
		}
		data, err := readModFile(winnerDef.ModID, winnerDef.FilePath)
		if err != nil {
			continue
		}
		block, ok := definitionBlock(data, winnerDef.Span)
		if !ok {
			continue
		}
		text := append([]byte(nil), data[winnerDef.Span.StartOffset:winnerDef.Span.EndOffset]...)
		states[c.Key] = &mergeState{text: text, block: block}
		byKey[c.Key] = c
	}

	if len(byKey) == 0 {
		return nil, MergeReport{}
	}

	// Every mod that's a non-winning candidate on at least one eligible
	// Key, in ascending load-order position - the processing order the
	// mod-level-atomicity design relies on (see docs/merge-patch.md).
	touchedBy := make(map[string]bool)
	var modOrder []string
	for _, c := range byKey {
		winner, _ := effectiveWinner(c, overrides)
		for _, cand := range c.Candidates {
			if cand.ModID == winner || touchedBy[cand.ModID] {
				continue
			}
			touchedBy[cand.ModID] = true
			modOrder = append(modOrder, cand.ModID)
		}
	}
	orderPos := make(map[string]int, len(order))
	for i, id := range order {
		orderPos[id] = i
	}
	sort.Slice(modOrder, func(i, j int) bool { return orderPos[modOrder[i]] < orderPos[modOrder[j]] })

	report := MergeReport{}
	touched := make(map[conflict.Key]bool)
	for _, modID := range modOrder {
		outcome := applyModBatch(modID, names[modID], byKey, states, overrides, readModFile, touched)
		if outcome == nil {
			continue
		}
		report.Mods = append(report.Mods, *outcome)
		if outcome.Applied {
			report.EntriesApplied += outcome.EntriesAdded
		}
	}

	if len(touched) == 0 {
		return nil, report
	}
	merged := make(map[conflict.Key][]byte, len(touched))
	for k := range touched {
		merged[k] = states[k].text
	}
	return merged, report
}

// applyModBatch gathers modID's proposed additions across every eligible Key
// it's a non-winning candidate for, validates the whole batch together
// against the *current* accumulated state (which already includes any
// earlier mod's own successfully-applied additions this same pass), and
// either commits all of them into states or none - see docs/merge-patch.md's
// mod-level atomicity. Returns nil if modID had no candidacy on any eligible
// Key this pass at all, or had candidacy but nothing new to add anywhere
// (neither is a failure - there's simply nothing to report).
func applyModBatch(modID, modName string, byKey map[conflict.Key]conflict.Conflict, states map[conflict.Key]*mergeState, overrides map[string]string, readModFile func(string, string) ([]byte, error), touched map[conflict.Key]bool) *MergeModOutcome {
	type proposal struct {
		key       conflict.Key
		added     []script.Entry
		candData  []byte
		candStart int
	}
	var proposals []proposal
	attempted := false

	for key, c := range byKey {
		winner, _ := effectiveWinner(c, overrides)
		if winner == modID {
			continue
		}
		var candDef *definition.Definition
		for i := range c.Candidates {
			if c.Candidates[i].ModID == modID {
				candDef = &c.Candidates[i]
				break
			}
		}
		if candDef == nil {
			continue // this mod isn't even a candidate for this Key
		}
		attempted = true

		data, err := readModFile(candDef.ModID, candDef.FilePath)
		if err != nil {
			return &MergeModOutcome{ModID: modID, ModName: modName, FailedKey: key, FailedReason: "could not read source file"}
		}
		candBlock, ok := definitionBlock(data, candDef.Span)
		if !ok {
			return &MergeModOutcome{ModID: modID, ModName: modName, FailedKey: key, FailedReason: "source content changed since it was scanned"}
		}
		added, ok := conflict.AdditiveEntries(states[key].block, candBlock, conflict.RepeatableMergeKeys[key.Type])
		if !ok {
			return &MergeModOutcome{ModID: modID, ModName: modName, FailedKey: key, FailedReason: "disagrees with the current winner on an existing entry"}
		}
		if len(added) == 0 {
			continue // nothing new from this mod at this Key
		}
		proposals = append(proposals, proposal{key: key, added: added, candData: data, candStart: candDef.Span.StartOffset})
	}

	if !attempted || len(proposals) == 0 {
		return nil
	}

	// Validate the whole batch before committing anything: splice and
	// re-parse every proposed Key against a scratch copy of its state - a
	// mid-batch failure must leave every earlier Key in this same batch
	// untouched too, matching all-or-nothing.
	assembled := make(map[conflict.Key]mergeState, len(proposals))
	totalAdded := 0
	for _, p := range proposals {
		cur := states[p.key]
		newText, newBlock, ok := spliceEntries(cur.text, cur.block, p.added, p.candData, p.candStart)
		if !ok {
			return &MergeModOutcome{ModID: modID, ModName: modName, FailedKey: p.key, FailedReason: "failed to re-parse after splicing"}
		}
		assembled[p.key] = mergeState{text: newText, block: newBlock}
		totalAdded += len(p.added)
	}

	for key, st := range assembled {
		states[key].text = st.text
		states[key].block = st.block
		touched[key] = true
	}
	return &MergeModOutcome{ModID: modID, ModName: modName, Applied: true, EntriesAdded: totalAdded}
}

// definitionBlock re-parses one Definition's own source bytes (sliced from
// data via its Span) and returns its Value's Block. ok is false if the
// value isn't a block (a bare scalar definition has nothing to merge inside
// it), the Span is stale - the same "file changed since this Span was
// computed" guard GeneratePatch's own copy loop already applies - or the
// slice fails to parse.
func definitionBlock(data []byte, span definition.Span) (script.Block, bool) {
	if span.EndOffset <= span.StartOffset || span.EndOffset > len(data) {
		return script.Block{}, false
	}
	f, err := script.Parse(data[span.StartOffset:span.EndOffset])
	if err != nil || len(f.Root.Entries) != 1 {
		return script.Block{}, false
	}
	v := f.Root.Entries[0].Value
	if v.Kind != script.KindBlock || v.Block == nil {
		return script.Block{}, false
	}
	return *v.Block, true
}

// spliceEntries appends added's own raw source bytes (sliced from candData
// using each entry's Offset/EndOffset, which are relative to a re-parse of
// the same candidate span candData came from - see definitionBlock, and
// candSpanStart is that span's own StartOffset) onto text, just before
// existing's own last entry ends - i.e. inside the block, before its
// closing brace and any trailing whitespace or comments - and re-parses the
// result to confirm it's still valid: exactly one top-level entry, whose
// block now has exactly len(existing.Entries) + len(added) entries. Returns
// ok=false, discarding the whole attempt, if anything about that isn't true
// - the round-trip safety net docs/merge-patch.md calls for.
//
// An existing block with no entries at all has no anchor to insert before
// and is deliberately refused rather than guessing where its closing brace
// is - see docs/merge-patch.md.
func spliceEntries(text []byte, existing script.Block, added []script.Entry, candData []byte, candSpanStart int) (newText []byte, newBlock script.Block, ok bool) {
	if len(existing.Entries) == 0 {
		return nil, script.Block{}, false
	}
	insertAt := existing.Entries[len(existing.Entries)-1].EndOffset
	if insertAt < 0 || insertAt > len(text) {
		return nil, script.Block{}, false
	}

	var buf bytes.Buffer
	buf.Write(text[:insertAt])
	for _, e := range added {
		start, end := candSpanStart+e.Offset, candSpanStart+e.EndOffset
		if start < 0 || end > len(candData) || start > end {
			return nil, script.Block{}, false
		}
		buf.WriteByte('\n')
		buf.Write(candData[start:end])
	}
	buf.Write(text[insertAt:])
	newText = buf.Bytes()

	f, err := script.Parse(newText)
	if err != nil || len(f.Root.Entries) != 1 {
		return nil, script.Block{}, false
	}
	v := f.Root.Entries[0].Value
	if v.Kind != script.KindBlock || v.Block == nil || len(v.Block.Entries) != len(existing.Entries)+len(added) {
		return nil, script.Block{}, false
	}
	return newText, *v.Block, true
}

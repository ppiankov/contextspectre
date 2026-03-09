package analyzer

import (
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func makeEntries(n int, rawSize int) []jsonl.Entry {
	entries := make([]jsonl.Entry, n)
	for i := range entries {
		entries[i].RawSize = rawSize
	}
	return entries
}

func TestBuildNoiseLedger_OverlapDetected(t *testing.T) {
	// Entry 2 is in both stale_reads and tangents — should count once in union.
	entries := makeEntries(6, 400) // 400 / 4 = 100 tokens each

	dupResult := &DuplicateReadResult{
		Groups: []DuplicateGroup{
			{
				StaleReads: []StaleRead{
					{AssistantIdx: 2, ResultIdx: 3},
				},
			},
		},
	}

	tangentResult := &TangentResult{
		Groups: []TangentGroup{
			{EntryIndices: []int{2, 4}},
		},
	}

	ledger := BuildNoiseLedger(entries, dupResult, nil, tangentResult, nil)

	// Per-category: stale_reads = 200 (entry 2 + 3), tangents = 200 (entry 2 + 4)
	// Union: entries 2, 3, 4 = 300
	// Overlap: 400 - 300 = 100
	if ledger.UnionTokens != 300 {
		t.Errorf("UnionTokens = %d, want 300", ledger.UnionTokens)
	}
	if ledger.OverlapTokens != 100 {
		t.Errorf("OverlapTokens = %d, want 100", ledger.OverlapTokens)
	}
	if ledger.PerCategory["stale_reads"] != 200 {
		t.Errorf("PerCategory[stale_reads] = %d, want 200", ledger.PerCategory["stale_reads"])
	}
	if ledger.PerCategory["tangents"] != 200 {
		t.Errorf("PerCategory[tangents] = %d, want 200", ledger.PerCategory["tangents"])
	}
}

func TestBuildNoiseLedger_NoOverlap(t *testing.T) {
	entries := makeEntries(8, 400)

	dupResult := &DuplicateReadResult{
		Groups: []DuplicateGroup{
			{StaleReads: []StaleRead{{AssistantIdx: 0, ResultIdx: 1}}},
		},
	}
	retryResult := &RetryResult{
		Sequences: []RetrySequence{{FailedToolUseIdx: 2, FailedResultIdx: 3}},
	}
	tangentResult := &TangentResult{
		Groups: []TangentGroup{{EntryIndices: []int{4, 5}}},
	}
	sidechainReport := &SidechainReport{
		Entries: []SidechainEntry{{EntryIndex: 6}, {EntryIndex: 7}},
	}

	ledger := BuildNoiseLedger(entries, dupResult, retryResult, tangentResult, sidechainReport)

	// 8 distinct entries × 100 = 800
	if ledger.UnionTokens != 800 {
		t.Errorf("UnionTokens = %d, want 800", ledger.UnionTokens)
	}
	if ledger.OverlapTokens != 0 {
		t.Errorf("OverlapTokens = %d, want 0", ledger.OverlapTokens)
	}
}

func TestBuildNoiseLedger_NilInputs(t *testing.T) {
	entries := makeEntries(4, 400)
	ledger := BuildNoiseLedger(entries, nil, nil, nil, nil)
	if ledger.UnionTokens != 0 {
		t.Errorf("UnionTokens = %d, want 0", ledger.UnionTokens)
	}
	if ledger.OverlapTokens != 0 {
		t.Errorf("OverlapTokens = %d, want 0", ledger.OverlapTokens)
	}
}

func TestRecommend_UnionTotal(t *testing.T) {
	// Entry 1 in both stale_reads and failed_retries.
	entries := makeEntries(4, 400)
	stats := &ContextStats{
		CurrentContextTokens: 10000,
		UsagePercent:         50,
	}
	dupResult := &DuplicateReadResult{
		TotalStale:  1,
		TotalTokens: 100,
		Groups: []DuplicateGroup{
			{StaleReads: []StaleRead{{AssistantIdx: 1, ResultIdx: -1}}},
		},
	}
	retryResult := &RetryResult{
		TotalFailed: 1,
		TotalTokens: 100,
		Sequences:   []RetrySequence{{FailedToolUseIdx: 1, FailedResultIdx: -1}},
	}

	rec := Recommend(entries, stats, dupResult, retryResult, nil, nil)
	if rec == nil {
		t.Fatal("expected non-nil recommendation")
	}

	// Per-category sum = 200 (100 stale + 100 retry)
	// Union = 100 (entry 1 counted once)
	// TotalTokens should be union (100), not sum (200)
	if rec.TotalTokens != 100 {
		t.Errorf("TotalTokens = %d, want 100 (union)", rec.TotalTokens)
	}
	if rec.OverlapTokens != 100 {
		t.Errorf("OverlapTokens = %d, want 100", rec.OverlapTokens)
	}
}

package analyzer

import (
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// FlightEvent represents a single reasoning event in the session timeline.
type FlightEvent struct {
	Turn   int    `json:"turn"`
	Type   string `json:"type"`   // compaction, decision, cleanup, vector_drift, file_burst
	Detail string `json:"detail"` // human-readable description
	Tokens int    `json:"tokens"` // context tokens at this point
	Epoch  int    `json:"epoch"`  // which epoch this event belongs to
}

// FlightEpoch describes a reasoning epoch in the session.
type FlightEpoch struct {
	Index  int    `json:"index"`
	Label  string `json:"label"` // exploration, implementation, refinement
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Turns  int    `json:"turns"`
	Tokens int    `json:"tokens"` // tokens used in this epoch
	Files  int    `json:"files"`  // unique files touched
}

// FlightRecord holds the complete reasoning flight recorder for a session.
type FlightRecord struct {
	TotalTurns  int           `json:"total_turns"`
	TotalTokens int           `json:"total_tokens"`
	Epochs      []FlightEpoch `json:"epochs"`
	Events      []FlightEvent `json:"events"`
	HalfLife    int           `json:"half_life"`
	SignalGrade string        `json:"signal_grade"`
}

// ComputeFlightRecord extracts a structured reasoning timeline from session data.
func ComputeFlightRecord(entries []jsonl.Entry, stats *ContextStats) *FlightRecord {
	turns := countAssistantTurns(entries)
	if turns < 1 {
		return &FlightRecord{
			Epochs: []FlightEpoch{},
			Events: []FlightEvent{},
		}
	}

	fr := &FlightRecord{
		TotalTurns: turns,
	}

	// Build epoch info from compaction boundaries.
	epochBounds := buildEpochBounds(entries, stats.Compactions)
	epochFiles := collectEpochFiles(entries, stats.Compactions, len(epochBounds))
	epochTokens := collectEpochTokens(entries, stats.Compactions, len(epochBounds))

	for i, eb := range epochBounds {
		label := classifyEpoch(i, len(epochBounds), eb)
		ep := FlightEpoch{
			Index:  i,
			Label:  label,
			Start:  eb.startTurn,
			End:    eb.endTurn,
			Turns:  eb.endTurn - eb.startTurn,
			Tokens: epochTokens[i],
			Files:  len(epochFiles[i]),
		}
		fr.TotalTokens += epochTokens[i]
		fr.Epochs = append(fr.Epochs, ep)
	}

	// Extract events.
	fr.Events = extractFlightEvents(entries, stats)

	// Add half-life and signal grade.
	hl := ComputeHalfLife(entries, stats.Compactions)
	fr.HalfLife = hl.HalfLife

	grade := GradeFromSignalPercent(signalPercentFromStats(stats))
	fr.SignalGrade = string(grade)

	return fr
}

// collectEpochFiles gathers unique file paths touched per epoch.
func collectEpochFiles(entries []jsonl.Entry, compactions []CompactionEvent, numEpochs int) []map[string]bool {
	files := make([]map[string]bool, numEpochs)
	for i := range files {
		files[i] = make(map[string]bool)
	}

	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		epoch := epochIndexForEntry(i, compactions)
		if epoch >= numEpochs {
			epoch = numEpochs - 1
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" {
				path := ExtractToolInputPath(b.Input)
				if path != "" {
					files[epoch][path] = true
				}
			}
		}
	}
	return files
}

// collectEpochTokens sums output tokens per epoch.
func collectEpochTokens(entries []jsonl.Entry, compactions []CompactionEvent, numEpochs int) []int {
	tokens := make([]int, numEpochs)
	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		epoch := epochIndexForEntry(i, compactions)
		if epoch >= numEpochs {
			epoch = numEpochs - 1
		}
		t := 0
		if e.Message.Usage != nil {
			t = e.Message.Usage.OutputTokens
		}
		if t == 0 {
			t = e.RawSize / 4
		}
		tokens[epoch] += t
	}
	return tokens
}

// extractFlightEvents builds a timeline of reasoning events.
func extractFlightEvents(entries []jsonl.Entry, stats *ContextStats) []FlightEvent {
	var events []FlightEvent
	assistantTurn := 0
	numEpochs := len(stats.Compactions) + 1

	// Track context token progression for drift detection.
	var prevTokens int
	var burstFiles int
	var prevFiles map[string]bool

	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		assistantTurn++

		epoch := epochIndexForEntry(i, stats.Compactions)
		if epoch >= numEpochs {
			epoch = numEpochs - 1
		}

		tokens := 0
		if e.Message.Usage != nil {
			tokens = e.Message.Usage.TotalContextTokens()
		}

		// Compaction event: large drop in context tokens.
		if prevTokens > 0 && prevTokens-tokens > CompactionDropThreshold {
			events = append(events, FlightEvent{
				Turn:   assistantTurn,
				Type:   "compaction",
				Detail: formatCompactionDetail(prevTokens, tokens),
				Tokens: tokens,
				Epoch:  epoch,
			})
		}

		// File burst: many new files in a single turn.
		currentFiles := extractTurnFiles(e)
		newFileCount := 0
		if prevFiles != nil {
			for f := range currentFiles {
				if !prevFiles[f] {
					newFileCount++
				}
			}
		}
		if newFileCount >= 5 {
			burstFiles += newFileCount
			events = append(events, FlightEvent{
				Turn:   assistantTurn,
				Type:   "file_burst",
				Detail: formatFileBurstDetail(newFileCount),
				Tokens: tokens,
				Epoch:  epoch,
			})
		}
		_ = burstFiles

		// Context growth spike: large jump in tokens.
		if prevTokens > 0 && tokens-prevTokens > 30000 {
			events = append(events, FlightEvent{
				Turn:   assistantTurn,
				Type:   "context_spike",
				Detail: formatContextSpikeDetail(prevTokens, tokens),
				Tokens: tokens,
				Epoch:  epoch,
			})
		}

		if tokens > 0 {
			prevTokens = tokens
		}
		if len(currentFiles) > 0 {
			if prevFiles == nil {
				prevFiles = make(map[string]bool)
			}
			for f := range currentFiles {
				prevFiles[f] = true
			}
		}
	}

	// Add commit point events from archaeology.
	if stats.Archaeology != nil {
		for ci, arch := range stats.Archaeology.Events {
			if arch.Before.DecisionCount > 0 {
				turn := estimateTurnFromLine(arch.LineIndex, entries)
				events = append(events, FlightEvent{
					Turn:   turn,
					Type:   "decision",
					Detail: formatDecisionDetail(arch.Before),
					Tokens: arch.Before.TokensPeak,
					Epoch:  ci,
				})
			}
		}
	}

	// Sort events by turn.
	sortFlightEvents(events)

	return events
}

// classifyEpoch labels an epoch based on its position in the session.
func classifyEpoch(index, total int, eb epochBound) string {
	if total == 1 {
		return "single"
	}
	turns := eb.endTurn - eb.startTurn
	position := float64(index) / float64(total-1)
	switch {
	case index == 0:
		return "exploration"
	case index == total-1:
		if turns < 20 {
			return "refinement"
		}
		return "implementation"
	case position < 0.4:
		return "architecture"
	default:
		return "implementation"
	}
}

// extractTurnFiles gets files touched in a single assistant turn.
func extractTurnFiles(e jsonl.Entry) map[string]bool {
	files := make(map[string]bool)
	if e.Message == nil {
		return files
	}
	blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
	if err != nil {
		return files
	}
	for _, b := range blocks {
		if b.Type == "tool_use" {
			path := ExtractToolInputPath(b.Input)
			if path != "" {
				files[path] = true
			}
		}
	}
	return files
}

// estimateTurnFromLine maps a line index to an approximate assistant turn number.
func estimateTurnFromLine(lineIdx int, entries []jsonl.Entry) int {
	turn := 0
	for i, e := range entries {
		if e.Type == jsonl.TypeAssistant {
			turn++
		}
		if i >= lineIdx {
			break
		}
	}
	return turn
}

// signalPercentFromStats extracts signal percent from context stats.
func signalPercentFromStats(stats *ContextStats) int {
	if stats.CurrentContextTokens <= 0 {
		return 100
	}
	noiseTokens := stats.ProgressTokens + stats.SidechainTokens + stats.TangentTokens
	totalTokens := stats.CurrentContextTokens
	if totalTokens > 0 {
		signal := totalTokens - noiseTokens
		if signal < 0 {
			signal = 0
		}
		return signal * 100 / totalTokens
	}
	return 100
}

func formatCompactionDetail(before, after int) string {
	drop := before - after
	return formatTokenCountInternal(drop) + " tokens compacted (" + formatTokenCountInternal(before) + " → " + formatTokenCountInternal(after) + ")"
}

func formatFileBurstDetail(count int) string {
	return formatIntStr(count) + " new files in one turn"
}

func formatContextSpikeDetail(before, after int) string {
	growth := after - before
	return "+" + formatTokenCountInternal(growth) + " tokens in one turn"
}

func formatDecisionDetail(epoch EpochSummary) string {
	if len(epoch.DecisionHints) > 0 {
		hint := epoch.DecisionHints[0]
		if len(hint) > 60 {
			hint = hint[:57] + "..."
		}
		return hint
	}
	return formatIntStr(epoch.DecisionCount) + " decisions"
}

func formatTokenCountInternal(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return formatFloat(float64(tokens)/1_000_000) + "M"
	case tokens >= 1_000:
		return formatFloat(float64(tokens)/1_000) + "K"
	default:
		return formatIntStr(tokens)
	}
}

func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return formatIntStr(int(f))
	}
	// Simple 1-decimal formatting without fmt.
	whole := int(f)
	frac := int((f - float64(whole)) * 10)
	if frac < 0 {
		frac = -frac
	}
	return formatIntStr(whole) + "." + formatIntStr(frac)
}

func formatIntStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func sortFlightEvents(events []FlightEvent) {
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			if events[j].Turn < events[i].Turn {
				events[i], events[j] = events[j], events[i]
			}
		}
	}
}

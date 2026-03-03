package analyzer

import "fmt"

// Epoch is a unified view of a compaction epoch, merging cost and archaeology data.
type Epoch struct {
	Index         int
	TurnCount     int
	PeakTokens    int
	Cost          float64
	Topic         string
	SurvivedChars int                    // -1 for active epoch
	IsActive      bool                   // true for last epoch
	Archaeology   *CompactionArchaeology // nil for active epoch
}

// BuildEpochs merges EpochCost and CompactionArchaeology into a unified epoch timeline.
// activeTopicHint is used as the topic for the active (last) epoch when no archaeology exists.
func BuildEpochs(epochCosts []EpochCost, archaeology *CompactionReport, activeTopicHint string) []Epoch {
	if len(epochCosts) == 0 {
		return nil
	}

	epochs := make([]Epoch, len(epochCosts))
	for i, ec := range epochCosts {
		epochs[i] = Epoch{
			Index:      ec.EpochIndex,
			TurnCount:  ec.TurnCount,
			PeakTokens: ec.PeakTokens,
			Cost:       ec.Cost.TotalCost,
		}

		isLast := i == len(epochCosts)-1

		// Link archaeology for completed epochs
		if archaeology != nil && i < len(archaeology.Events) {
			arch := &archaeology.Events[i]
			epochs[i].Archaeology = arch
			epochs[i].SurvivedChars = arch.After.SummaryCharCount
			epochs[i].Topic = extractTopic(i, arch)
		} else if isLast {
			epochs[i].IsActive = true
			epochs[i].SurvivedChars = -1
			if activeTopicHint != "" {
				epochs[i].Topic = TruncateHint(activeTopicHint, 30)
			} else {
				epochs[i].Topic = fmt.Sprintf("Epoch %d (active)", i)
			}
		} else {
			epochs[i].Topic = fmt.Sprintf("Epoch %d", i)
			epochs[i].SurvivedChars = 0
		}
	}

	return epochs
}

// extractTopic extracts a short topic label from epoch archaeology data.
func extractTopic(index int, arch *CompactionArchaeology) string {
	if arch == nil {
		return fmt.Sprintf("Epoch %d", index)
	}
	if len(arch.Before.UserQuestions) > 0 {
		return TruncateHint(arch.Before.UserQuestions[0], 30)
	}
	if arch.After.SummaryText != "" {
		return TruncateHint(arch.After.SummaryText, 30)
	}
	return fmt.Sprintf("Epoch %d", index)
}

package skilldb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/insightsdb"
)

// PopulateFromInsightsDB populates the skill analytics DB from insightsdb data.
// It queries skill_signals and tool_calls to find sessions that used discovered skills.
func (sdb *SkillDB) PopulateFromInsightsDB(ctx context.Context, idb *insightsdb.InsightsDB, discoveredSkills []SkillRow) error {
	if len(discoveredSkills) == 0 {
		return nil
	}

	// Collect skill names for querying.
	skillNames := make([]string, len(discoveredSkills))
	skillMap := make(map[string]SkillRow, len(discoveredSkills))
	for i, s := range discoveredSkills {
		skillNames[i] = s.Name
		skillMap[s.Name] = s
	}

	tx, err := sdb.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback() //nolint:errcheck // Rollback after failed tx; error is irrelevant
		}
	}()

	// Track which sessions we've already inserted to avoid duplicates.
	type sessionKey struct {
		skillName    string
		checkpointID string
		sessionIndex int
	}
	inserted := make(map[sessionKey]bool)

	// Step 1: From skill_signals — sessions with friction for specific skills.
	signals, err := idb.QuerySkillSignalsForSkills(ctx, skillNames)
	if err != nil {
		return fmt.Errorf("query skill signals: %w", err)
	}

	for _, sig := range signals {
		skill, ok := skillMap[sig.SkillName]
		if !ok {
			continue
		}

		key := sessionKey{sig.SkillName, sig.CheckpointID, sig.SessionIndex}
		if inserted[key] {
			continue
		}
		inserted[key] = true

		frictionCount := len(sig.Friction)
		outcome := "success"
		if frictionCount > 0 {
			outcome = "friction"
		}

		if err = sdb.InsertSessionTx(ctx, tx, SkillSessionRow{
			SkillName:     sig.SkillName,
			SourceAgent:   skill.SourceAgent,
			CheckpointID:  sig.CheckpointID,
			SessionIndex:  sig.SessionIndex,
			SessionID:     sig.SessionID,
			Agent:         sig.Agent,
			Model:         sig.Model,
			Branch:        sig.Branch,
			CreatedAt:     sig.CreatedAt,
			TotalTokens:   sig.TotalTokens,
			TurnCount:     sig.TurnCount,
			OverallScore:  sig.OverallScore,
			FrictionCount: frictionCount,
			Outcome:       outcome,
		}); err != nil {
			return fmt.Errorf("insert skill session: %w", err)
		}

		// Insert friction items.
		for _, f := range sig.Friction {
			if err = sdb.InsertFrictionTx(ctx, tx,
				sig.SkillName, skill.SourceAgent,
				sig.CheckpointID, sig.SessionIndex,
				f, "",
			); err != nil {
				return fmt.Errorf("insert skill friction: %w", err)
			}
		}

		// Insert missing instruction if present.
		if sig.MissingInstruction != "" {
			evidence := strings.Join(sig.Friction, "\n")
			if err = sdb.InsertMissingInstructionTx(ctx, tx,
				sig.SkillName, skill.SourceAgent,
				sig.CheckpointID, sig.SessionIndex,
				sig.MissingInstruction, evidence,
			); err != nil {
				return fmt.Errorf("insert missing instruction: %w", err)
			}
		}
	}

	// Step 2: From tool_calls — sessions that used the Skill tool (friction-free uses).
	toolSessions, err := idb.QuerySkillToolCallSessions(ctx)
	if err != nil {
		return fmt.Errorf("query skill tool call sessions: %w", err)
	}

	for _, ts := range toolSessions {
		// We can't determine which specific skill was used from tool_calls alone,
		// so we skip sessions already covered by skill_signals.
		// These sessions indicate the Skill tool was invoked but we only record
		// them if there's exactly one discovered skill (unambiguous attribution).
		if len(discoveredSkills) == 1 {
			skill := discoveredSkills[0]
			key := sessionKey{skill.Name, ts.CheckpointID, ts.SessionIndex}
			if inserted[key] {
				continue
			}
			inserted[key] = true

			if err = sdb.InsertSessionTx(ctx, tx, SkillSessionRow{
				SkillName:    skill.Name,
				SourceAgent:  skill.SourceAgent,
				CheckpointID: ts.CheckpointID,
				SessionIndex: ts.SessionIndex,
				SessionID:    ts.SessionID,
				Agent:        ts.Agent,
				Model:        ts.Model,
				Branch:       ts.Branch,
				CreatedAt:    ts.CreatedAt,
				TotalTokens:  ts.TotalTokens,
				TurnCount:    ts.TurnCount,
				OverallScore: ts.OverallScore,
				Outcome:      "success",
			}); err != nil {
				return fmt.Errorf("insert tool call session: %w", err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// RefreshFromInsightsDB checks if the cache is stale and repopulates if needed.
// Returns true if the cache was refreshed.
func (sdb *SkillDB) RefreshFromInsightsDB(ctx context.Context, idb *insightsdb.InsightsDB, discoveredSkills []SkillRow) (bool, error) {
	// Check insightsdb branch tip.
	currentTip, err := idb.GetBranchTip(ctx)
	if err != nil {
		return false, fmt.Errorf("get insightsdb branch tip: %w", err)
	}

	cachedTip, err := sdb.GetCacheTip(ctx)
	if err != nil {
		return false, fmt.Errorf("get skilldb cache tip: %w", err)
	}

	if currentTip != "" && currentTip == cachedTip {
		return false, nil
	}

	// Upsert discovered skills.
	now := time.Now().UTC()
	for _, skill := range discoveredSkills {
		if err = sdb.UpsertSkill(ctx, SkillRow{
			Name:         skill.Name,
			SourceAgent:  skill.SourceAgent,
			Path:         skill.Path,
			Kind:         skill.Kind,
			DiscoveredAt: now,
			LastSeenAt:   now,
		}); err != nil {
			return false, fmt.Errorf("upsert skill %q: %w", skill.Name, err)
		}
	}

	// Clear existing session data before repopulating.
	if err = sdb.clearSessionData(ctx); err != nil {
		return false, fmt.Errorf("clear session data: %w", err)
	}

	// Populate from insightsdb.
	if err = sdb.PopulateFromInsightsDB(ctx, idb, discoveredSkills); err != nil {
		return false, fmt.Errorf("populate from insightsdb: %w", err)
	}

	// Update cache tip.
	if currentTip != "" {
		if err = sdb.SetCacheTip(ctx, currentTip); err != nil {
			return false, fmt.Errorf("set cache tip: %w", err)
		}
	}

	return true, nil
}

// clearSessionData removes all rows from session-related tables.
func (sdb *SkillDB) clearSessionData(ctx context.Context) error {
	tables := []string{"skill_sessions", "skill_friction", "skill_missing_instructions"}
	for _, table := range tables {
		if _, err := sdb.db.ExecContext(ctx, "DELETE FROM "+table); err != nil { //nolint:gosec // table names are hardcoded
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	return nil
}

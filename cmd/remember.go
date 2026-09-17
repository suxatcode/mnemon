package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mnemon-dev/mnemon/internal/daemonemit"
	"github.com/mnemon-dev/mnemon/internal/graph"
	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/spf13/cobra"
)

var (
	remCategory   string
	remImportance int
	remTags       string
	remSource     string
	remEntities   string
	remEntityMode string
	remNoDiff     bool
)

var rememberCmd = &cobra.Command{
	Use:   "remember [content]",
	Short: "Store a new insight",
	Long:  "Store a new insight into the memory graph with optional category, importance, and tags.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		content := strings.Join(args, " ")
		if len(content) > 8000 {
			return fmt.Errorf("content too long (%d chars, max 8000); consider chunking into multiple remember calls", len(content))
		}
		cat := model.Category(remCategory)
		if !model.ValidCategories[cat] {
			return fmt.Errorf("invalid category %q; valid: preference, decision, fact, insight, context, general", remCategory)
		}
		if remImportance < 1 || remImportance > 5 {
			return fmt.Errorf("importance must be 1-5, got %d", remImportance)
		}
		entityMode := graph.EntityMode(remEntityMode)
		if !graph.ValidEntityMode(entityMode) {
			return fmt.Errorf("invalid entity mode %q; valid: merge, provided, auto", remEntityMode)
		}
		input := memorysvc.RememberInput{
			Content: content, Category: remCategory, Importance: remImportance,
			Tags: remTags, Source: remSource, Entities: remEntities,
			EntityMode: remEntityMode, NoDiff: remNoDiff,
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Remember(remoteapi.RememberRequest{
				Content: input.Content, Category: input.Category, Importance: input.Importance,
				Tags: input.Tags, Source: input.Source, Entities: input.Entities,
				EntityMode: input.EntityMode, NoDiff: input.NoDiff,
			})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			res, err := svc.Remember(actor, input)
			if err != nil {
				return err
			}
			if err := writeResult(res, nil); err != nil {
				return err
			}
			var payload struct {
				ID     string `json:"id"`
				Action string `json:"action"`
			}
			_ = json.Unmarshal(res.JSON, &payload)
			if payload.ID != "" {
				emitRememberEvent(&model.Insight{
					ID:         payload.ID,
					Content:    content,
					Category:   cat,
					Importance: remImportance,
				}, payload.Action)
			}
			return nil
		})
	},
}

func init() {
	rememberCmd.Flags().StringVar(&remCategory, "cat", "general", "category (preference|decision|fact|insight|context|general)")
	rememberCmd.Flags().IntVar(&remImportance, "imp", 3, "importance (1-5)")
	rememberCmd.Flags().StringVar(&remTags, "tags", "", "comma-separated tags")
	rememberCmd.Flags().StringVar(&remSource, "source", "user", "source (user|agent|external)")
	rememberCmd.Flags().StringVar(&remEntities, "entities", "", "comma-separated entities (LLM-extracted, merged with auto-extraction)")
	rememberCmd.Flags().StringVar(&remEntityMode, "entity-mode", string(graph.EntityModeMerge), "entity handling mode (merge|provided|auto)")
	rememberCmd.Flags().BoolVar(&remNoDiff, "no-diff", false, "skip duplicate/conflict detection")
	rootCmd.AddCommand(rememberCmd)
}

func emitRememberEvent(insight *model.Insight, action string) {
	if os.Getenv("MNEMON_HARNESS_EVENT_EMIT") != "1" {
		return
	}
	_, _, _ = daemonemit.Emit(daemonemit.Options{
		Root:          ".",
		Topic:         "memory.hot_write_observed",
		CorrelationID: "memory:" + insight.ID,
		Loop:          "memory",
		Host:          "mnemon",
		Actor:         "mnemon-manual",
		Source:        "mnemon.remember",
		Store:         resolveStoreName(),
		Payload: map[string]any{
			"insight_id": insight.ID,
			"category":   string(insight.Category),
			"importance": insight.Importance,
			"action":     action,
		},
	})
}

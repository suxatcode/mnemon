package cmd

import (
	"time"

	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/mnemon-dev/mnemon/internal/store"
	"github.com/spf13/cobra"
)

var receiptLimit int

var receiptCmd = &cobra.Command{
	Use:   "receipt",
	Short: "Export a privacy-safe memory operation receipt",
	Long: `Export a JSON receipt for recent memory operations without printing raw memory
contents, queries, paths, or operation details. The receipt hashes identifiers and
details so it can be shared for audits of what crossed the memory boundary.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requirePositiveLimit("--limit", receiptLimit); err != nil {
			return err
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Receipt(remoteapi.ReceiptRequest{Limit: receiptLimit})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			return writeResult(svc.Receipt(actor, memorysvc.ReceiptInput{Limit: receiptLimit}))
		})
	},
}

func init() {
	receiptCmd.Flags().IntVar(&receiptLimit, "limit", 20, "max operations to include")
	rootCmd.AddCommand(receiptCmd)
}

type receiptDocument = memorysvc.ReceiptDocument

type receiptPrivacy struct {
	RawDetailIncluded bool   `json:"raw_detail_included"`
	HashAlgorithm     string `json:"hash_algorithm"`
	Note              string `json:"note"`
}

type receiptEvent struct {
	EventName     string `json:"event_name"`
	Operation     string `json:"operation"`
	CreatedAt     string `json:"created_at"`
	InsightIDHash string `json:"insight_id_hash,omitempty"`
	DetailHash    string `json:"detail_hash,omitempty"`
	DetailPresent bool   `json:"detail_present"`
}

type receiptDoc struct {
	Schema      string         `json:"schema"`
	GeneratedAt string         `json:"generated_at"`
	Store       string         `json:"store"`
	Limit       int            `json:"limit"`
	Count       int            `json:"count"`
	Privacy     receiptPrivacy `json:"privacy"`
	Events      []receiptEvent `json:"events"`
}

func buildReceipt(storeName string, limit int, entries []store.OplogEntry, generatedAt time.Time) receiptDoc {
	src := memorysvc.BuildReceipt(storeName, limit, entries, generatedAt)
	events := make([]receiptEvent, 0, len(src.Events))
	for _, e := range src.Events {
		ev := receiptEvent{
			EventName:     strField(e, "event_name"),
			Operation:     strField(e, "operation"),
			CreatedAt:     strField(e, "created_at"),
			InsightIDHash: strField(e, "insight_id_hash"),
			DetailHash:    strField(e, "detail_hash"),
		}
		if v, ok := e["detail_present"].(bool); ok {
			ev.DetailPresent = v
		}
		events = append(events, ev)
	}
	raw, _ := src.Privacy["raw_detail_included"].(bool)
	algo, _ := src.Privacy["hash_algorithm"].(string)
	note, _ := src.Privacy["note"].(string)
	return receiptDoc{
		Schema:      src.Schema,
		GeneratedAt: src.GeneratedAt,
		Store:       src.Store,
		Limit:       src.Limit,
		Count:       src.Count,
		Privacy: receiptPrivacy{
			RawDetailIncluded: raw,
			HashAlgorithm:     algo,
			Note:              note,
		},
		Events: events,
	}
}

func strField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

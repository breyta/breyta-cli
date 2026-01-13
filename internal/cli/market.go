package cli

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/state"

	"github.com/spf13/cobra"
)

func newRevenueCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "revenue", Short: "Marketplace revenue (mock)"}
	cmd.AddCommand(newRevenueShowCmd(app))
	return cmd
}

func newRevenueShowCmd(app *App) *cobra.Command {
	var last string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show revenue (mock)",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}

			since, err := parseWindow(last)
			if err != nil {
				return writeErr(cmd, err)
			}
			cutoff := time.Now().UTC().Add(-since)

			filtered := make([]state.RevenueEvent, 0, len(ws.RevenueEvents))
			for _, e := range ws.RevenueEvents {
				if e.At.After(cutoff) {
					filtered = append(filtered, e)
				}
			}
			sort.Slice(filtered, func(i, j int) bool { return filtered[i].At.After(filtered[j].At) })

			events := append([]any{}, anySliceRevenue(filtered)...)
			// total per currency
			totals := map[string]int64{}
			for _, e := range filtered {
				totals[e.Currency] += e.AmountCents
			}

			return writeData(cmd, app, nil, map[string]any{
				"window": last,
				"totals": totals,
				"events": events,
			})
		},
	}
	cmd.Flags().StringVar(&last, "last", "30d", "Window (e.g. 30d)")
	return cmd
}

func newDemandCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "demand", Short: "Marketplace demand signals (mock)"}
	cmd.AddCommand(newDemandTopCmd(app))
	cmd.AddCommand(newDemandIngestCmd(app))
	cmd.AddCommand(newDemandQueriesCmd(app))
	cmd.AddCommand(newDemandClustersCmd(app))
	cmd.AddCommand(newDemandClusterShowCmd(app))
	return cmd
}

func newDemandTopCmd(app *App) *cobra.Command {
	var window string
	cmd := &cobra.Command{
		Use:   "top",
		Short: "Show top demand (mock)",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			// Prefer the new clustered demand view, but keep legacy DemandTop if clusters are empty.
			items := make([]any, 0)
			if len(ws.DemandClusters) > 0 {
				type citem struct {
					c state.DemandCluster
				}
				cs := make([]citem, 0, len(ws.DemandClusters))
				for _, c := range ws.DemandClusters {
					cs = append(cs, citem{c: c})
				}
				sort.Slice(cs, func(i, j int) bool { return cs[i].c.Count > cs[j].c.Count })
				for _, x := range cs {
					items = append(items, map[string]any{
						"clusterId":       x.c.ID,
						"query":           x.c.Title,
						"count":           x.c.Count,
						"window":          x.c.Window,
						"suggestedPrice":  x.c.SuggestedPrice,
						"matchedListings": x.c.MatchedListings,
					})
				}
			} else {
				items = append([]any{}, anySliceDemand(ws.DemandTop)...)
				sort.Slice(items, func(i, j int) bool {
					// items are maps; keep stable order by count descending when possible
					mi, iok := items[i].(map[string]any)
					mj, jok := items[j].(map[string]any)
					if iok && jok {
						ci, _ := mi["count"].(int)
						cj, _ := mj["count"].(int)
						return ci > cj
					}
					return false
				})
			}

			meta := map[string]any{"window": window, "hint": "Use `demand clusters` for clustered demand and `demand ingest` to add new signals."}
			return writeData(cmd, app, meta, map[string]any{
				"window": window,
				"items":  items,
			})
		},
	}
	cmd.Flags().StringVar(&window, "window", "30d", "Window (e.g. 30d)")
	return cmd
}

func newDemandIngestCmd(app *App) *cobra.Command {
	var (
		window   string
		offer    int64
		currency string
		norm     string
	)
	cmd := &cobra.Command{
		Use:   "ingest <query>",
		Short: "Ingest a demand query (mock)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			q := strings.TrimSpace(args[0])
			if q == "" {
				return writeErr(cmd, errors.New("empty query"))
			}
			if window == "" {
				window = "30d"
			}
			if currency == "" {
				currency = "USD"
			}
			if norm == "" {
				norm = normalizeDemandQuery(q)
			}

			now := time.Now().UTC()
			ws.DemandQueries = append(ws.DemandQueries, state.DemandQuery{
				Query:        q,
				At:           now,
				Window:       window,
				OfferCents:   offer,
				Currency:     currency,
				NormalizedTo: norm,
			})

			// Update clusters (very naive).
			c := findOrCreateCluster(ws, norm, window)
			c.Count++
			if len(c.Examples) < 8 {
				c.Examples = append(c.Examples, q)
			}
			if offer > 0 {
				// A "best effort" suggested price string.
				c.SuggestedPrice = fmt.Sprintf("$%.2f / success", float64(offer)/100.0)
			}
			// Refresh matched listings based on keywords.
			c.MatchedListings = matchListings(ws, norm)

			ws.UpdatedAt = now
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			meta := map[string]any{"hint": "Mock ingest updates demandQueries and demandClusters in state."}
			return writeData(cmd, app, meta, map[string]any{"query": q, "normalizedTo": norm, "clusterId": c.ID})
		},
	}
	cmd.Flags().StringVar(&window, "window", "30d", "Window (e.g. 30d)")
	cmd.Flags().Int64Var(&offer, "offer-cents", 0, "Willingness-to-pay offer (cents)")
	cmd.Flags().StringVar(&currency, "currency", "USD", "Offer currency")
	cmd.Flags().StringVar(&norm, "normalized-to", "", "Override normalized cluster title (advanced)")
	return cmd
}

func newDemandQueriesCmd(app *App) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "queries",
		Short: "List raw demand queries (mock)",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			items := make([]state.DemandQuery, 0, len(ws.DemandQueries))
			items = append(items, ws.DemandQueries...)
			sort.Slice(items, func(i, j int) bool { return items[i].At.After(items[j].At) })
			if limit <= 0 {
				limit = 50
			}
			if len(items) > limit {
				items = items[:limit]
			}
			out := make([]any, 0, len(items))
			for _, q := range items {
				out = append(out, q)
			}
			return writeData(cmd, app, map[string]any{"total": len(out)}, map[string]any{"items": out})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Max items")
	return cmd
}

func newDemandClustersCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clusters",
		Short: "List demand clusters (mock)",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			cs := make([]state.DemandCluster, 0, len(ws.DemandClusters))
			cs = append(cs, ws.DemandClusters...)
			sort.Slice(cs, func(i, j int) bool { return cs[i].Count > cs[j].Count })
			items := make([]any, 0, len(cs))
			for _, c := range cs {
				items = append(items, map[string]any{
					"clusterId":       c.ID,
					"title":           c.Title,
					"count":           c.Count,
					"window":          c.Window,
					"suggestedPrice":  c.SuggestedPrice,
					"matchedListings": c.MatchedListings,
				})
			}
			return writeData(cmd, app, map[string]any{"total": len(items), "hint": "Use `demand cluster <id>` for details"}, map[string]any{"items": items})
		},
	}
	return cmd
}

func newDemandClusterShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster <cluster-id>",
		Short: "Show a demand cluster (mock)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			id := strings.TrimSpace(args[0])
			for _, c := range ws.DemandClusters {
				if c.ID == id {
					return writeData(cmd, app, nil, map[string]any{"cluster": c})
				}
			}
			return writeErr(cmd, errors.New("cluster not found"))
		},
	}
	return cmd
}

func normalizeDemandQuery(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// strip some punctuation
	s = strings.NewReplacer(",", " ", ".", " ", ":", " ", ";", " ", "?", " ", "!", " ", "/", " ", "\\", " ", "\"", " ", "'", " ").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	// keep it short for cluster titles
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func findOrCreateCluster(ws *state.Workspace, title, window string) *state.DemandCluster {
	// Try exact title match.
	for i := range ws.DemandClusters {
		if ws.DemandClusters[i].Title == title {
			return &ws.DemandClusters[i]
		}
	}
	id := fmt.Sprintf("dem-%d", time.Now().UTC().UnixNano())
	ws.DemandClusters = append(ws.DemandClusters, state.DemandCluster{
		ID:              id,
		Title:           title,
		Count:           0,
		Window:          window,
		Examples:        []string{},
		MatchedListings: []string{},
	})
	return &ws.DemandClusters[len(ws.DemandClusters)-1]
}

func matchListings(ws *state.Workspace, q string) []string {
	toks := strings.Fields(strings.ToLower(q))
	out := make([]string, 0)
	if len(toks) == 0 {
		return out
	}
	for id, e := range ws.Registry {
		hay := strings.ToLower(e.Slug + " " + e.Title + " " + e.Summary + " " + strings.Join(e.Tags, " "))
		hit := 0
		for _, t := range toks {
			if t != "" && strings.Contains(hay, t) {
				hit++
			}
		}
		if hit > 0 {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func anySliceRevenue(xs []state.RevenueEvent) []any {
	items := make([]any, 0, len(xs))
	for _, e := range xs {
		items = append(items, map[string]any{
			"at":          e.At,
			"currency":    e.Currency,
			"amountCents": e.AmountCents,
			"source":      e.Source,
			"flowSlug":    e.FlowSlug,
			"runId":       e.RunID,
		})
	}
	return items
}

func anySliceDemand(xs []state.DemandItem) []any {
	items := make([]any, 0, len(xs))
	for _, d := range xs {
		items = append(items, map[string]any{
			"query":          d.Query,
			"count":          d.Count,
			"window":         d.Window,
			"suggestedPrice": d.SuggestedPrice,
			"matchedFlows":   d.MatchedFlows,
		})
	}
	return items
}

func parseWindow(s string) (time.Duration, error) {
	// supports: "30d", "24h", "60m"
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty window")
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, errors.New("invalid day window")
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, errors.New("invalid window")
	}
	return d, nil
}

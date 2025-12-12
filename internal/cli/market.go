package cli

import (
        "errors"
        "sort"
        "strconv"
        "strings"
        "time"

        "breyta-cli/internal/state"

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

                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "window":      last,
                                "totals":      totals,
                                "events":      events,
                        })
                },
        }
        cmd.Flags().StringVar(&last, "last", "30d", "Window (e.g. 30d)")
        return cmd
}

func newDemandCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "demand", Short: "Marketplace demand signals (mock)"}
        cmd.AddCommand(newDemandTopCmd(app))
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
                        items := append([]any{}, anySliceDemand(ws.DemandTop)...)
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

                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "window":      window,
                                "items":       items,
                        })
                },
        }
        cmd.Flags().StringVar(&window, "window", "30d", "Window (e.g. 30d)")
        return cmd
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

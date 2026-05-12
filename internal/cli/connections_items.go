package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/breyta/breyta-cli/internal/state"

	"github.com/spf13/cobra"
)

func newConnectionsItemsCmd(app *App) *cobra.Command {
	var itemType string
	var limit int
	var includeRaw bool
	cmd := &cobra.Command{
		Use:   "items <connection-id>",
		Short: "Inspect cached connection items",
		Long: strings.TrimSpace(`
Inspect cached non-secret items for a workspace connection.

Connection item caches back installable-flow dropdowns such as repository,
channel, project, folder, or account selectors. The default output summarizes
cached rows and omits raw provider payloads. Use --raw only when debugging item
metadata, and --limit 0 only when the full dropdown cache is required.`),
		Example: strings.TrimSpace(`
breyta connections items conn-github
breyta connections items conn-github --item-type github/repository --limit 25
breyta connections items conn-github --item-type github/repository --raw
breyta connections items conn-github --item-type github/repository --limit 0`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			connectionID := strings.TrimSpace(args[0])
			if connectionID == "" {
				return writeErr(cmd, errors.New("missing connection-id"))
			}
			if limit < 0 {
				return writeErr(cmd, errors.New("--limit must be 0 or greater"))
			}

			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				out, status, err := fetchConnectionItemsAPI(app, connectionID, strings.TrimSpace(itemType), limit)
				if err != nil {
					return writeErr(cmd, err)
				}
				if status >= 400 {
					return writeREST(cmd, app, status, out)
				}
				return writeREST(cmd, app, status, normalizeConnectionItemsAPIResponse(connectionID, out, strings.TrimSpace(itemType), includeRaw))
			}

			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			conn := ws.Connections[connectionID]
			if conn == nil {
				return writeErr(cmd, errors.New("connection not found"))
			}
			return writeData(cmd, app, nil, buildConnectionItemsData(connectionID, conn, strings.TrimSpace(itemType), limit, includeRaw))
		},
	}
	cmd.Flags().StringVar(&itemType, "item-type", "", "Filter to one cached item type")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum items to return; use 0 for all")
	cmd.Flags().BoolVar(&includeRaw, "raw", false, "Include raw cached item payloads")
	return cmd
}

const connectionItemsAllPageLimit = 500

func fetchConnectionItemsAPI(app *App, connectionID string, itemType string, limit int) (any, int, error) {
	if itemType == "" || limit != 0 {
		return fetchConnectionItemsAPIPage(app, connectionID, itemType, limit, "")
	}

	var firstPage map[string]any
	allItems := make([]any, 0)
	cursor := ""
	seenCursors := map[string]bool{}
	for {
		out, status, err := fetchConnectionItemsAPIPage(app, connectionID, itemType, connectionItemsAllPageLimit, cursor)
		if err != nil || status >= 400 {
			return out, status, err
		}
		pageMap := asAnyMap(out)
		if pageMap == nil {
			return out, status, err
		}
		if firstPage == nil {
			firstPage = cloneAnyMap(pageMap)
		}
		allItems = append(allItems, anySlice(pageMap["items"])...)
		nextCursor := firstNonEmptyString(
			lookupString(pageMap, "nextCursor"),
			lookupString(pageMap, "next-cursor"),
		)
		hasMore := lookupBool(pageMap, "hasMore", "has-more")
		if !hasMore || nextCursor == "" {
			if hasMore && nextCursor == "" {
				return nil, 0, errors.New("connection items response had hasMore=true without a next cursor")
			}
			break
		}
		if seenCursors[nextCursor] {
			return nil, 0, fmt.Errorf("connection items pagination repeated cursor %q", nextCursor)
		}
		seenCursors[nextCursor] = true
		cursor = nextCursor
	}
	if firstPage == nil {
		firstPage = map[string]any{}
	}
	firstPage["items"] = allItems
	firstPage["hasMore"] = false
	firstPage["nextCursor"] = nil
	firstPage["has-more"] = false
	firstPage["next-cursor"] = nil
	if summary := asAnyMap(firstPage["summary"]); summary != nil {
		summary["returned"] = len(allItems)
		firstPage["summary"] = summary
	}
	return firstPage, http.StatusOK, nil
}

func fetchConnectionItemsAPIPage(app *App, connectionID string, itemType string, limit int, cursor string) (any, int, error) {
	q := url.Values{}
	if strings.TrimSpace(itemType) != "" {
		q.Set("item-type", strings.TrimSpace(itemType))
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if strings.TrimSpace(cursor) != "" {
		q.Set("cursor", strings.TrimSpace(cursor))
	}
	return apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/connections/"+url.PathEscape(connectionID)+"/items", q, nil)
}

func normalizeConnectionItemsAPIResponse(connectionID string, response any, itemType string, includeRaw bool) any {
	m := asAnyMap(response)
	if m == nil {
		return response
	}
	items, _ := m["items"].([]any)
	_, hasItemTypes := m["itemTypes"]
	summary := asAnyMap(m["summary"])
	if itemType == "" {
		if !hasItemTypes {
			m["itemTypes"] = items
		}
		if summary == nil {
			summary = map[string]any{}
		}
		if _, ok := summary["itemTypes"]; !ok {
			summary["itemTypes"] = len(items)
		}
		if connectionID != "" {
			summary["connectionId"] = connectionID
		}
		m["summary"] = summary
		return m
	}
	typeRows := make([]map[string]any, 0, 1)
	if summary != nil {
		if count, ok := summary["count"]; ok {
			typeRows = append(typeRows, map[string]any{"itemType": itemType, "count": count})
		}
	}
	itemRows := make([]map[string]any, 0, len(items))
	for idx, item := range items {
		itemRows = append(itemRows, summarizeConnectionItem(itemType, idx, item, includeRaw))
	}
	if summary == nil {
		summary = map[string]any{}
	}
	if _, ok := summary["items"]; !ok {
		if count, ok := summary["count"]; ok {
			summary["items"] = count
		} else {
			summary["items"] = len(itemRows)
		}
	}
	summary["returned"] = len(itemRows)
	summary["filteredItemType"] = itemType
	nextCursor := firstNonEmptyString(
		lookupString(m, "nextCursor"),
		lookupString(m, "next-cursor"),
	)
	var nextCursorValue any
	if nextCursor != "" {
		nextCursorValue = nextCursor
	}
	return map[string]any{
		"connectionId": connectionID,
		"itemTypes":    typeRows,
		"items":        itemRows,
		"summary":      summary,
		"nextCursor":   nextCursorValue,
		"hasMore":      lookupBool(m, "hasMore", "has-more"),
	}
}

func buildConnectionItemsData(connectionID string, connection any, itemType string, limit int, includeRaw bool) map[string]any {
	connMap := connectionToMap(connection)
	if connectionID == "" {
		connectionID = firstNonEmptyString(
			lookupString(connMap, "id"),
			lookupString(connMap, "connectionId"),
			lookupString(connMap, "connection-id"),
		)
	}

	itemsByType := extractConnectionItems(connMap)
	itemTypes := make([]string, 0, len(itemsByType))
	for typ := range itemsByType {
		if itemType != "" && typ != itemType {
			continue
		}
		itemTypes = append(itemTypes, typ)
	}
	sort.Strings(itemTypes)

	typeRows := make([]map[string]any, 0, len(itemTypes))
	itemRows := make([]map[string]any, 0)
	totalItems := 0
	for _, typ := range itemTypes {
		rawItems := itemsByType[typ]
		totalItems += len(rawItems)
		typeRows = append(typeRows, map[string]any{
			"itemType": typ,
			"count":    len(rawItems),
		})
		for idx, raw := range rawItems {
			if limit > 0 && len(itemRows) >= limit {
				continue
			}
			itemRows = append(itemRows, summarizeConnectionItem(typ, idx, raw, includeRaw))
		}
	}

	summary := map[string]any{
		"itemTypes": len(typeRows),
		"items":     totalItems,
		"returned":  len(itemRows),
	}
	if itemType != "" {
		summary["filteredItemType"] = itemType
	}

	return map[string]any{
		"connectionId": connectionID,
		"itemTypes":    typeRows,
		"items":        itemRows,
		"summary":      summary,
	}
}

func extractConnectionItems(connection map[string]any) map[string][]any {
	out := map[string][]any{}
	for _, source := range []any{
		lookupAny(connection, "connectionItems", "connection-items", "connection_items"),
		lookupAny(asAnyMap(lookupAny(connection, "config")), "connectionItems", "connection-items", "connection_items"),
	} {
		switch typed := source.(type) {
		case map[string]any:
			for key, value := range typed {
				itemType := strings.TrimSpace(key)
				if itemType == "" {
					continue
				}
				out[itemType] = append(out[itemType], anySlice(value)...)
			}
		case []any:
			for _, item := range typed {
				itemMap := asAnyMap(item)
				itemType := strings.TrimSpace(lookupString(itemMap, "itemType", "item-type", "type"))
				if itemType == "" {
					itemType = "default"
				}
				out[itemType] = append(out[itemType], item)
			}
		}
	}
	return out
}

func summarizeConnectionItem(itemType string, index int, raw any, includeRaw bool) map[string]any {
	item := asAnyMap(raw)
	row := map[string]any{
		"itemType": itemType,
		"index":    index,
	}
	if includeRaw {
		row["raw"] = raw
	}
	if value := firstNonEmptyString(
		lookupString(item, "value"),
		lookupString(item, "fullName", "full-name", "full_name"),
		lookupString(item, "key"),
		lookupString(item, "slug"),
		lookupString(item, "name"),
		lookupString(item, "id"),
	); value != "" {
		row["value"] = value
	}
	if label := firstNonEmptyString(
		lookupString(item, "label"),
		lookupString(item, "name"),
		lookupString(item, "fullName", "full-name", "full_name"),
		lookupString(item, "title"),
		lookupString(item, "id"),
		lookupString(item, "value"),
	); label != "" {
		row["label"] = label
	}
	if description := firstNonEmptyString(
		lookupString(item, "description"),
		lookupString(item, "desc"),
		lookupString(item, "summary"),
	); description != "" {
		row["description"] = description
	}
	return row
}

func connectionToMap(connection any) map[string]any {
	if m := asAnyMap(connection); m != nil {
		return m
	}
	switch typed := connection.(type) {
	case *state.Connection:
		return map[string]any{
			"id":        typed.ID,
			"name":      typed.Name,
			"type":      typed.Type,
			"status":    typed.Status,
			"updatedAt": typed.UpdatedAt,
			"config":    typed.Config,
		}
	case state.Connection:
		return connectionToMap(&typed)
	default:
		var out map[string]any
		b, err := json.Marshal(connection)
		if err == nil {
			_ = json.Unmarshal(b, &out)
		}
		if out == nil {
			out = map[string]any{}
		}
		return out
	}
}

func asAnyMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = v
		}
		return out
	default:
		var out map[string]any
		b, err := json.Marshal(value)
		if err == nil {
			_ = json.Unmarshal(b, &out)
		}
		return out
	}
}

func cloneAnyMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for k, v := range value {
		out[k] = v
	}
	return out
}

func anySlice(value any) []any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return []any{value}
	}
}

func lookupAny(m map[string]any, keys ...string) any {
	if m == nil {
		return nil
	}
	for _, key := range keys {
		if value, ok := m[key]; ok {
			return value
		}
	}
	return nil
}

func lookupString(m map[string]any, keys ...string) string {
	value := lookupAny(m, keys...)
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func lookupBool(m map[string]any, keys ...string) bool {
	value := lookupAny(m, keys...)
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

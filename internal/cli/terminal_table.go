package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

func writeTerminalTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(headers) > 0 {
		if err := writeTerminalTableRow(tw, headers); err != nil {
			return err
		}
	}
	for _, row := range rows {
		if err := writeTerminalTableRow(tw, row); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func writeTerminalTableRow(w io.Writer, cells []string) error {
	for i, cell := range cells {
		if i > 0 {
			if _, err := io.WriteString(w, "\t"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, terminalTableCell(cell)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func terminalTableCell(value string) string {
	replacer := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	return strings.TrimSpace(replacer.Replace(value))
}

func terminalTableList(value any) string {
	switch typed := value.(type) {
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := scalarString(item); s != "" {
				items = append(items, s)
			}
		}
		return strings.Join(items, ",")
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(item); s != "" {
				items = append(items, s)
			}
		}
		return strings.Join(items, ",")
	default:
		return scalarString(value)
	}
}

func writeFlowSearchHitsTable(w io.Writer, out map[string]any) error {
	data := mapStringAny(out["data"])
	result := mapStringAny(data["result"])
	hits := sliceAny(result["hits"])
	rows := make([][]string, 0, len(hits))
	for _, hitAny := range hits {
		hit := mapStringAny(hitAny)
		if hit == nil {
			continue
		}
		rows = append(rows, []string{
			firstNonBlankString(hit["flow_slug"], hit["flowSlug"], hit["slug"]),
			truncateRunes(firstNonBlankString(hit["name"], hit["title"]), 42),
			terminalTableList(firstPresentAny(hit["step_types"], hit["stepTypes"])),
			scalarString(firstPresentAny(hit["step_count"], hit["stepCount"])),
			terminalTableList(firstPresentAny(hit["providers"])),
		})
	}
	return writeTerminalTable(w, []string{"slug", "name", "steps", "count", "providers"}, rows)
}

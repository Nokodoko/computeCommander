// Package export provides data export functionality for ComputeCommander.
// It reads data from the SQLite database and outputs it as JSON or CSV.
package export

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// ExportOpts configures an export operation.
type ExportOpts struct {
	Format  string   // "json" or "csv"
	Tables  []string // tables to export (empty = all)
	Writer  io.Writer
	Version string
	Project string
}

// ExportResult contains metadata about the export operation.
type ExportResult struct {
	ExportedAt string   `json:"exportedAt"`
	Tables     []string `json:"tables"`
	TotalRows  int      `json:"totalRows"`
}

// DefaultTables returns the list of tables available for export.
func DefaultTables() []string {
	return []string{"sessions", "events", "mail", "merge_queue", "metrics", "runs"}
}

// Export reads data from the database and writes it to the provided writer.
func Export(ctx context.Context, database db.DB, opts ExportOpts) (*ExportResult, error) {
	tables := opts.Tables
	if len(tables) == 0 {
		tables = DefaultTables()
	}

	format := opts.Format
	if format == "" {
		format = "json"
	}

	now := time.Now().Format(time.RFC3339)
	totalRows := 0

	switch format {
	case "json":
		result := map[string]any{
			"exportedAt":  now,
			"version":     opts.Version,
			"projectName": opts.Project,
		}

		for _, table := range tables {
			rows, count, err := exportTable(ctx, database, table)
			if err != nil {
				// Table may not exist; skip with empty data.
				result[table] = []map[string]any{}
				continue
			}
			result[table] = rows
			totalRows += count
		}

		enc := json.NewEncoder(opts.Writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return nil, fmt.Errorf("encode JSON: %w", err)
		}

	case "csv":
		for _, table := range tables {
			rows, count, err := exportTableCSV(ctx, database, table, opts.Writer)
			if err != nil {
				continue
			}
			_ = rows
			totalRows += count
		}

	default:
		return nil, fmt.Errorf("unsupported format: %q (use json or csv)", format)
	}

	return &ExportResult{
		ExportedAt: now,
		Tables:     tables,
		TotalRows:  totalRows,
	}, nil
}

// exportTable reads all rows from a table and returns them as maps.
func exportTable(ctx context.Context, database db.DB, table string) ([]map[string]any, int, error) {
	// Query all rows from the table.
	query := fmt.Sprintf("SELECT * FROM %s", table)
	rows, err := database.Query(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("query table %s: %w", table, err)
	}
	defer rows.Close()

	var result []map[string]any
	count := 0

	// Since our DB interface doesn't expose column names via Rows,
	// we export a simplified representation per table using known schemas.
	for rows.Next() {
		row, err := scanTableRow(table, rows)
		if err != nil {
			continue
		}
		result = append(result, row)
		count++
	}

	return result, count, rows.Err()
}

// scanTableRow scans a single row based on the table name.
func scanTableRow(table string, rows *db.Rows) (map[string]any, error) {
	switch table {
	case "sessions":
		var id, name, capability, wtPath, branch, taskID, pane, state, parent, runID, startedAt, lastActivity, stalledSince, transcript, runtime string
		var pid, depth, escalation int
		if err := rows.Scan(&id, &name, &capability, &wtPath, &branch, &taskID, &pane, &state, &pid, &parent, &depth, &runID, &startedAt, &lastActivity, &escalation, &stalledSince, &transcript, &runtime); err != nil {
			return nil, err
		}
		return map[string]any{
			"id": id, "agentName": name, "capability": capability,
			"state": state, "runtime": runtime, "startedAt": startedAt,
		}, nil

	case "events":
		var id, agent, eventType, toolName, data, level, createdAt string
		var runID string
		if err := rows.Scan(&id, &runID, &agent, &eventType, &toolName, &data, &level, &createdAt); err != nil {
			return nil, err
		}
		return map[string]any{
			"id": id, "agent": agent, "eventType": eventType,
			"data": data, "level": level, "createdAt": createdAt,
		}, nil

	case "metrics":
		var agentName, capability, modelUsed, startedAt string
		var inputTokens, outputTokens int64
		var durationMs int
		var estimatedCost float64
		if err := rows.Scan(&agentName, &capability, &modelUsed, &durationMs, &inputTokens, &outputTokens, &estimatedCost, &startedAt); err != nil {
			return nil, err
		}
		return map[string]any{
			"agent": agentName, "capability": capability, "model": modelUsed,
			"inputTokens": inputTokens, "outputTokens": outputTokens,
			"estimatedCost": estimatedCost,
		}, nil

	default:
		// For unknown tables, try a generic 2-column scan.
		var col1, col2 string
		if err := rows.Scan(&col1, &col2); err != nil {
			return nil, err
		}
		return map[string]any{"col1": col1, "col2": col2}, nil
	}
}

// exportTableCSV exports a table as CSV.
func exportTableCSV(ctx context.Context, database db.DB, table string, w io.Writer) ([][]string, int, error) {
	query := fmt.Sprintf("SELECT * FROM %s LIMIT 10000", table)
	rows, err := database.Query(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write table header.
	if err := writer.Write([]string{fmt.Sprintf("--- Table: %s ---", table)}); err != nil {
		return nil, 0, err
	}

	count := 0
	for rows.Next() {
		row, err := scanTableRow(table, rows)
		if err != nil {
			continue
		}
		var record []string
		for _, v := range row {
			record = append(record, fmt.Sprint(v))
		}
		if err := writer.Write(record); err != nil {
			return nil, count, err
		}
		count++
	}

	return nil, count, rows.Err()
}

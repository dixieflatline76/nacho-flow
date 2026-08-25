package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/dixieflatline76/nacho-flow/pkg/server"
)

// Presentation header, rule, and tip constants.
const (
	BannerHeatSeeker      = "\n🔥  HEAT SEEKER"
	BannerElCazador       = BannerHeatSeeker // Alias for backward compatibility
	BannerBenchmarkFmt    = "Benchmark: %s ($%.2f/1M tokens)"
	BannerRule            = "─────────────────────────────────────────────────────────────────────────────────────────────────────────────────"
	TableHeaderCols       = "MODEL\tROLE\tCONTEXT\tPROMPT/1M\tCOMP/1M\tCODING\tDISCOUNT\t"
	TipExtensionDashboard = "💡 Tip: Use the VS Code extension dashboard to adopt any deal into your active routing tiers with 1-click."
	TabWriterPadding      = 2
	TabWriterPadChar      = ' '
	JSONDefaultIndent     = "  "
)

// DealsReporter defines the strategy interface for outputting deal intelligence.
type DealsReporter interface {
	Render(out io.Writer, resp server.DealsResponse) error
}

// TableReporter formats deal intelligence into an aligned ASCII terminal table using text/tabwriter.
type TableReporter struct {
	minWidth int
	tabWidth int
	padding  int
	padChar  byte
}

// NewTableReporter initializes a configured TableReporter.
func NewTableReporter() *TableReporter {
	return &TableReporter{
		minWidth: 0,
		tabWidth: 0,
		padding:  TabWriterPadding,
		padChar:  TabWriterPadChar,
	}
}

// Render writes a formatted, elastic table to the destination writer.
func (r *TableReporter) Render(out io.Writer, resp server.DealsResponse) error {
	fmt.Fprintln(out, BannerHeatSeeker)
	fmt.Fprintf(out, BannerBenchmarkFmt+"\n", resp.BenchmarkModel, resp.BenchmarkCostPerM)
	fmt.Fprintln(out, BannerRule)

	w := tabwriter.NewWriter(out, r.minWidth, r.tabWidth, r.padding, r.padChar, 0)
	fmt.Fprintln(w, TableHeaderCols)

	for _, deal := range resp.Deals {
		view := ToDealRowView(deal)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s %s\t\n",
			view.ModelID, view.Role, view.Context, view.PromptPrice, view.CompPrice, view.CodingScore, view.Discount, view.Badge)
		if view.Why != "" {
			fmt.Fprintf(w, "   ↳ %s\t\t\t\t\t\t\t\n", view.Why)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush tabwriter: %w", err)
	}

	fmt.Fprintln(out, BannerRule)
	fmt.Fprintln(out, TipExtensionDashboard)
	return nil
}

// JSONReporter formats deal intelligence as structured JSON.
type JSONReporter struct {
	indent string
}

// NewJSONReporter initializes a configured JSONReporter.
func NewJSONReporter() *JSONReporter {
	return &JSONReporter{indent: JSONDefaultIndent}
}

// Render writes formatted JSON to the destination writer.
func (r *JSONReporter) Render(out io.Writer, resp server.DealsResponse) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", r.indent)
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to encode deals json: %w", err)
	}
	return nil
}

// NewDealsReporter is the strategy factory for selecting the appropriate reporter.
func NewDealsReporter(asJSON bool) DealsReporter {
	if asJSON {
		return NewJSONReporter()
	}
	return NewTableReporter()
}

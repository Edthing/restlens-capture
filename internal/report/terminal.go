package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// RenderTerminal outputs a human-readable traffic summary.
func RenderTerminal(summary TrafficSummary, w io.Writer) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "restlens-capture Report")
	fmt.Fprintln(w, "=======================")
	fmt.Fprintln(w, "")

	if summary.TotalHits == 0 {
		fmt.Fprintln(w, "No traffic captured.")
		return
	}

	fmt.Fprintf(w, "Captured %d requests across %d endpoints\n",
		summary.TotalHits, len(summary.Endpoints))
	fmt.Fprintf(w, "Period: %s to %s\n",
		summary.DateRange[0].Format("2006-01-02 15:04"),
		summary.DateRange[1].Format("2006-01-02 15:04"))
	fmt.Fprintln(w, "")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ENDPOINT\tHITS\tSTATUS CODES\tAVG (ms)\tP95 (ms)")
	fmt.Fprintln(tw, "--------\t----\t------------\t--------\t--------")

	for _, ep := range summary.Endpoints {
		fmt.Fprintf(tw, "%s %s\t%d\t%s\t%.0f\t%.0f\n",
			ep.Method,
			ep.Pattern,
			ep.HitCount,
			formatStatusCodes(ep.StatusCodes),
			ep.AvgLatency,
			ep.P95Latency,
		)
	}

	tw.Flush()
	fmt.Fprintln(w, "")
}

func formatStatusCodes(codes map[int]int) string {
	type codeCount struct {
		code  int
		count int
	}
	sorted := make([]codeCount, 0, len(codes))
	for c, n := range codes {
		sorted = append(sorted, codeCount{c, n})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].code < sorted[j].code })

	parts := make([]string, len(sorted))
	for i, cc := range sorted {
		parts[i] = fmt.Sprintf("%d(%d)", cc.code, cc.count)
	}
	return strings.Join(parts, " ")
}

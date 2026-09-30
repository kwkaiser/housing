package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func Table(w io.Writer, r Report) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := []string{"#", "SCORE"}
	if len(r.Models) > 1 {
		for _, m := range r.Models {
			header = append(header, strings.ToUpper(shortModel(m)))
		}
	}
	header = append(header, "COV", "VIBE", "MISSING", "DEALBREAKERS", "PRICE", "BEDS", "SOURCE", "ADDRESS", "URL")
	fmt.Fprintln(tw, strings.Join(header, "\t"))

	for _, row := range r.Rows {
		cols := []string{strconv.Itoa(row.Rank), score(row)}
		if len(r.Models) > 1 {
			for _, m := range r.Models {
				cols = append(cols, modelScore(row, m))
			}
		}
		cols = append(cols,
			fmt.Sprintf("%.0f%%", row.Coverage),
			fmt.Sprintf("%g", row.Vibe),
			dash(strings.Join(row.MissingEssentials, ",")),
			dash(strings.Join(row.Dealbreakers, ",")),
			price(row.Listing),
			beds(row.Listing),
			string(row.Listing.Source),
			address(row.Listing),
			row.Listing.URL,
		)
		fmt.Fprintln(tw, strings.Join(cols, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return footer(w, r)
}

func Markdown(w io.Writer, r Report) error {
	fmt.Fprintf(w, "# %s\n\n", cmpStr(r.Profile.Name, r.Profile.ID))
	if r.Profile.Summary != "" {
		fmt.Fprintf(w, "%s\n\n", r.Profile.Summary)
	}
	fmt.Fprintf(w, "Profile `%s`, graded by %s.\n\n", r.Profile.ID, strings.Join(r.Models, ", "))

	for _, row := range r.Rows {
		fmt.Fprintf(w, "- **#%d · %s** — [%s](%s) (%s)\n", row.Rank, score(row), address(row.Listing), row.Listing.URL, row.Listing.Source)
		fmt.Fprintf(w, "  - %s · %s · coverage %.0f%% · vibe %g/5\n", price(row.Listing), bedsLabel(row.Listing), row.Coverage, row.Vibe)
		if len(r.Models) > 1 {
			var scores []string
			for _, m := range r.Models {
				scores = append(scores, shortModel(m)+" "+modelScore(row, m))
			}
			fmt.Fprintf(w, "  - Scores: %s\n", strings.Join(scores, ", "))
		}
		if len(row.Dealbreakers) > 0 {
			fmt.Fprintf(w, "  - Dealbreakers: %s\n", strings.Join(row.Dealbreakers, ", "))
		}
		if len(row.MissingEssentials) > 0 {
			fmt.Fprintf(w, "  - Missing essentials: %s\n", strings.Join(row.MissingEssentials, ", "))
		}
		if len(row.AvoidsHit) > 0 {
			fmt.Fprintf(w, "  - Avoids hit: %s\n", strings.Join(row.AvoidsHit, ", "))
		}
		if row.Summary != "" {
			fmt.Fprintf(w, "  - %s\n", strings.ReplaceAll(row.Summary, "\n", " "))
		}
	}
	fmt.Fprintln(w)
	return footer(w, r)
}

func JSON(w io.Writer, r Report) error {
	rows := make([]Row, len(r.Rows))
	for i, row := range r.Rows {
		row.Listing.Raw = nil
		row.Listing.Assessments = nil
		rows[i] = row
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		Profile string   `json:"profile"`
		Models  []string `json:"models"`
		Rows    []Row    `json:"rows"`
	}{r.Profile.ID, r.Models, rows})
}

func footer(w io.Writer, r Report) error {
	if r.Stale > 0 {
		_, err := fmt.Fprintf(w, "\n%d listing(s) omitted because their assessment is stale (profile or listing changed); rerun `housing assess` or pass --include-stale.\n", r.Stale)
		return err
	}
	return nil
}

func score(row Row) string {
	s := fmt.Sprintf("%.1f", row.Score)
	if row.Stale {
		s += "*"
	}
	return s
}

func modelScore(row Row, model string) string {
	if a, ok := row.ByModel[model]; ok {
		return fmt.Sprintf("%.1f", a.Score)
	}
	return "-"
}

func shortModel(m string) string {
	if _, name, ok := strings.Cut(m, "/"); ok {
		return name
	}
	return m
}

func price(l listing.Listing) string {
	if l.Price.Cents == 0 {
		return "-"
	}
	s := "$" + thousands(l.Price.Cents/100)
	if l.Offer == listing.OfferRent {
		s += "/mo"
	}
	return s
}

func thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func beds(l listing.Listing) string {
	if l.Beds == nil {
		return "-"
	}
	if *l.Beds == 0 {
		return "studio"
	}
	return strconv.Itoa(*l.Beds)
}

func address(l listing.Listing) string {
	a := l.Address.Formatted
	if l.Address.Unit != "" && !strings.Contains(a, l.Address.Unit) {
		a += " #" + l.Address.Unit
	}
	return a
}

func bedsLabel(l listing.Listing) string {
	switch b := beds(l); b {
	case "-":
		return "beds unknown"
	case "studio":
		return b
	case "1":
		return "1 bed"
	default:
		return b + " beds"
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func cmpStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

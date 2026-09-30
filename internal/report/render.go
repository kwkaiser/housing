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
	header = append(header, "COV", "VIBE", "MISSING", "PRICE", "BEDS", "SOURCE", "ADDRESS", "URL")
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

	header := []string{"#", "Score"}
	if len(r.Models) > 1 {
		for _, m := range r.Models {
			header = append(header, shortModel(m))
		}
	}
	header = append(header, "Coverage", "Vibe", "Missing essentials", "Price", "Beds", "Listing", "Summary")
	fmt.Fprintf(w, "| %s |\n|%s\n", strings.Join(header, " | "), strings.Repeat(" --- |", len(header)))

	for _, row := range r.Rows {
		cols := []string{strconv.Itoa(row.Rank), "**" + score(row) + "**"}
		if len(r.Models) > 1 {
			for _, m := range r.Models {
				cols = append(cols, modelScore(row, m))
			}
		}
		cols = append(cols,
			fmt.Sprintf("%.0f%%", row.Coverage),
			fmt.Sprintf("%g/5", row.Vibe),
			dash(strings.Join(row.MissingEssentials, ", ")),
			price(row.Listing),
			beds(row.Listing),
			fmt.Sprintf("[%s](%s) (%s)", mdEscape(address(row.Listing)), row.Listing.URL, row.Listing.Source),
			mdEscape(row.Summary),
		)
		fmt.Fprintf(w, "| %s |\n", strings.Join(cols, " | "))
	}
	fmt.Fprintln(w)
	return footer(w, r)
}

func JSON(w io.Writer, r Report) error {
	type jsonRow struct {
		Rank              int                           `json:"rank"`
		Score             float64                       `json:"score"`
		Coverage          float64                       `json:"coverage"`
		Vibe              float64                       `json:"vibe"`
		MissingEssentials []string                      `json:"missing_essentials,omitempty"`
		AvoidsHit         []string                      `json:"avoids_hit,omitempty"`
		Stale             bool                          `json:"stale,omitempty"`
		Source            listing.Source                `json:"source"`
		SourceID          string                        `json:"source_id"`
		URL               string                        `json:"url"`
		Address           string                        `json:"address"`
		Price             listing.Money                 `json:"price"`
		Beds              *int                          `json:"beds,omitempty"`
		Summary           string                        `json:"summary"`
		ByModel           map[string]listing.Assessment `json:"by_model"`
	}
	rows := make([]jsonRow, len(r.Rows))
	for i, row := range r.Rows {
		rows[i] = jsonRow{
			Rank: row.Rank, Score: row.Score, Coverage: row.Coverage, Vibe: row.Vibe,
			MissingEssentials: row.MissingEssentials, AvoidsHit: row.AvoidsHit, Stale: row.Stale,
			Source: row.Listing.Source, SourceID: row.Listing.SourceID, URL: row.Listing.URL,
			Address: address(row.Listing), Price: row.Listing.Price, Beds: row.Listing.Beds,
			Summary: row.Summary, ByModel: row.ByModel,
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		Profile string    `json:"profile"`
		Models  []string  `json:"models"`
		Rows    []jsonRow `json:"rows"`
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

func mdEscape(s string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(s)
}

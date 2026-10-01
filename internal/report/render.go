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

type cell struct {
	Text string
	Sort string
}

type column struct {
	Label string
	Cell  func(Row) cell
}

func (r Report) scoreColumns() []column {
	var cols []column
	switch {
	case len(r.Templates) > 1:
		for _, id := range r.ProfileIDs() {
			cols = append(cols, column{Label: id, Cell: func(row Row) cell {
				if g, ok := row.Grades[id]; ok {
					return cell{match(g), num(g.Match)}
				}
				return cell{"-", ""}
			}})
		}
	case len(r.Models) > 1:
		for _, m := range r.Models {
			cols = append(cols, column{Label: shortModel(m), Cell: func(row Row) cell {
				if a, ok := row.ByModel[m]; ok {
					return cell{fmt.Sprintf("%.1f", a.Score), num(a.Score)}
				}
				return cell{"-", ""}
			}})
		}
	}
	return cols
}

func Table(w io.Writer, r Report) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	extra := r.scoreColumns()
	header := []string{"#", "MATCH", "SCORE"}
	if len(r.Templates) > 1 {
		header = append(header, "PROFILE")
	}
	for _, c := range extra {
		header = append(header, strings.ToUpper(c.Label))
	}
	header = append(header, "COV", "VIBE", "MISSING", "DEALBREAKERS", "PRICE", "BEDS", "SOURCE", "ADDRESS", "URL")
	fmt.Fprintln(tw, strings.Join(header, "\t"))

	for _, row := range r.Rows {
		cols := []string{strconv.Itoa(row.Rank), match(row.Grade), fmt.Sprintf("%.1f", row.Score)}
		if len(r.Templates) > 1 {
			cols = append(cols, row.Profile)
		}
		for _, c := range extra {
			cols = append(cols, c.Cell(row).Text)
		}
		cols = append(cols,
			fmt.Sprintf("%.0f%%", row.Coverage),
			fmt.Sprintf("%g", row.Vibe),
			dash(strings.Join(row.MissingEssentials, ",")),
			dash(strings.Join(row.Dealbreakers, ",")),
			price(row.Listing),
			beds(row.Listing),
			sources(row),
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
		Profiles     []string `json:"profiles"`
		Models       []string `json:"models"`
		Uncalibrated []string `json:"uncalibrated,omitempty"`
		Rows         []Row    `json:"rows"`
	}{r.ProfileIDs(), r.Models, r.Uncalibrated, rows})
}

func footer(w io.Writer, r Report) error {
	for _, n := range r.notes() {
		if _, err := fmt.Fprintf(w, "\n%s\n", n); err != nil {
			return err
		}
	}
	return nil
}

func (r Report) notes() []string {
	var out []string
	if r.Stale > 0 {
		out = append(out, fmt.Sprintf("%d listing(s) omitted because their assessment is stale (profile or listing changed); rerun `housing assess` or pass --include-stale.", r.Stale))
	}
	if r.Dealbreakers > 0 {
		out = append(out, fmt.Sprintf("%d listing(s) hidden because they hit a dealbreaker; pass --include-dealbreakers to show them.", r.Dealbreakers))
	}
	if len(r.Uncalibrated) > 0 {
		out = append(out, fmt.Sprintf("Match equals the raw score for %s: its reference listing has not been graded; rerun `housing assess`.", strings.Join(r.Uncalibrated, ", ")))
	}
	return out
}

func match(g Grade) string {
	s := fmt.Sprintf("%.0f%%", g.Match)
	if !g.Calibrated {
		s = fmt.Sprintf("%.1f", g.Match)
	}
	if g.Stale {
		s += "*"
	}
	return s
}

func num(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
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

func sources(row Row) string {
	names := []string{string(row.Listing.Source)}
	for _, d := range row.AlsoListed {
		names = append(names, string(d.Source))
	}
	return strings.Join(names, "+")
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

func AddressLine(l listing.Listing) string {
	return address(l)
}

package report

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"strconv"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

//go:embed report.html.tmpl
var htmlSource string

var htmlTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"join":    strings.Join,
	"match":   match,
	"num":     num,
	"price":   price,
	"beds":    beds,
	"address": address,
	"refScore": func(t Template, model string) string {
		ref, ok := profile.ReferenceScore(t.Profile, t.References, model)
		if !ok {
			return ""
		}
		return fmt.Sprintf("%s %.1f", shortModel(model), ref)
	},
	"bedsSort": func(l listing.Listing) string {
		if l.Beds == nil {
			return ""
		}
		return strconv.Itoa(*l.Beds)
	},
}).Parse(htmlSource))

func HTML(w io.Writer, r Report) error {
	title := "Listings"
	if len(r.Templates) == 1 {
		p := r.Templates[0].Profile
		title = cmpStr(p.Name, p.ID)
	}
	if r.Day != "" {
		title += " · " + r.Day
	}
	return htmlTemplate.Execute(w, struct {
		Report
		Title   string
		Columns []column
		Notes   []string
	}{r, title, r.scoreColumns(), r.notes()})
}

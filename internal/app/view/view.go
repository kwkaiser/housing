package view

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

func Funcs() template.FuncMap {
	return template.FuncMap{
		"money":     Money,
		"beds":      Beds,
		"match":     Match,
		"thousands": Thousands,
		"fixed1":    Fixed1,
		"percent":   Percent,
		"model":     ShortModel,
		"ints":      Ints,
		"join":      strings.Join,
		"address":   report.AddressLine,
		"baths":     Baths,
		"sqft":      SqFt,
		"cost":      Cost,
	}
}

func Money(cents int64, offer listing.OfferType) string {
	if cents == 0 {
		return "-"
	}
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	s := sign + "$" + Thousands((cents+50)/100)
	if offer == listing.OfferRent {
		s += "/mo"
	}
	return s
}

func Thousands(n int64) string {
	if n < 0 {
		return "-" + Thousands(-n)
	}
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func Beds(beds *int) string {
	switch {
	case beds == nil:
		return "-"
	case *beds == 0:
		return "studio"
	}
	return strconv.Itoa(*beds)
}

func Match(match float64, calibrated, stale bool) string {
	s := fmt.Sprintf("%.0f%%", match)
	if !calibrated {
		s = fmt.Sprintf("%.1f", match)
	}
	if stale {
		s += "*"
	}
	return s
}

func Fixed1(f float64) string {
	return strconv.FormatFloat(f, 'f', 1, 64)
}

func Percent(f float64) string {
	return fmt.Sprintf("%.0f%%", f)
}

func ShortModel(m string) string {
	return m[strings.LastIndex(m, "/")+1:]
}

func Ints(ns []int) string {
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ", ")
}

func Baths(baths *float64) string {
	if baths == nil {
		return "-"
	}
	return strconv.FormatFloat(*baths, 'f', -1, 64)
}

func SqFt(sqft *int) string {
	if sqft == nil {
		return "-"
	}
	return Thousands(int64(*sqft))
}

func Ago(t, now time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m ago"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d/time.Hour)) + "h ago"
	}
	return strconv.Itoa(int(d/(24*time.Hour))) + "d ago"
}

func Elapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d/time.Second)) + "s"
	case d < time.Hour:
		return fmt.Sprintf("%dm %02ds", d/time.Minute, d%time.Minute/time.Second)
	}
	return fmt.Sprintf("%dh %02dm", d/time.Hour, d%time.Hour/time.Minute)
}

func Cost(usd float64) string {
	switch {
	case usd == 0:
		return "-"
	case usd < 1:
		return fmt.Sprintf("$%.3f", usd)
	}
	return fmt.Sprintf("$%.2f", usd)
}

func Truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimRight(string(r[:n]), " ") + "…"
}

func StatusClass(status string) string {
	switch status {
	case "queued", "running", "succeeded", "degraded", "failed", "cancelled":
		return "status " + status
	}
	return "status"
}

func KindLabel(kind string) string {
	switch kind {
	case "run_collection":
		return "Collection run"
	case "create_profile":
		return "Create profile"
	case "draft_profile":
		return "Draft profile"
	}
	return strings.ReplaceAll(kind, "_", " ")
}

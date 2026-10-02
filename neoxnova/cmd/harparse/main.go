// Command harparse extracts a sanitized game-data fixture from a niburuspace.com
// HAR capture. It emits only names, codes, costs, durations and production
// values — no headers, cookies or other session data.
//
// Usage:
//
//	go run ./cmd/harparse -in "<path to .har>" -out testdata/niburus_catalog.json
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type harFile struct {
	Log struct {
		Entries []harEntry `json:"entries"`
	} `json:"log"`
}

type harEntry struct {
	Request struct {
		URL    string `json:"url"`
		Method string `json:"method"`
	} `json:"request"`
	Response struct {
		Status  int `json:"status"`
		Content struct {
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
			Encoding string `json:"encoding"`
		} `json:"content"`
	} `json:"response"`
}

// Money is a resource cost.
type Money struct {
	Metal     int64 `json:"metal"`
	Crystal   int64 `json:"crystal"`
	Deuterium int64 `json:"deuterium"`
}

// Item is a buildable structure/tech/ship/defense with its captured next level.
type Item struct {
	Code        int    `json:"code"`
	Name        string `json:"name"`
	Level       int    `json:"level,omitempty"`
	Cost        Money  `json:"next_cost"`
	DurationSec int64  `json:"duration_seconds,omitempty"`
	Kind        string `json:"kind"`
}

// Production is one row of the resources page.
type Production struct {
	Label     string  `json:"label"`
	Metal     float64 `json:"metal"`
	Crystal   float64 `json:"crystal"`
	Deuterium float64 `json:"deuterium"`
	Energy    float64 `json:"energy"`
}

type Catalog struct {
	Variant    string       `json:"variant"`
	Structures []Item       `json:"structures"`
	Techs      []Item       `json:"techs"`
	Ships      []Item       `json:"ships"`
	Defenses   []Item       `json:"defenses"`
	Production []Production `json:"production"`
}

var (
	reCode    = regexp.MustCompile(`(?:id="[sd]_|Dialog\.info\()(\d+)`)
	reTitle   = regexp.MustCompile(`(?s)class="title">\s*(.*?)\s*</a>`)
	reTags    = regexp.MustCompile(`<[^>]+>`)
	reLevel   = regexp.MustCompile(`(?i)^(.*?)\s+(\d+)\s+Level$`)
	rePrice   = regexp.MustCompile(`(?s)class="price res90([123])[^"]*".*?class="text[^"]*"[^>]*>\s*([\d.]+)\s*</div>`)
	reDur     = regexp.MustCompile(`Duration:\s*<span>([^<]+)</span>`)
	reDurPart = regexp.MustCompile(`(?:(\d+)d)?\s*(?:(\d+)h)?\s*(?:(\d+)m)?\s*(?:(\d+)s)?`)
	reRow     = regexp.MustCompile(`(?s)<tr[^>]*>\s*<td>(.*?)</td>((?:\s*<td[^>]*>.*?</td>)+)`)
	reCell    = regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)
)

func decodeText(e harEntry) string {
	if e.Response.Content.Encoding == "base64" {
		b, err := base64.StdEncoding.DecodeString(e.Response.Content.Text)
		if err != nil {
			return ""
		}
		return string(b)
	}
	return e.Response.Content.Text
}

func stripTags(s string) string {
	return strings.TrimSpace(reTags.ReplaceAllString(s, ""))
}

func parseNum(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func parseDuration(s string) int64 {
	m := reDurPart.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	toInt := func(i int) int64 { n, _ := strconv.ParseInt(m[i], 10, 64); return n }
	return toInt(1)*86400 + toInt(2)*3600 + toInt(3)*60 + toInt(4)
}

// parseItems splits the page into build_box chunks and extracts each item.
func parseItems(html, kind string) []Item {
	var out []Item
	parts := strings.Split(html, `class="build_box`)
	for _, ch := range parts[1:] {
		codeM := reCode.FindStringSubmatch(ch)
		titleM := reTitle.FindStringSubmatch(ch)
		if codeM == nil || titleM == nil {
			continue
		}
		code, _ := strconv.Atoi(codeM[1])
		title := stripTags(titleM[1])
		name, level := title, 0
		if lm := reLevel.FindStringSubmatch(title); lm != nil {
			name = strings.TrimSpace(lm[1])
			level, _ = strconv.Atoi(lm[2])
		}
		item := Item{Code: code, Name: name, Level: level, Kind: kind}
		for _, pm := range rePrice.FindAllStringSubmatch(ch, -1) {
			val := parseNum(pm[2])
			switch pm[1] {
			case "1":
				item.Cost.Metal = val
			case "2":
				item.Cost.Crystal = val
			case "3":
				item.Cost.Deuterium = val
			}
		}
		if dm := reDur.FindStringSubmatch(ch); dm != nil {
			item.DurationSec = parseDuration(dm[1])
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func parseProduction(html string) []Production {
	var out []Production
	for _, rm := range reRow.FindAllStringSubmatch(html, -1) {
		label := stripTags(rm[1])
		if label == "" {
			continue
		}
		cells := reCell.FindAllStringSubmatch(rm[2], -1)
		vals := make([]float64, 0, 4)
		for _, c := range cells {
			vals = append(vals, parseFloat(stripTags(c[1])))
		}
		for len(vals) < 4 {
			vals = append(vals, 0)
		}
		p := Production{Label: label, Metal: vals[0], Crystal: vals[1], Deuterium: vals[2], Energy: vals[3]}
		// Only keep rows that look like production entries.
		if strings.Contains(strings.ToLower(label), "production") ||
			strings.Contains(label, "Level)") || strings.Contains(strings.ToLower(label), "mine") {
			out = append(out, p)
		}
	}
	return out
}

func parseFloat(s string) float64 {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.TrimSuffix(s, "%")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func main() {
	in := flag.String("in", "", "path to the .har capture")
	out := flag.String("out", "testdata/niburus_catalog.json", "output JSON path")
	flag.Parse()
	if *in == "" {
		log.Fatal("missing -in <path to .har>")
	}

	raw, err := os.ReadFile(*in)
	if err != nil {
		log.Fatalf("read har: %v", err)
	}
	var h harFile
	if err := json.Unmarshal(raw, &h); err != nil {
		log.Fatalf("parse har: %v", err)
	}
	log.Printf("HAR entries: %d", len(h.Log.Entries))

	cat := Catalog{Variant: "XNova (GOW theme) - niburuspace.com"}
	for _, e := range h.Log.Entries {
		if e.Response.Status != 200 {
			continue
		}
		u := e.Request.URL
		body := func() string { return decodeText(e) }
		switch {
		case regexp.MustCompile(`page=buildings$`).MatchString(u):
			if len(cat.Structures) == 0 {
				cat.Structures = parseItems(body(), "structure")
			}
		case regexp.MustCompile(`page=research$`).MatchString(u):
			if len(cat.Techs) == 0 {
				cat.Techs = parseItems(body(), "tech")
			}
		case regexp.MustCompile(`page=shipyard&mode=fleet`).MatchString(u):
			if len(cat.Ships) == 0 {
				cat.Ships = parseItems(body(), "ship")
			}
		case regexp.MustCompile(`page=shipyard&mode=defense`).MatchString(u):
			if len(cat.Defenses) == 0 {
				cat.Defenses = parseItems(body(), "defense")
			}
		case regexp.MustCompile(`page=resources$`).MatchString(u):
			if len(cat.Production) == 0 {
				cat.Production = parseProduction(body())
			}
		}
	}

	buf, _ := json.MarshalIndent(cat, "", "  ")
	if err := os.WriteFile(*out, buf, 0o644); err != nil {
		log.Fatalf("write fixture: %v", err)
	}
	fmt.Printf("wrote %s: %d structures, %d techs, %d ships, %d defenses, %d production rows\n",
		*out, len(cat.Structures), len(cat.Techs), len(cat.Ships), len(cat.Defenses), len(cat.Production))
}

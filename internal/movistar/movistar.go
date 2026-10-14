// Package movistar reads Movistar Plus+'s own programme guide: the channel
// list from its public TV-guide page and each channel's airings from the JSON
// guide its web player uses. Unlike the XMLTV guides built from it, the JSON
// says per airing whether it is live (Directo) or a first showing (Estreno):
// a live match and its replays share the same title, description and even
// the same programme id, so that flag is the only way to tell them apart.
package movistar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	guidePage = "https://www.movistarplus.es/programacion-tv"
	epgURL    = "https://ottcache.dof6.com/movistarplus/webplayer/OTT/epg"
	userAgent = "Mozilla/5.0 (compatible; krinkuto11-epg; +https://github.com/krinkuto11/epg)"
)

// Madrid is the guide's time zone: a day's request covers that local day.
var Madrid = mustZone("Europe/Madrid")

func mustZone(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

// Channel is one channel of the guide: its name as Movistar shows it and the
// code its JSON guide is asked for.
type Channel struct {
	Name string
	Code string
}

// Airing is one programme airing.
type Airing struct {
	Start    time.Time
	End      time.Time
	Title    string // series or competition, e.g. "UEFA Champions League"
	Episode  string // e.g. "Roma - Real Madrid"
	Genre    string
	Live     bool // Directo
	Premiere bool // Estreno
}

// Client fetches the guide.
type Client struct {
	HTTP *http.Client
}

func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return body, nil
}

// channelRe matches the TV-guide page's schema.org channel list:
// "name": "M+ Liga de Campeones", "url": ".../programacion-tv/chapio".
var channelRe = regexp.MustCompile(`"name":\s*"([^"]+)",\s*"url":\s*"https://www\.movistarplus\.es/programacion-tv/([^"/?]+)"`)

// Channels lists the channels on the guide page.
func (c *Client) Channels(ctx context.Context) ([]Channel, error) {
	body, err := c.get(ctx, guidePage)
	if err != nil {
		return nil, err
	}
	return ParseChannels(body), nil
}

// ParseChannels reads the channel list from the guide page.
func ParseChannels(page []byte) []Channel {
	seen := map[string]bool{}
	var out []Channel
	for _, m := range channelRe.FindAllSubmatch(page, -1) {
		code := strings.ToUpper(string(m[2]))
		if seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, Channel{Name: strings.TrimSpace(string(m[1])), Code: code})
	}
	return out
}

// Day fetches one channel's airings for one local day.
func (c *Client) Day(ctx context.Context, code string, day time.Time) ([]Airing, error) {
	q := url.Values{
		"from":        {day.In(Madrid).Format("2006-01-02") + "T00:00:00"},
		"span":        {"1"},
		"channel":     {code},
		"version":     {"8"},
		"mdrm":        {"true"},
		"tlsstream":   {"true"},
		"demarcation": {"1"},
	}
	body, err := c.get(ctx, epgURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	return ParseDay(body)
}

type rawAiring struct {
	Start    json.RawMessage `json:"FechaHoraInicio"`
	End      json.RawMessage `json:"FechaHoraFin"`
	Title    string          `json:"TituloSerie"`
	Titulo   string          `json:"Titulo"`
	Episode  string          `json:"TituloEpisodio"`
	Genre    string          `json:"GeneroComAntena"`
	Live     bool            `json:"Directo"`
	Premiere bool            `json:"Estreno"`
}

// ParseDay reads a day of the JSON guide.
func ParseDay(body []byte) ([]Airing, error) {
	var raw []rawAiring
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("guide: %w", err)
	}
	out := make([]Airing, 0, len(raw))
	for _, r := range raw {
		start, err1 := millis(r.Start)
		end, err2 := millis(r.End)
		if err1 != nil || err2 != nil {
			continue
		}
		a := Airing{Start: start, End: end, Title: r.Title, Episode: strings.Trim(r.Episode, `"`),
			Genre: r.Genre, Live: r.Live, Premiere: r.Premiere}
		if a.Title == "" {
			a.Title = r.Titulo
		}
		out = append(out, a)
	}
	return out, nil
}

// millis reads a time in Unix milliseconds, given as a string or a number.
func millis(m json.RawMessage) (time.Time, error) {
	s := strings.Trim(string(m), `"`)
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(n), nil
}

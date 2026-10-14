// Package overlay corrects a guide's live markers with Movistar's own data.
//
// The base guide (dobleM, built from Movistar among others) marks live
// programmes by prefixing the title with "DIRECTO", but the marker lands on
// the wrong airing: over three days of sports channels it was missing on 12
// live matches (Roma - Real Madrid, Euroleague games) and present on 11
// pre-match shows. Movistar's JSON guide flags each airing itself (Directo),
// and a live match and its replays share the same title, so that flag is the
// only reliable source. For every programme of a channel Movistar carries, the
// airing at the same start time decides: the title gets "DIRECTO " exactly
// when it is live, and the programme gets <live/> (which Dispatcharr reads as
// is_live) and <premiere/> for a first showing. Programmes without a Movistar
// airing at that time are left as they are.
package overlay

import (
	"regexp"
	"strings"
	"time"

	"github.com/krinkuto11/epg/internal/movistar"
	"github.com/krinkuto11/epg/internal/xmltv"
)

// LivePrefix is how the base guide marks a live programme's title.
const LivePrefix = "DIRECTO "

var prefixRe = regexp.MustCompile(`^DIRECTO\s+`)

// Norm reduces a channel name for matching guide channels to Movistar's:
// case, repeated spaces and a ".TV" suffix aside.
func Norm(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	return strings.TrimSuffix(s, ".tv")
}

// Map pairs the guide's channel ids with Movistar codes, by the channel id or
// any display name. overrides (guide id -> Movistar code) win.
func Map(chs []xmltv.Channel, mv []movistar.Channel, overrides map[string]string) map[string]string {
	byName := map[string]string{}
	for _, c := range mv {
		if k := Norm(c.Name); k != "" {
			if _, dup := byName[k]; !dup {
				byName[k] = c.Code
			}
		}
	}
	out := map[string]string{}
	for _, c := range chs {
		if code, ok := overrides[c.ID]; ok {
			if code != "" { // "" = never overlay this channel
				out[c.ID] = code
			}
			continue
		}
		for _, n := range append([]string{c.ID}, c.Names...) {
			if code, ok := byName[Norm(n)]; ok {
				out[c.ID] = code
				break
			}
		}
	}
	return out
}

// Day is one channel's airings by start minute.
type Day map[int64]movistar.Airing

// Index is the Movistar airings by channel code.
type Index map[string]Day

// Add files airings under their channel.
func (ix Index) Add(code string, as []movistar.Airing) {
	d := ix[code]
	if d == nil {
		d = Day{}
		ix[code] = d
	}
	for _, a := range as {
		d[a.Start.Unix()/60] = a
	}
}

// Stats counts what the overlay did.
type Stats struct {
	Programmes    int `json:"programmes"`    // in the guide
	OnMovistar    int `json:"onMovistar"`    // on a channel Movistar carries, within the days read
	Matched       int `json:"matched"`       // of those, with a Movistar airing at the same start
	Live          int `json:"live"`          // matched and live
	Premiere      int `json:"premiere"`      // matched and a first showing
	PrefixAdded   int `json:"prefixAdded"`   // live, the base guide had no DIRECTO
	PrefixRemoved int `json:"prefixRemoved"` // not live, the base guide said DIRECTO
}

// Apply corrects one programme. chans maps guide ids to Movistar codes, ix
// holds the airings fetched for the days in [from, to): a programme outside
// them is left alone.
func Apply(p *xmltv.Programme, chans map[string]string, ix Index, from, to time.Time, st *Stats) {
	st.Programmes++
	code, ok := chans[p.Channel()]
	if !ok {
		return
	}
	day := ix[code]
	if day == nil {
		return
	}
	start, err := p.Start()
	if err != nil || start.Before(from) || !start.Before(to) {
		return
	}
	st.OnMovistar++
	a, ok := day[start.Unix()/60]
	if !ok {
		return
	}
	st.Matched++
	for i := range p.Titles {
		t := p.Titles[i].Value
		had := prefixRe.MatchString(t)
		bare := prefixRe.ReplaceAllString(t, "")
		switch {
		case a.Live && !had:
			p.Titles[i].Value = LivePrefix + bare
			if i == 0 {
				st.PrefixAdded++
			}
		case !a.Live && had:
			p.Titles[i].Value = bare
			if i == 0 {
				st.PrefixRemoved++
			}
		}
	}
	p.Unset("live")
	if a.Live {
		p.Set("live")
		st.Live++
	}
	if a.Premiere {
		p.Set("premiere")
		st.Premiere++
	}
}

// Days are the local days to fetch, today and the n-1 after it, and the
// window they cover.
func Days(now time.Time, n int) (days []time.Time, from, to time.Time) {
	t := now.In(movistar.Madrid)
	from = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, movistar.Madrid)
	for i := range n {
		days = append(days, from.AddDate(0, 0, i).Add(12*time.Hour))
	}
	return days, from, from.AddDate(0, 0, n)
}

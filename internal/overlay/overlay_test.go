package overlay

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/krinkuto11/epg/internal/movistar"
	"github.com/krinkuto11/epg/internal/xmltv"
)

// A slice of dobleM's guide on 2026-10-14: the base marks the pre-match show
// live and the match not; the replay the next day has the match's title.
const guide = `<?xml version="1.0" encoding="UTF-8"?>
<tv generator-info-name="dobleM 14/10/2026 08:47">
	<channel id="M+ Liga de Campeones HD">
		<display-name>M+ Liga de Campeones</display-name>
		<display-name>M+ Liga de Campeones.TV</display-name>
	</channel>
	<channel id="Other">
		<display-name>Other</display-name>
	</channel>
	<programme start="20261014193000 +0200" stop="20261014205300 +0200" channel="M+ Liga de Campeones HD">
		<title>DIRECTO 14/10/2026 T26/27 · La Previa de Champions</title>
		<desc>Deportes | Programa &amp; previa</desc>
	</programme>
	<programme start="20261014205300 +0200" stop="20261014230000 +0200" channel="M+ Liga de Campeones HD">
		<title>Jornada 2: Roma - Real Madrid T26/27 · UEFA Champions League</title>
		<desc>Deportes,Fútbol | 2026</desc>
	</programme>
	<programme start="20261015145000 +0200" stop="20261015164000 +0200" channel="M+ Liga de Campeones HD">
		<title>Jornada 2: Roma - Real Madrid T26/27 · UEFA Champions League</title>
		<desc>Deportes,Fútbol | 2026</desc>
	</programme>
	<programme start="20261014205300 +0200" stop="20261014230000 +0200" channel="Other">
		<title>DIRECTO Something</title>
		<desc>untouched</desc>
	</programme>
</tv>
`

func at(h, m, day int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, movistar.Madrid) }

func run(t *testing.T) (string, Stats) {
	t.Helper()
	chs, err := xmltv.Channels([]byte(guide))
	if err != nil {
		t.Fatal(err)
	}
	chans := Map(chs, []movistar.Channel{{Name: "M+ Liga de Campeones", Code: "CHAPIO"}, {Name: "Other TV", Code: "OTH"}}, nil)
	ix := Index{}
	ix.Add("CHAPIO", []movistar.Airing{
		{Start: at(19, 30, 14), Premiere: true},
		{Start: at(20, 53, 14), Live: true},
		{Start: at(14, 50, 15)},
	})
	var st Stats
	var out bytes.Buffer
	if err := xmltv.Rewrite([]byte(guide), &out, func(p *xmltv.Programme) {
		Apply(p, chans, ix, at(0, 0, 14), at(0, 0, 17), &st)
	}); err != nil {
		t.Fatal(err)
	}
	return out.String(), st
}

// programme returns the programme starting at start on channel.
func programme(t *testing.T, out, start, channel string) string {
	t.Helper()
	for _, p := range strings.Split(out, "<programme ")[1:] {
		if strings.Contains(p, `start="`+start+`"`) && strings.Contains(p, `channel="`+channel+`"`) {
			return p
		}
	}
	t.Fatalf("no programme %s on %s in\n%s", start, channel, out)
	return ""
}

func TestLiveFollowsMovistar(t *testing.T) {
	out, st := run(t)
	ch := "M+ Liga de Campeones HD"

	match := programme(t, out, "20261014205300 +0200", ch)
	if !strings.Contains(match, "<title>DIRECTO Jornada 2: Roma - Real Madrid") || !strings.Contains(match, "<live></live>") {
		t.Errorf("live match not marked:\n%s", match)
	}
	replay := programme(t, out, "20261015145000 +0200", ch)
	if strings.Contains(replay, "DIRECTO") || strings.Contains(replay, "<live>") {
		t.Errorf("replay marked live:\n%s", replay)
	}
	show := programme(t, out, "20261014193000 +0200", ch)
	if strings.Contains(show, "DIRECTO") || !strings.Contains(show, "<premiere></premiere>") {
		t.Errorf("pre-match show:\n%s", show)
	}
	if !strings.Contains(show, "Programa &amp; previa") {
		t.Errorf("description not kept verbatim:\n%s", show)
	}
	other := programme(t, out, "20261014205300 +0200", "Other")
	if !strings.Contains(other, "<title>DIRECTO Something</title>") || strings.Contains(other, "<live>") {
		t.Errorf("a channel Movistar does not carry was changed:\n%s", other)
	}
	want := Stats{Programmes: 4, OnMovistar: 3, Matched: 3, Live: 1, Premiere: 1, PrefixAdded: 1, PrefixRemoved: 1}
	if st != want {
		t.Errorf("stats %+v, want %+v", st, want)
	}
}

func TestOverridesWin(t *testing.T) {
	chs := []xmltv.Channel{{ID: "A HD", Names: []string{"A"}}, {ID: "B", Names: []string{"B"}}}
	mv := []movistar.Channel{{Name: "A", Code: "AA"}, {Name: "B", Code: "BB"}}
	got := Map(chs, mv, map[string]string{"A HD": "", "B": "XX"})
	if len(got) != 1 || got["B"] != "XX" {
		t.Fatalf("got %v", got)
	}
	if got := Map(chs, mv, nil); got["A HD"] != "AA" || got["B"] != "BB" {
		t.Fatalf("by display name: %v", got)
	}
}

func TestDaysCoverLocalDays(t *testing.T) {
	days, from, to := Days(time.Date(2026, 10, 14, 23, 30, 0, 0, time.UTC), 3) // 01:30 on the 15th in Madrid
	if from != at(0, 0, 15) || to != at(0, 0, 18) || len(days) != 3 || days[0].Day() != 15 {
		t.Fatalf("days %v from %v to %v", days, from, to)
	}
}

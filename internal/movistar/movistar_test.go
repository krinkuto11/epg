package movistar

import (
	"os"
	"testing"
	"time"
)

func TestParseChannels(t *testing.T) {
	page := []byte(`{"@type": "TelevisionChannel",
                "name": "M+ Liga de Campeones",
                "url": "https://www.movistarplus.es/programacion-tv/chapio"
            }, {"@type": "TelevisionChannel",
                "name": "M+ Liga de Campeones  3",
                "url": "https://www.movistarplus.es/programacion-tv/chap2"
            }, {"name": "M+ Liga de Campeones",
                "url": "https://www.movistarplus.es/programacion-tv/chapio"},
            {"name": "M+ Golf", "url": "https://www.movistarplus.es/programacion-tv/golf+"}`)
	got := ParseChannels(page)
	want := []Channel{{"M+ Liga de Campeones", "CHAPIO"}, {"M+ Liga de Campeones  3", "CHAP2"}, {"M+ Golf", "GOLF+"}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %v, want %v", i, got[i], want[i])
		}
	}
}

// The fixture is Movistar's guide for M+ Liga de Campeones on 2026-10-14,
// trimmed: Roma - Real Madrid live at 20:53 and its replay the next day share
// the title; only Directo differs.
func TestParseDayTellsLiveFromReplay(t *testing.T) {
	b, err := os.ReadFile("testdata/chapio.json")
	if err != nil {
		t.Fatal(err)
	}
	as, err := ParseDay(b)
	if err != nil {
		t.Fatal(err)
	}
	live := time.Date(2026, 10, 14, 20, 53, 0, 0, Madrid)
	var seenLive, seenReplay bool
	for _, a := range as {
		if a.Episode != "Roma - Real Madrid" || a.Title != "UEFA Champions League" {
			continue
		}
		if a.Start.Equal(live) {
			seenLive = a.Live
		} else if !a.Live {
			seenReplay = true
		}
	}
	if !seenLive || !seenReplay {
		t.Fatalf("live %v replay %v in %+v", seenLive, seenReplay, as)
	}
}

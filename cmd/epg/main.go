// Command epg builds the guide: dobleM's guiatv.xml with the live markers
// corrected from Movistar's own guide (see internal/overlay). It writes
// guiatv.xml, guiatv.xml.gz and status.json to -out.
//
// When Movistar cannot be read, the base guide is written unchanged and
// status.json says why: the guide is never worse than its base.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/krinkuto11/epg/internal/movistar"
	"github.com/krinkuto11/epg/internal/overlay"
	"github.com/krinkuto11/epg/internal/xmltv"
)

const defaultBase = "https://raw.githubusercontent.com/davidmuma/EPG_dobleM/master/guiatv.xml"

// Status is status.json: what this build used and changed.
type Status struct {
	Built       time.Time         `json:"built"`
	Base        string            `json:"base"`
	BaseInfo    string            `json:"baseGenerator,omitempty"`
	Movistar    bool              `json:"movistar"` // the overlay ran
	Error       string            `json:"error,omitempty"`
	Channels    int               `json:"channels"`         // in the guide
	Mapped      map[string]string `json:"mapped,omitempty"` // guide id -> Movistar code
	FetchFailed []string          `json:"fetchFailed,omitempty"`
	overlay.Stats
}

func main() {
	base := flag.String("base", defaultBase, "base XMLTV guide (URL or file)")
	out := flag.String("out", "dist", "output directory")
	days := flag.Int("days", 3, "days of Movistar's guide to read, from today")
	workers := flag.Int("workers", 6, "concurrent Movistar requests")
	overridesPath := flag.String("overrides", "channels.json", "guide id -> Movistar code overrides (\"\" = never overlay); optional")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	hc := &http.Client{Timeout: 2 * time.Minute}

	guide, err := read(ctx, hc, *base)
	if err != nil {
		log.Fatalf("base guide: %v", err)
	}
	st := Status{Built: time.Now().UTC(), Base: *base, BaseInfo: generator(guide)}
	chs, err := xmltv.Channels(guide)
	if err != nil {
		log.Fatalf("base guide: %v", err)
	}
	st.Channels = len(chs)

	ix := overlay.Index{}
	dayList, from, to := overlay.Days(time.Now(), *days)
	mc := &movistar.Client{HTTP: &http.Client{Timeout: 30 * time.Second}}
	mv, err := mc.Channels(ctx)
	if err == nil && len(mv) == 0 {
		err = fmt.Errorf("movistar: no channels on the guide page")
	}
	if err != nil {
		st.Error = err.Error()
		log.Printf("movistar: %v; writing the base guide unchanged", err)
	} else {
		st.Mapped = overlay.Map(chs, mv, overrides(*overridesPath))
		st.FetchFailed = fetch(ctx, mc, st.Mapped, dayList, *workers, ix)
		st.Movistar = len(ix) > 0
		if !st.Movistar {
			st.Error = "movistar: every guide request failed"
		}
	}

	var buf bytes.Buffer
	if err := xmltv.Rewrite(guide, &buf, func(p *xmltv.Programme) { overlay.Apply(p, st.Mapped, ix, from, to, &st.Stats) }); err != nil {
		log.Fatalf("rewrite: %v", err)
	}
	if err := write(*out, buf.Bytes(), st); err != nil {
		log.Fatal(err)
	}
	log.Printf("%d channels (%d on Movistar), %d programmes: %d/%d matched, %d live (+%d / -%d DIRECTO), %d premieres; %d requests failed",
		st.Channels, len(st.Mapped), st.Programmes, st.Matched, st.OnMovistar, st.Live, st.PrefixAdded, st.PrefixRemoved, st.Premiere, len(st.FetchFailed))
}

func read(ctx context.Context, hc *http.Client, src string) ([]byte, error) {
	if !regexp.MustCompile(`^https?://`).MatchString(src) {
		return os.ReadFile(src)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", src, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

var generatorRe = regexp.MustCompile(`generator-info-name="([^"]*)"`)

func generator(guide []byte) string {
	if m := generatorRe.FindSubmatch(guide[:min(len(guide), 4096)]); m != nil {
		return string(m[1])
	}
	return ""
}

func overrides(path string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		log.Fatalf("%s: %v", path, err)
	}
	return m
}

// fetch reads every mapped channel's days into ix and returns the requests
// that failed ("CODE day").
func fetch(ctx context.Context, mc *movistar.Client, mapped map[string]string, days []time.Time, workers int, ix overlay.Index) []string {
	codes := map[string]bool{}
	for _, c := range mapped {
		codes[c] = true
	}
	type job struct {
		code string
		day  time.Time
	}
	jobs := make(chan job)
	var mu sync.Mutex
	var failed []string
	var wg sync.WaitGroup
	for range max(1, workers) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				as, err := mc.Day(ctx, j.code, j.day)
				mu.Lock()
				if err != nil {
					failed = append(failed, j.code+" "+j.day.Format("2006-01-02"))
				} else {
					ix.Add(j.code, as)
				}
				mu.Unlock()
			}
		}()
	}
	for code := range codes {
		for _, d := range days {
			jobs <- job{code, d}
		}
	}
	close(jobs)
	wg.Wait()
	sort.Strings(failed)
	return failed
}

func write(dir string, guide []byte, st Status) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "guiatv.xml"), guide, 0o644); err != nil {
		return err
	}
	var gz bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	zw.Write(guide)
	zw.Close()
	if err := os.WriteFile(filepath.Join(dir, "guiatv.xml.gz"), gz.Bytes(), 0o644); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(filepath.Join(dir, "status.json"), append(b, '\n'), 0o644)
}

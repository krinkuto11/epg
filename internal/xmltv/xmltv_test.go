package xmltv

import (
	"bytes"
	"strings"
	"testing"
)

func TestRewriteKeepsWhatItDoesNotTouch(t *testing.T) {
	in := `<?xml version="1.0" encoding="UTF-8"?>
<tv generator-info-name="x">
	<channel id="C"><display-name>C</display-name></channel>
	<programme start="20261014205300 +0200" stop="20261014230000 +0200" channel="C">
		<title lang="es">Fútbol &amp; más</title>
		<desc>a &lt; b</desc>
		<icon src="http://i/x.jpg"/>
	</programme>
</tv>
`
	var out bytes.Buffer
	if err := Rewrite([]byte(in), &out, func(p *Programme) {}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<?xml version="1.0" encoding="UTF-8"?>`, `generator-info-name="x"`,
		`<title lang="es">Fútbol &amp; más</title>`, `<desc>a &lt; b</desc>`, `<icon src="http://i/x.jpg"></icon>`,
		`channel="C"`, `<display-name>C</display-name>`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %s in\n%s", want, out.String())
		}
	}
	chs, err := Channels(out.Bytes())
	if err != nil || len(chs) != 1 || chs[0].ID != "C" {
		t.Fatalf("channels %v %v", chs, err)
	}
}

func TestParseTime(t *testing.T) {
	a, err1 := ParseTime("20261014205300 +0200")
	b, err2 := ParseTime("20261014185300")
	if err1 != nil || err2 != nil || !a.Equal(b) {
		t.Fatalf("%v %v %v %v", a, b, err1, err2)
	}
}

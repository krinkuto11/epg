// Package xmltv reads and rewrites an XMLTV guide as a stream: everything
// passes through unchanged except the programmes a caller edits, so a 34 MB
// guide is never held as a tree.
package xmltv

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

// Channel is a <channel>: its id and display names.
type Channel struct {
	ID    string   `xml:"id,attr"`
	Names []string `xml:"display-name"`
}

// Programme is a <programme>. Titles are decoded for editing; every other
// child is kept verbatim, in order, in Rest.
type Programme struct {
	XMLName xml.Name   `xml:"programme"`
	Attrs   []xml.Attr `xml:",any,attr"`
	Titles  []Text     `xml:"title"`
	Rest    []Element  `xml:",any"`
}

// Text is an element with text and an optional lang.
type Text struct {
	Lang  string `xml:"lang,attr,omitempty"`
	Value string `xml:",chardata"`
}

// Element is any child element, kept as is.
type Element struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Inner   []byte     `xml:",innerxml"`
}

// Attr returns an attribute's value.
func (p *Programme) Attr(name string) string {
	for _, a := range p.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// Channel is the programme's channel id.
func (p *Programme) Channel() string { return p.Attr("channel") }

// Start is the programme's start time.
func (p *Programme) Start() (time.Time, error) { return ParseTime(p.Attr("start")) }

// Has reports whether the programme has a child element with that name.
func (p *Programme) Has(name string) bool {
	for _, e := range p.Rest {
		if e.XMLName.Local == name {
			return true
		}
	}
	return false
}

// Set adds an empty flag element (<live/>, <premiere/>) if it is missing.
func (p *Programme) Set(name string) {
	if !p.Has(name) {
		p.Rest = append(p.Rest, Element{XMLName: xml.Name{Local: name}})
	}
}

// Unset removes a child element.
func (p *Programme) Unset(name string) {
	out := p.Rest[:0]
	for _, e := range p.Rest {
		if e.XMLName.Local != name {
			out = append(out, e)
		}
	}
	p.Rest = out
}

// ParseTime reads an XMLTV time: "20261014205300 +0200" (the offset optional,
// UTC when missing).
func ParseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 20 {
		return time.Parse("20060102150405 -0700", s[:20])
	}
	if len(s) >= 14 {
		return time.Parse("20060102150405", s[:14])
	}
	return time.Time{}, fmt.Errorf("xmltv: bad time %q", s)
}

// Channels lists a guide's channels.
func Channels(guide []byte) ([]Channel, error) {
	d := xml.NewDecoder(bytes.NewReader(guide))
	var out []Channel
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "channel":
			var c Channel
			if err := d.DecodeElement(&c, &se); err != nil {
				return nil, err
			}
			out = append(out, c)
		case "programme":
			return out, nil // channels come first
		}
	}
}

// Rewrite copies a guide to w, passing each programme through edit.
func Rewrite(guide []byte, w io.Writer, edit func(*Programme)) error {
	d := xml.NewDecoder(bytes.NewReader(guide))
	e := xml.NewEncoder(w)
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "programme" {
			var p Programme
			if err := d.DecodeElement(&p, &se); err != nil {
				return err
			}
			edit(&p)
			if err := e.Encode(&p); err != nil {
				return err
			}
			continue
		}
		if cd, ok := tok.(xml.CharData); ok && len(bytes.TrimSpace(cd)) == 0 {
			// Indentation between elements, written as is: the encoder
			// would escape every tab as &#x9;.
			if err := e.Flush(); err != nil {
				return err
			}
			if _, err := w.Write(cd); err != nil {
				return err
			}
			continue
		}
		if err := e.EncodeToken(xml.CopyToken(tok)); err != nil {
			return err
		}
	}
	return e.Flush()
}

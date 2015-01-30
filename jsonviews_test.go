package jsonviews

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNext(t *testing.T) {
	data := "  \t  \n  n7  \t  s   8 saf"
	expected := []struct {
		c rune
		n int
	}{
		{'n', 9},
		{'7', 1},
		{'s', 6},
		{'8', 4},
		{'s', 2},
		{'a', 1},
		{'f', 1},
	}
	r := bufio.NewReader(strings.NewReader(data))
	for i := range expected {
		c, s, err := next(r)
		if err != nil {
			t.Error(err)
			return
		}
		if expected[i].c != c {
			t.Errorf("expected '%c' got '%c'", c, r)
		}
		if expected[i].n != s {
			t.Errorf("expected to read '%d' bytes, read '%d'", expected[i].n, s)
		}
	}
	_, _, err := next(r)
	if err != io.EOF {
		t.Errorf("expected EOF error go '%v'", err)
	}
}

func TestPeek(t *testing.T) {
	data := "  \t  \n  n7  \t  s   8 saf"
	expected := []struct {
		c rune
		n int
	}{
		{'n', 9},
		{'7', 1},
		{'s', 6},
		{'8', 4},
		{'s', 2},
		{'a', 1},
		{'f', 1},
	}
	r := bufio.NewReader(strings.NewReader(data))
	for i := range expected {
		c, s, err := peek(r)
		if err != nil {
			t.Error(err)
			return
		}
		if expected[i].c != c {
			t.Errorf("expected '%c' got '%c'", expected[i].c, c)
		}
		if expected[i].n-utf8.RuneLen(c) != s {
			t.Errorf("expected to read '%d' bytes, read '%d'", expected[i].n, s)
		}
		rr, _, err := r.ReadRune()
		if err != nil {
			t.Error(err)
			continue
		}
		if c != rr {
			t.Errorf("expected '%c' got '%c'", c, rr)
		}
	}
	_, _, err := next(r)
	if err != io.EOF {
		t.Errorf("expected EOF error go '%v'", err)
	}
}

type ReadTest struct {
	data string
	out  string
	ok   bool
	n    int
}

var ReadTests = []ReadTest{
	{` "hello"    `, `"hello"`, true, len(` "hello"`)},
	{"", "", false, 0},
	{`" sfa`, "", false, 0},
}

func TestReadString(t *testing.T) {
	for _, test := range ReadTests {
		r := bufio.NewReader(strings.NewReader(test.data))
		out := bytes.NewBuffer([]byte{})
		w := bufio.NewWriter(out)
		v := &View{}
		n, err := v.readString(w, r)
		if !test.ok {
			if err == nil {
				t.Errorf("expected error for '%s'", test.data)
			}
			continue
		}
		if err != nil {
			t.Errorf("got error when processing '%s': %v", test.data, err)
		}
		if n != test.n {
			t.Errorf("expected to read '%d' bytes, read '%d'", test.n, n)
		}
		w.Flush()
		s := out.String()
		if test.out != s {
			t.Errorf("expected the output '%s', got '%s'", test.out, s)
		}
	}
}

func TestReadObject(t *testing.T) {
	data := `
{"menu": {
  "id": "file",
  "value": "File",
  "tf": null,
  "popup": {
    "menuitem": [
      {"value": "New", "onclick": "CreateNewDoc()"},
      {"value": "Open", "onclick": "OpenDoc()"},
      {"value": "Close", "onclick": "CloseDoc()"}
    ]
  }
}}`
	view := NewView(bytes.NewReader([]byte(data)))
	view.AddFilter(".menu.popup")
	view.AddFilter(".menu.id")
	view.AddFilter(".menu.tf")
	d := json.NewDecoder(view)
	s := struct {
		Menu struct {
			Id    string `json:"id"`
			Value string `json:"value"`
			Popup struct {
				MenuItem []struct {
					Value   string `json:"value"`
					Onclick string `json:"onclick"`
				} `json:"menuitem"`
			} `json:"popup"`
		} `json:"menu"`
	}{}
	if err := d.Decode(&s); err != nil {
		t.Error(err)
		return
	}
	menu := s.Menu
	if menu.Value != "" {
		t.Errorf("expected .menu.value to be filtered out: '%s'", menu.Value)
	}
	if menu.Id != "file" {
		t.Errorf(".menu.id corrupted")
	}
}

type ViewTest struct {
	Input   string
	Filters []string
	Output  string
	OK      bool
}

func (vt ViewTest) Run() error {
	v := NewView(strings.NewReader(vt.Input))
	for _, f := range vt.Filters {
		v.AddFilter(f)
	}
	out, err := ioutil.ReadAll(v)
	if err != nil {
		if vt.OK {
			return fmt.Errorf("test returned error: %v", err)
		}
		return nil
	}
	if !vt.OK {
		return fmt.Errorf("expected test to fail")
	}
	output := string(out)
	if output != vt.Output {
		return fmt.Errorf("expected '%s' got '%s'", vt.Output, output)
	}
	return nil
}

func TestViews(t *testing.T) {
	for _, vt := range ViewTests {
		if err := vt.Run(); err != nil {
			t.Error(err)
		}
	}
}

var ViewTests = []ViewTest{
	ViewTest{
		Input:   Example1,
		Filters: []string{".glossary.title"},
		Output:  `{"glossary":{"title":"example glossary"}}`,
		OK:      true,
	},
	ViewTest{
		Input:   Example1,
		Filters: []string{".glossary.title", ".glossary.GlossDiv.title"},
		Output:  `{"glossary":{"title":"example glossary","GlossDiv":{"title":"S"}}}`,
		OK:      true,
	},
	ViewTest{
		Input: Example1,
		Filters: []string{
			".glossary.GlossDiv.GlossList.GlossEntry.ID",
			".glossary.GlossDiv.GlossList.GlossEntry.Abbrev",
		},
		Output: `{"glossary":{"GlossDiv":{"GlossList":{"GlossEntry":{"ID":"SGML","Abbrev":"ISO 8879:1986"}}}}}`,
		OK:     true,
	},
	ViewTest{
		Input: Example1,
		Filters: []string{
			".glossary.GlossDiv.GlossList.GlossEntry.ID",
			".glossary.GlossDiv.GlossList.GlossEntry.Abbrev",
			".glossary.GlossDiv.GlossList.UnknownEntry.Abbrev",
		},
		Output: `{"glossary":{"GlossDiv":{"GlossList":{"GlossEntry":{"ID":"SGML","Abbrev":"ISO 8879:1986"}}}}}`,
		OK:     true,
	},
	ViewTest{
		Input:   Example1,
		Filters: []string{},
		Output:  `{}`,
		OK:      true,
	},
	ViewTest{
		Input: Example1,
		Filters: []string{
			".glossary.GlossDiv.GlossList.GlossEntry.ID",
			".glossary.GlossDiv.GlossList.GlossEntry.Abbrev",
			".glossary.GlossDiv.GlossList.GlossEntry.GlossDef",
		},
		Output: `{"glossary":{"GlossDiv":{"GlossList":{"GlossEntry":{"ID":"SGML","Abbrev":"ISO 8879:1986","GlossDef":{"para":"A meta-markup language, used to create markup languages such as DocBook.","GlossSeeAlso":["GML","XML"]}}}}}}`,
		OK:     true,
	},
}

// Examples take from http://json.org/example
var (
	Example1 = `
{
    "glossary": {
        "title": "example glossary",
		"GlossDiv": {
            "title": "S",
			"GlossList": {
                "GlossEntry": {
                    "ID": "SGML",
					"SortAs": "SGML",
					"GlossTerm": "Standard Generalized Markup Language",
					"Acronym": "SGML",
					"Abbrev": "ISO 8879:1986",
					"GlossDef": {
                        "para": "A meta-markup language, used to create markup languages such as DocBook.",
						"GlossSeeAlso": ["GML", "XML"]
                    },
					"GlossSee": "markup"
                }
            }
        }
    }
}
`
	Example2 = `{"menu": {
  "id": "file",
  "value": "File",
  "popup": {
    "menuitem": [
      {"value": "New", "onclick": "CreateNewDoc()"},
      {"value": "Open", "onclick": "OpenDoc()"},
      {"value": "Close", "onclick": "CloseDoc()"}
    ]
  }
}}`
	Example3 = `{"widget": {
    "debug": "on",
    "window": {
        "title": "Sample Konfabulator Widget",
        "name": "main_window",
        "width": 500,
        "height": 500
    },
    "image": { 
        "src": "Images/Sun.png",
        "name": "sun1",
        "hOffset": 250,
        "vOffset": 250,
        "alignment": "center"
    },
    "text": {
        "data": "Click Here",
        "size": 36,
        "style": "bold",
        "name": "text1",
        "hOffset": 250,
        "vOffset": 100,
        "alignment": "center",
        "onMouseUp": "sun1.opacity = (sun1.opacity / 100) * 90;"
    }
}}`
	Example5 = `{"menu": {
    "header": "SVG Viewer",
    "items": [
        {"id": "Open"},
        {"id": "OpenNew", "label": "Open New"},
        null,
        {"id": "ZoomIn", "label": "Zoom In"},
        {"id": "ZoomOut", "label": "Zoom Out"},
        {"id": "OriginalView", "label": "Original View"},
        null,
        {"id": "Quality"},
        {"id": "Pause"},
        {"id": "Mute"},
        null,
        {"id": "Find", "label": "Find..."},
        {"id": "FindAgain", "label": "Find Again"},
        {"id": "Copy"},
        {"id": "CopyAgain", "label": "Copy Again"},
        {"id": "CopySVG", "label": "Copy SVG"},
        {"id": "ViewSVG", "label": "View SVG"},
        {"id": "ViewSource", "label": "View Source"},
        {"id": "SaveAs", "label": "Save As"},
        null,
        {"id": "Help"},
        {"id": "About", "label": "About Adobe CVG Viewer..."}
    ]
}}`
)

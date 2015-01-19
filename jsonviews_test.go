package jsonviews

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
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
	r := io.TeeReader(view, os.Stderr)
	d := json.NewDecoder(r)
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

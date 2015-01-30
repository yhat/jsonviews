package jsonviews

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"
)

type View struct {
	src     io.RuneScanner // src of JSON
	filters []string
	curr    string
	pr      io.Reader      // reads of the View read from this end of the pipe
	pw      *io.PipeWriter // decoding writes to this end concurrently
	once    *sync.Once
}

func NewView(r io.Reader) *View {
	v := &View{
		src:     bufio.NewReader(r),
		filters: []string{},
		once:    &sync.Once{},
	}
	v.pr, v.pw = io.Pipe()
	return v
}

func (v *View) Read(p []byte) (n int, err error) {
	v.once.Do(func() {
		go func() {
			w := bufio.NewWriter(v.pw)
			_, err := v.readJSON(w, v.src)
			w.Flush()
			if err != nil {
				v.pw.CloseWithError(err)
			}
		}()
	})
	return v.pr.Read(p)
}

func (v *View) AddFilter(filter string) {
	v.filters = append(v.filters, filter)
}

func (v *View) skip(curr string) bool {
	for _, filter := range v.filters {
		if filter == curr {
			return false
		}
		longer, shorter := filter, curr
		if len(longer) < len(shorter) {
			shorter, longer = longer, shorter
		}
		if !strings.HasPrefix(longer, shorter) {
			continue
		}
		// if the very next rune is a '.' don't skip
		if longer[len(shorter)] == '.' {
			return false
		}
	}
	return true
}

type runeWriter interface {
	WriteRune(r rune) (n int, err error)
}

var discard runeWriter = new(discardWriter)

type discardWriter struct{}

func (dw *discardWriter) WriteRune(r rune) (n int, err error) {
	return utf8.RuneLen(r), nil
}

type SyntaxError struct {
	Offset int
	msg    string
}

func (s *SyntaxError) Error() string { return s.msg }

func (v *View) readJSON(dest runeWriter, src io.RuneScanner) (n int, err error) {
	var r rune
	var nn int
	defer func() {
		if err != nil && err != io.EOF {
			err = &SyntaxError{
				Offset: n,
				msg:    err.Error(),
			}
		}
	}()
	r, n, err = peek(src)
	if err != nil {
		return
	}
	switch r {
	case '{':
		nn, err = v.readObject(dest, src)
		n += nn
		if err != nil {
			return
		}
	case '[':
		nn, err = v.readArray(dest, src)
		n += nn
		if err != nil {
			return
		}
	default:
		err = fmt.Errorf("expected '{' or '[' got '%c'", r)
		return
	}
	// read until EOF
	r, _, err = next(src)
	if err == nil {
		err = fmt.Errorf("expected EOF, got '%c'", r)
		return
	}
	return
}

func (v *View) readObject(dest runeWriter, src io.RuneScanner) (n int, err error) {
	var r rune
	var nn int
	r, n, err = next(src)
	if err != nil {
		return
	}
	if r != '{' {
		return n, fmt.Errorf("expected '{' got '%c'", r)
	}
	if _, err = dest.WriteRune(r); err != nil {
		return
	}
	defer func(dest runeWriter) {
		if err == nil {
			_, err = dest.WriteRune('}')
		}
	}(dest)
	curr := v.curr
	num := 0 // number of items actually written
	for {
		// some scoping to ensure v.curr and dest are refreshed for each loop
		v.curr = curr
		dest := dest
		// read the key and determine if is should be read
		keyBuf := bytes.NewBuffer([]byte{})
		nn, err = v.readString(keyBuf, src)
		n += nn
		if err != nil {
			return
		}
		key := keyBuf.String()
		// by the definitino of a JSON string "key" is guaranteed to be
		// surrounded by quotes
		v.curr = v.curr + "." + key[1:len(key)-1]
		if v.skip(v.curr) {
			dest = discard
		} else {
			num++
		}
		if num > 1 {
			if _, err = dest.WriteRune(','); err != nil {
				return
			}
		}
		for _, r = range []rune(key) {
			if _, err = dest.WriteRune(r); err != nil {
				return
			}
		}
		r, nn, err = next(src)
		n += nn
		if err != nil {
			return
		}
		if r != ':' {
			return n, fmt.Errorf("expected ':' got '%c'", r)
		}
		if _, err = dest.WriteRune(r); err != nil {
			return
		}
		nn, err = v.readValue(dest, src)
		n += nn
		if err != nil {
			return
		}
		r, nn, err = next(src)
		n += nn
		if err != nil {
			return
		}
		switch r {
		case '}':
			return
		case ',':
			continue
		default:
			return n, fmt.Errorf("expected ':' got '%c'", r)
		}
	}
}

func (v *View) readArray(dest runeWriter, src io.RuneScanner) (n int, err error) {
	r, n, err := next(src)
	if err != nil {
		return
	}
	if r != '[' {
		return n, fmt.Errorf("expected '[' got '%c'", r)
	}
	if _, err := dest.WriteRune(r); err != nil {
		return n, err
	}
	var nn int
	for {
		nn, err = v.readValue(dest, src)
		n += nn
		if err != nil {
			return
		}
		r, nn, err = next(src)
		if err != nil {
			return
		}
		n += nn
		switch r {
		case ',':
			if _, err = dest.WriteRune(r); err != nil {
				return
			}
			continue
		case ']':
			if _, err = dest.WriteRune(r); err != nil {
				return
			}
			return
		default:
			return n, fmt.Errorf("expected '[' or ',' got '%c'", r)
		}
	}
}

func (v *View) readValue(dest runeWriter, src io.RuneScanner) (n int, err error) {
	r, n, err := peek(src)
	if err != nil {
		return n, err
	}
	var nextSlice []rune
	switch r {
	case '"':
		nn, err := v.readString(dest, src)
		return n + nn, err
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		nn, err := v.readNumber(dest, src)
		return n + nn, err
	case '{':
		nn, err := v.readObject(dest, src)
		return n + nn, err
	case '[':
		nn, err := v.readArray(dest, src)
		return n + nn, err
	case 't':
		nextSlice = []rune("true")
	case 'f':
		nextSlice = []rune("false")
	case 'n':
		nextSlice = []rune("null")
	}
	for i := range nextSlice {
		rr, nn, err := src.ReadRune()
		if err != nil {
			return n, err
		}
		n += nn
		if nextSlice[i] != rr {
			// TODO: Clean up error message
			return n, fmt.Errorf("expected '%s'", string(nextSlice))
		}
		if _, err := dest.WriteRune(rr); err != nil {
			return n, err
		}
	}
	return
}

func (v *View) readString(dest runeWriter, src io.RuneScanner) (n int, err error) {
	r, n, err := next(src)
	if err != nil {
		return 0, err
	}
	if r != '"' {
		return n, fmt.Errorf(`expected '"' got '%c'`, r)
	}
	if _, err := dest.WriteRune(r); err != nil {
		return n, err
	}
	for {
		r, s, err := src.ReadRune()
		if err != nil {
			return n, err
		}
		n += s
		switch r {
		case '"':
			_, err := dest.WriteRune(r)
			return n, err
		case '\\':
			if _, err := dest.WriteRune(r); err != nil {
				return n, err
			}
			r, s, err := src.ReadRune()
			if err != nil {
				return n, err
			}
			n += s
			switch r {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				if _, err := dest.WriteRune(r); err != nil {
					return n, err
				}
			case 'u':
				if _, err := dest.WriteRune(r); err != nil {
					return n, err
				}
				for i := 0; i < 4; i++ {
					r, s, err := src.ReadRune()
					if err != nil {
						return n, err
					}
					n += s
					switch {
					case '0' <= r && r <= '9':
					case 'a' <= r && r <= 'f':
					case 'A' <= r && r <= 'F':
					default:
						msg := "illegal character after hex signifier: '%c'"
						return n, fmt.Errorf(msg, r)
					}
					if _, err := dest.WriteRune(r); err != nil {
						return n, err
					}
				}
			default:
				return n, fmt.Errorf("unexpected error after '/': '%c'", r)
			}
		default:
			if _, err := dest.WriteRune(r); err != nil {
				return n, err
			}
		}
	}
	return 0, nil
}

func (v *View) readNumber(dest runeWriter, src io.RuneScanner) (n int, err error) {
	var r rune
	var nn int
	if r, nn, err = next(src); err != nil {
		return nn, err
	}
	defer func() {
		// because this function reads the number until a rune not in the
		// definition of a number, it must always unread that rune
		if err != nil {
			err = src.UnreadRune()
		}
	}()
	// a helper function to read a series of digits
	readDigits := func() (rune, error) {
		for {
			r, nn, err = src.ReadRune()
			if err != nil {
				return r, err
			}
			switch {
			case '0' <= r && r <= '9':
				n += nn
				if _, err = dest.WriteRune(r); err != nil {
					return r, err
				}
			default:
				return r, nil
			}
		}
	}
	if r == '-' {
		n += nn
		if _, err := dest.WriteRune(r); err != nil {
			return n, err
		}
		r, nn, err = src.ReadRune()
		if err != nil {
			return n, err
		}
	}
	switch {
	case r == '0':
		if _, err = dest.WriteRune(r); err != nil {
			return n, err
		}
		n += nn
		r, nn, err = src.ReadRune()
		if err != nil {
			return n, err
		}
	case '1' <= r && r <= '9':
		if _, err = dest.WriteRune(r); err != nil {
			return n, err
		}
		n += nn
		r, err = readDigits()
		if err != nil {
			return n, err
		}
	}
	if r == '.' {
		if _, err = dest.WriteRune(r); err != nil {
			return n, err
		}
		n += nn
		r, err = readDigits()
		if err != nil {
			return n, err
		}
	}
	if r == 'e' || r == 'E' {
		n += nn
		if _, err = dest.WriteRune(r); err != nil {
			return n, err
		}
		r, nn, err = src.ReadRune()
		if err != nil {
			return n, err
		}
		if r == '+' || r == '-' {
			n += nn
			if _, err = dest.WriteRune(r); err != nil {
				return n, err
			}
			r, nn, err = src.ReadRune()
			if err != nil {
				return n, err
			}
		}
		_, err = readDigits()
	}
	return
}

func peek(r io.RuneScanner) (rune, int, error) {
	n := 0
	for {
		c, s, err := r.ReadRune()
		if err != nil {
			return c, n, err
		}
		n += s
		switch c {
		case ' ', '\t', '\n', '\r':
		default:
			n -= s
			err = r.UnreadRune()
			return c, n, err
		}
	}
}

// next returns the next non-space rune
func next(r io.RuneScanner) (rune, int, error) {
	n := 0
	for {
		c, s, err := r.ReadRune()
		if err != nil {
			return c, n, err
		}
		n += s
		switch c {
		case ' ', '\t', '\n', '\r':
		default:
			return c, n, nil
		}
	}
}

package memkv

import (
	"slices"
	"strings"
	"testing"
)

func TestTx_Key(t *testing.T) {
	tests := []struct {
		name  string
		typ   string
		attrs []string
		want  string
		fails bool
	}{
		{name: "error - object type holding NUL", typ: "a\x00b", fails: true},
		{name: "error - attribute holding NUL", typ: "t", attrs: []string{"a\x00b"}, fails: true},
		{name: "error - attribute holding U+10FFFF", typ: "t", attrs: []string{"a\U0010FFFFb"}, fails: true},
		{name: "error - attribute not UTF-8", typ: "t", attrs: []string{"a\xffb"}, fails: true},
		{name: "success - object type alone", typ: "t", want: "\x00t\x00"},
		{name: "success - attributes are NUL-terminated", typ: "prop", attrs: []string{"a", "b"}, want: "\x00prop\x00a\x00b\x00"},
		{name: "success - empty attribute", typ: "t", attrs: []string{""}, want: "\x00t\x00\x00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New().Begin().Key(tt.typ, tt.attrs...)
			if (err != nil) != tt.fails {
				t.Fatalf("err = %v, want failure %v", err, tt.fails)
			}
			if got != tt.want {
				t.Errorf("key = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTx_Isolation(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, s *Store)
	}{
		{
			name: "success - a put is invisible to its own transaction",
			run: func(t *testing.T, s *Store) {
				tx := s.Begin()
				put(t, tx, "k", "v")
				assertGet(t, tx, "k", "")
			},
		},
		{
			name: "success - a transaction begun earlier sees a later commit",
			run: func(t *testing.T, s *Store) {
				writer, reader := s.Begin(), s.Begin()
				put(t, writer, "k", "v")
				writer.Commit()
				assertGet(t, reader, "k", "v")
			},
		},
		{
			name: "success - an uncommitted transaction leaves no trace",
			run: func(t *testing.T, s *Store) {
				put(t, s.Begin(), "k", "v")
				assertGet(t, s.Begin(), "k", "")
			},
		},
		{
			name: "success - a later put in one transaction wins",
			run: func(t *testing.T, s *Store) {
				tx := s.Begin()
				put(t, tx, "k", "1")
				put(t, tx, "k", "2")
				tx.Commit()
				assertGet(t, s.Begin(), "k", "2")
			},
		},
		{
			name: "success - a read returns a copy",
			run: func(t *testing.T, s *Store) {
				tx := s.Begin()
				put(t, tx, "k", "v")
				tx.Commit()
				b, _ := s.Begin().Get("k")
				b[0] = 'X'
				assertGet(t, s.Begin(), "k", "v")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { tt.run(t, New()) })
	}
}

func TestTx_Scan(t *testing.T) {
	tests := []struct {
		name  string
		typ   string
		attrs []string
		want  []string
		fails bool
	}{
		{name: "error - attribute holding NUL", typ: "prop", attrs: []string{"a\x00"}, fails: true},
		{name: "success - whole object type in key order", typ: "prop", want: []string{"a1", "a2", "ab1", "b1"}},
		{name: "success - partial key does not match a longer attribute", typ: "prop", attrs: []string{"a"}, want: []string{"a1", "a2"}},
		{name: "success - full key", typ: "prop", attrs: []string{"a", "2"}, want: []string{"a2"}},
		{name: "success - other object types are not included", typ: "topic", want: []string{"t"}},
		{name: "success - no match", typ: "prop", attrs: []string{"z"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			tx := s.Begin()
			for _, k := range [][]string{{"prop", "b", "1"}, {"prop", "a", "2"}, {"prop", "ab", "1"}, {"prop", "a", "1"}, {"topic", "t"}} {
				key, _ := tx.Key(k[0], k[1:]...)
				put(t, tx, key, strings.Join(k[1:], ""))
			}
			tx.Commit()

			seq, err := s.Begin().Scan(tt.typ, tt.attrs...)
			if (err != nil) != tt.fails {
				t.Fatalf("err = %v, want failure %v", err, tt.fails)
			}
			if tt.fails {
				return
			}
			var got []string
			for v, err := range seq {
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, string(v))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("scanned %v, want %v", got, tt.want)
			}
		})
	}
}

func put(t *testing.T, tx *Tx, key, value string) {
	t.Helper()
	if err := tx.Put(key, []byte(value)); err != nil {
		t.Fatal(err)
	}
}

// assertGet holds a key to a value; "" means absent.
func assertGet(t *testing.T, tx *Tx, key, want string) {
	t.Helper()
	got, err := tx.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want || (want == "") != (got == nil) {
		t.Errorf("Get(%q) = %q, want %q", key, got, want)
	}
}

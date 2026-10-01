// Package memkv is a fake Fabric world state: a sorted in-memory map whose
// composite keys are Fabric's, and whose transactions read committed state
// only, so a test that reads its own write fails as it would on a peer.
package memkv

import (
	"fmt"
	"iter"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	namespace = "\x00"
	minRune   = 0
	maxRune   = utf8.MaxRune
)

// Store is the committed world state. Safe for concurrent use.
type Store struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func New() *Store { return &Store{data: map[string][]byte{}} }

func (s *Store) Begin() *Tx { return &Tx{store: s, puts: map[string][]byte{}} }

// Invoke runs f in one transaction, committed only if f succeeds, as a failed
// endorsement commits nothing.
func (s *Store) Invoke(f func(*Tx) error) error {
	tx := s.Begin()
	if err := f(tx); err != nil {
		return err
	}
	tx.Commit()
	return nil
}

// Tx is one chaincode invocation: reads see committed state, puts are held
// until Commit. Not safe for concurrent use.
type Tx struct {
	store *Store
	puts  map[string][]byte
}

func (t *Tx) Get(key string) ([]byte, error) {
	t.store.mu.RLock()
	defer t.store.mu.RUnlock()
	return slices.Clone(t.store.data[key]), nil
}

func (t *Tx) Put(key string, value []byte) error {
	t.puts[key] = slices.Clone(value)
	return nil
}

func (t *Tx) Commit() {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	for k, v := range t.puts {
		t.store.data[k] = v
	}
	t.puts = map[string][]byte{}
}

func (*Tx) Key(objectType string, attrs ...string) (string, error) {
	var b strings.Builder
	b.WriteString(namespace)
	for _, a := range append([]string{objectType}, attrs...) {
		if err := validate(a); err != nil {
			return "", err
		}
		b.WriteString(a)
		b.WriteString(string(rune(minRune)))
	}
	return b.String(), nil
}

// Scan is Fabric's GetStateByPartialCompositeKey.
func (t *Tx) Scan(objectType string, attrs ...string) (iter.Seq2[[]byte, error], error) {
	start, err := t.Key(objectType, attrs...)
	if err != nil {
		return nil, err
	}
	end := start + string(rune(maxRune))

	t.store.mu.RLock()
	var keys []string
	for k := range t.store.data {
		if k >= start && k < end {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	values := make([][]byte, len(keys))
	for i, k := range keys {
		values[i] = slices.Clone(t.store.data[k])
	}
	t.store.mu.RUnlock()

	return func(yield func([]byte, error) bool) {
		for _, v := range values {
			if !yield(v, nil) {
				return
			}
		}
	}, nil
}

func validate(attr string) error {
	if !utf8.ValidString(attr) {
		return fmt.Errorf("not a valid utf8 string: [%x]", attr)
	}
	for i, r := range attr {
		if r == minRune || r == maxRune {
			return fmt.Errorf("input contains unicode %#U starting at position [%d]; %#U and %#U are not allowed in a composite key attribute", r, i, rune(minRune), rune(maxRune))
		}
	}
	return nil
}

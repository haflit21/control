// Package household keeps the people of the house (people.json in the state
// directory; in memory when dir is empty). Until the app adds or removes
// someone, the list is control.yaml's people: (the seed); from the first
// change on, people.json is the reference and people: is no longer read.
package household

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

// Max is the number of people the house can have.
const Max = 12

// Member is one person: ID is also their personal calendar id and their id in
// the face service.
type Member struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

var (
	ErrNotFound = errors.New("personne inconnue")
	idRule      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	colorRule   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// ValidID: the face service's identifier rule (lowercase, digits, - and _).
func ValidID(id string) bool { return idRule.MatchString(id) }

type Store struct {
	dir    string
	mu     sync.RWMutex
	people []Member
	// OnChange is called (outside the lock) after every successful change.
	OnChange func()
}

// Open reads people.json, or starts from seed (control.yaml) when it does not exist.
func Open(dir string, seed []Member) (*Store, error) {
	s := &Store{dir: dir, people: append([]Member{}, seed...)}
	if dir == "" {
		return s, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.file())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Member
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, errors.New("people.json illisible : " + err.Error())
	}
	s.people = list[:0]
	for _, m := range list {
		if ValidID(m.ID) && m.Name != "" {
			s.people = append(s.people, m)
		}
	}
	return s, nil
}

func (s *Store) file() string { return filepath.Join(s.dir, "people.json") }

// Stored says whether the list comes from people.json (true) or from control.yaml.
func (s *Store) Stored() bool {
	if s.dir == "" {
		return false
	}
	_, err := os.Stat(s.file())
	return err == nil
}

// List returns a copy, in the order they were added.
func (s *Store) List() []Member {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Member{}, s.people...)
}

func (s *Store) Get(id string) (Member, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.people {
		if m.ID == id {
			return m, true
		}
	}
	return Member{}, false
}

// Add creates a person from a name (the id is derived from it). taken reports
// ids used elsewhere (the shared calendars).
func (s *Store) Add(name, color string, taken func(string) bool) (Member, error) {
	name = strings.Join(strings.Fields(name), " ")
	switch {
	case name == "":
		return Member{}, errors.New("prénom manquant")
	case utf8.RuneCountInString(name) > 40:
		return Member{}, errors.New("prénom trop long (40 caractères au plus)")
	case color != "" && !colorRule.MatchString(color):
		return Member{}, errors.New("couleur invalide (#rrggbb attendu)")
	}
	s.mu.Lock()
	if len(s.people) >= Max {
		s.mu.Unlock()
		return Member{}, fmt.Errorf("%d personnes au plus", Max)
	}
	for _, m := range s.people {
		if strings.EqualFold(m.Name, name) {
			s.mu.Unlock()
			return Member{}, fmt.Errorf("%s fait déjà partie du foyer", m.Name)
		}
	}
	base := Slug(name)
	id := base
	for n := 2; s.has(id) || (taken != nil && taken(id)); n++ {
		suffix := fmt.Sprintf("-%d", n)
		id = strings.TrimRight(base[:min(len(base), 32-len(suffix))], "-_") + suffix
	}
	m := Member{ID: id, Name: name, Color: strings.ToLower(color)}
	prev := s.people
	s.people = append(append([]Member{}, s.people...), m)
	err := s.save()
	if err != nil {
		s.people = prev
	}
	s.mu.Unlock()
	if err == nil && s.OnChange != nil {
		s.OnChange()
	}
	return m, err
}

// Remove takes the person out of the house.
func (s *Store) Remove(id string) (Member, error) {
	s.mu.Lock()
	idx := -1
	for i, m := range s.people {
		if m.ID == id {
			idx = i
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return Member{}, ErrNotFound
	}
	gone, prev := s.people[idx], s.people
	s.people = append(append([]Member{}, prev[:idx]...), prev[idx+1:]...)
	err := s.save()
	if err != nil {
		s.people = prev
	}
	s.mu.Unlock()
	if err == nil && s.OnChange != nil {
		s.OnChange()
	}
	return gone, err
}

func (s *Store) has(id string) bool {
	for _, m := range s.people {
		if m.ID == id {
			return true
		}
	}
	return false
}

// save writes atomically (temp file + rename). Caller holds the lock.
func (s *Store) save() error {
	if s.dir == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.people, "", " ")
	if err != nil {
		return err
	}
	tmp := s.file() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.file())
}

// fold drops the accents of French (and nearby) names.
var fold = strings.NewReplacer("à", "a", "á", "a", "â", "a", "ä", "a", "ã", "a", "å", "a", "æ", "ae", "ç", "c",
	"é", "e", "è", "e", "ê", "e", "ë", "e", "í", "i", "ì", "i", "î", "i", "ï", "i", "ñ", "n",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o", "œ", "oe", "ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ý", "y", "ÿ", "y", "ß", "ss")

// Slug: « Zoé Martin » → « zoe-martin » (an id the face service accepts).
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range fold.Replace(strings.ToLower(name)) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 32 {
			break
		}
	}
	id := strings.Trim(b.String(), "-")
	if len(id) > 32 {
		id = strings.Trim(id[:32], "-")
	}
	if id == "" {
		id = "personne"
	}
	return id
}

// Package favorites keeps the household's favourite tracks and albums
// (favorites.json in the state directory; in memory when dir is empty).
package favorites

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"ahomelab.io/control/internal/model"
)

// Max is the number of favourites kept per kind (the oldest go first).
const Max = 300

type Store struct {
	dir  string
	mu   sync.Mutex
	data model.Results
}

func Open(dir string) (*Store, error) {
	s := &Store{dir: dir, data: model.Results{Tracks: []model.Track{}, Albums: []model.Album{}}}
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
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, errors.New("favorites.json illisible : " + err.Error())
	}
	if s.data.Tracks == nil {
		s.data.Tracks = []model.Track{}
	}
	if s.data.Albums == nil {
		s.data.Albums = []model.Album{}
	}
	return s, nil
}

func (s *Store) file() string { return filepath.Join(s.dir, "favorites.json") }

// List returns a copy, the latest added first.
func (s *Store) List() model.Results {
	s.mu.Lock()
	defer s.mu.Unlock()
	return model.Results{Tracks: append([]model.Track{}, s.data.Tracks...), Albums: append([]model.Album{}, s.data.Albums...)}
}

// SetTrack adds (on) or removes a track, keyed by its URI.
func (s *Store) SetTrack(t model.Track, on bool) (model.Results, error) {
	if err := check(t.URI, t.Title, t.Artists+t.Album, t.Cover); err != nil {
		return model.Results{}, err
	}
	s.mu.Lock()
	out := []model.Track{}
	if on {
		out = append(out, t)
	}
	for _, x := range s.data.Tracks {
		if x.URI != t.URI && len(out) < Max {
			out = append(out, x)
		}
	}
	s.data.Tracks = out
	s.mu.Unlock()
	return s.save()
}

// SetAlbum adds (on) or removes an album, keyed by its URI.
func (s *Store) SetAlbum(a model.Album, on bool) (model.Results, error) {
	if err := check(a.URI, a.Title, a.Artists, a.Cover); err != nil {
		return model.Results{}, err
	}
	s.mu.Lock()
	out := []model.Album{}
	if on {
		out = append(out, a)
	}
	for _, x := range s.data.Albums {
		if x.URI != a.URI && len(out) < Max {
			out = append(out, x)
		}
	}
	s.data.Albums = out
	s.mu.Unlock()
	return s.save()
}

func check(uri, title, rest, cover string) error {
	switch {
	case uri == "" || len(uri) > 300 || strings.ContainsAny(uri, " \n\t\"'<>") || (!strings.Contains(uri, "://") && strings.Count(uri, ":") < 2):
		return errors.New("titre invalide")
	case title == "" || utf8.RuneCountInString(title) > 200 || utf8.RuneCountInString(rest) > 400:
		return errors.New("titre trop long ou vide")
	case cover != "" && (!strings.HasPrefix(cover, "https://") || len(cover) > 500):
		return errors.New("pochette invalide")
	}
	return nil
}

func (s *Store) save() (model.Results, error) {
	list := s.List()
	if s.dir == "" {
		return list, nil
	}
	b, err := json.MarshalIndent(list, "", " ")
	if err != nil {
		return list, err
	}
	tmp := s.file() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return list, err
	}
	return list, os.Rename(tmp, s.file())
}

// Package agenda is the family calendar owned by the app: events written from
// the interface (form or voice), stored as JSON in the state dir, expanded into
// occurrences for the weeks shown.
//
// Dates and times are wall-clock strings ("2026-11-12", "09:30") interpreted in
// the house's time zone: a weekly 09:30 stays at 09:30 across DST changes.
package agenda

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Repeat frequencies.
const (
	Weekly   = "weekly"
	Biweekly = "biweekly"
	Monthly  = "monthly"
	Yearly   = "yearly"
)

type Repeat struct {
	Freq  string `json:"freq"`            // weekly | biweekly | monthly | yearly
	Until string `json:"until,omitempty"` // last day (inclusive), YYYY-MM-DD; empty = forever
}

type Event struct {
	ID       string  `json:"id"`
	Calendar string  `json:"calendar"`
	Title    string  `json:"title"`
	Location string  `json:"location,omitempty"`
	AllDay   bool    `json:"allDay"`
	Date     string  `json:"date"`              // first day, YYYY-MM-DD
	EndDate  string  `json:"endDate,omitempty"` // all-day events spanning several days: last day (inclusive)
	Start    string  `json:"start,omitempty"`   // HH:MM (timed events)
	End      string  `json:"end,omitempty"`     // HH:MM; before Start = the next day
	Repeat   *Repeat `json:"repeat,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	CreatedBy string    `json:"createdBy,omitempty"`
}

// Occurrence is one concrete instance of an event in a window.
type Occurrence struct {
	ID      string // "<event id>@<date>"
	Event   *Event
	Start   time.Time
	End     time.Time
	AllDay  bool
	Repeats bool
}

const (
	dateLayout = "2006-01-02"
	timeLayout = "15:04"
)

var ErrNotFound = errors.New("événement introuvable")

// Validate normalises an event in place (trims, defaults) and checks it.
// known: the calendar ids allowed (first one is the default).
func (e *Event) Validate(known []string) error {
	e.Title = strings.TrimSpace(e.Title)
	e.Location = strings.TrimSpace(e.Location)
	if e.Title == "" {
		return errors.New("le titre est obligatoire")
	}
	if utf8.RuneCountInString(e.Title) > 120 {
		return errors.New("titre trop long (120 caractères au plus)")
	}
	if utf8.RuneCountInString(e.Location) > 120 {
		return errors.New("lieu trop long (120 caractères au plus)")
	}
	if len(known) > 0 {
		ok := false
		for _, k := range known {
			ok = ok || k == e.Calendar
		}
		if !ok {
			if e.Calendar != "" {
				return fmt.Errorf("calendrier inconnu : %s", e.Calendar)
			}
			e.Calendar = known[0]
		}
	}
	d, err := time.Parse(dateLayout, e.Date)
	if err != nil {
		return fmt.Errorf("date invalide : %q", e.Date)
	}
	if e.AllDay {
		e.Start, e.End = "", ""
		if e.EndDate == e.Date {
			e.EndDate = ""
		}
		if e.EndDate != "" {
			ed, err := time.Parse(dateLayout, e.EndDate)
			if err != nil {
				return fmt.Errorf("date de fin invalide : %q", e.EndDate)
			}
			if ed.Before(d) {
				return errors.New("la date de fin est avant le début")
			}
			if ed.Sub(d) > 62*24*time.Hour {
				return errors.New("un événement d'une journée entière dure au plus deux mois")
			}
		}
	} else {
		e.EndDate = ""
		st, err := time.Parse(timeLayout, e.Start)
		if err != nil {
			return fmt.Errorf("heure de début invalide : %q", e.Start)
		}
		e.Start = st.Format(timeLayout)
		if e.End == "" {
			e.End = st.Add(time.Hour).Format(timeLayout)
		}
		en, err := time.Parse(timeLayout, e.End)
		if err != nil {
			return fmt.Errorf("heure de fin invalide : %q", e.End)
		}
		e.End = en.Format(timeLayout)
	}
	if e.Repeat != nil {
		switch e.Repeat.Freq {
		case "", "none":
			e.Repeat = nil
		case Weekly, Biweekly, Monthly, Yearly:
			if e.Repeat.Until != "" {
				u, err := time.Parse(dateLayout, e.Repeat.Until)
				if err != nil {
					return fmt.Errorf("date de fin de répétition invalide : %q", e.Repeat.Until)
				}
				if u.Before(d) {
					return errors.New("la répétition s'arrête avant le premier jour")
				}
			}
		default:
			return fmt.Errorf("répétition inconnue : %s", e.Repeat.Freq)
		}
	}
	return nil
}

// bounds gives the occurrence starting on day (a date in loc, at 00:00).
func (e *Event) bounds(day time.Time, loc *time.Location) (time.Time, time.Time) {
	y, m, d := day.Date()
	if e.AllDay {
		days := 1
		if e.EndDate != "" {
			a, _ := time.Parse(dateLayout, e.Date)
			b, _ := time.Parse(dateLayout, e.EndDate)
			days = int(b.Sub(a).Hours()/24) + 1
		}
		return time.Date(y, m, d, 0, 0, 0, 0, loc), time.Date(y, m, d+days, 0, 0, 0, 0, loc)
	}
	st, _ := time.Parse(timeLayout, e.Start)
	en, _ := time.Parse(timeLayout, e.End)
	start := time.Date(y, m, d, st.Hour(), st.Minute(), 0, 0, loc)
	endDay := d
	if !en.After(st) {
		endDay++ // 22:00 → 01:00, or 00:00 meaning midnight
	}
	return start, time.Date(y, m, endDay, en.Hour(), en.Minute(), 0, 0, loc)
}

// Occurrences of the event overlapping [from, to).
func (e *Event) Occurrences(from, to time.Time, loc *time.Location) []Occurrence {
	base, err := time.ParseInLocation(dateLayout, e.Date, loc)
	if err != nil {
		return nil
	}
	var until time.Time
	if e.Repeat != nil && e.Repeat.Until != "" {
		until, _ = time.ParseInLocation(dateLayout, e.Repeat.Until, loc)
	}
	by, bm, bd := base.Date()
	var out []Occurrence
	emit := func(day time.Time) {
		s, en := e.bounds(day, loc)
		if en.After(from) && s.Before(to) {
			out = append(out, Occurrence{ID: e.ID + "@" + day.Format(dateLayout), Event: e, Start: s, End: en, AllDay: e.AllDay, Repeats: e.Repeat != nil})
		}
	}
	if e.Repeat == nil {
		emit(base)
		return out
	}
	// first index worth looking at: skip whole periods that end before `from`
	k0 := 0
	span := 62 * 24 * time.Hour // longest occurrence (see Validate)
	if gap := from.Add(-span).Sub(base); gap > 0 {
		switch e.Repeat.Freq {
		case Weekly:
			k0 = int(gap / (7 * 24 * time.Hour))
		case Biweekly:
			k0 = int(gap / (14 * 24 * time.Hour))
		case Monthly:
			k0 = int(gap/(31*24*time.Hour)) - 1
		case Yearly:
			k0 = int(gap/(366*24*time.Hour)) - 1
		}
		if k0 < 0 {
			k0 = 0
		}
	}
	for k := k0; k < k0+2000; k++ {
		var day time.Time
		switch e.Repeat.Freq {
		case Weekly:
			day = time.Date(by, bm, bd+7*k, 0, 0, 0, 0, loc)
		case Biweekly:
			day = time.Date(by, bm, bd+14*k, 0, 0, 0, 0, loc)
		case Monthly:
			day = time.Date(by, bm+time.Month(k), bd, 0, 0, 0, 0, loc)
			if day.Day() != bd { // the 31st in a 30-day month: no occurrence (as in iCalendar)
				continue
			}
		case Yearly:
			day = time.Date(by+k, bm, bd, 0, 0, 0, 0, loc)
			if day.Day() != bd { // 29 February in a common year
				continue
			}
		default:
			return out
		}
		if !until.IsZero() && day.After(until) {
			break
		}
		if !day.Before(to) {
			break
		}
		emit(day)
	}
	return out
}

// Store keeps the events. dir "" = memory only (demo, tests).
type Store struct {
	dir    string
	mu     sync.RWMutex
	events map[string]*Event
	// OnChange is called (outside the lock) after every successful write.
	OnChange func()
}

func Open(dir string) (*Store, error) {
	s := &Store{dir: dir, events: map[string]*Event{}}
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
	var list []*Event
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("agenda %s : %w", s.file(), err)
	}
	for _, e := range list {
		if e.ID != "" {
			s.events[e.ID] = e
		}
	}
	return s, nil
}

func (s *Store) file() string { return filepath.Join(s.dir, "events.json") }

// save writes atomically (temp file + rename). Caller holds the lock.
func (s *Store) save() error {
	if s.dir == "" {
		return nil
	}
	list := make([]*Event, 0, len(s.events))
	for _, e := range s.events {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Date+list[i].Start+list[i].ID < list[j].Date+list[j].Start+list[j].ID
	})
	b, err := json.MarshalIndent(list, "", " ")
	if err != nil {
		return err
	}
	tmp := s.file() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.file())
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Add stores a new event (already validated) and returns it with its id.
func (s *Store) Add(e Event, by string) (Event, error) {
	now := time.Now()
	e.ID, e.CreatedAt, e.UpdatedAt, e.CreatedBy = newID(), now, now, by
	s.mu.Lock()
	s.events[e.ID] = &e
	err := s.save()
	if err != nil {
		delete(s.events, e.ID)
	}
	s.mu.Unlock()
	if err == nil && s.OnChange != nil {
		s.OnChange()
	}
	return e, err
}

// Update replaces an existing event (already validated), keeping its creation data.
func (s *Store) Update(id string, e Event) (Event, error) {
	s.mu.Lock()
	old, ok := s.events[id]
	if !ok {
		s.mu.Unlock()
		return Event{}, ErrNotFound
	}
	e.ID, e.CreatedAt, e.CreatedBy, e.UpdatedAt = id, old.CreatedAt, old.CreatedBy, time.Now()
	s.events[id] = &e
	err := s.save()
	if err != nil {
		s.events[id] = old
	}
	s.mu.Unlock()
	if err == nil && s.OnChange != nil {
		s.OnChange()
	}
	return e, err
}

func (s *Store) Delete(id string) (Event, error) {
	s.mu.Lock()
	old, ok := s.events[id]
	if !ok {
		s.mu.Unlock()
		return Event{}, ErrNotFound
	}
	delete(s.events, id)
	err := s.save()
	if err != nil {
		s.events[id] = old
	}
	s.mu.Unlock()
	if err == nil && s.OnChange != nil {
		s.OnChange()
	}
	return *old, err
}

// DeleteCalendar removes every event of a calendar (a person leaving the house)
// and returns how many there were.
func (s *Store) DeleteCalendar(cal string) (int, error) {
	s.mu.Lock()
	gone := map[string]*Event{}
	for id, e := range s.events {
		if e.Calendar == cal {
			gone[id] = e
			delete(s.events, id)
		}
	}
	if len(gone) == 0 {
		s.mu.Unlock()
		return 0, nil
	}
	err := s.save()
	if err != nil {
		for id, e := range gone {
			s.events[id] = e
		}
	}
	s.mu.Unlock()
	if err == nil && s.OnChange != nil {
		s.OnChange()
	}
	return len(gone), err
}

func (s *Store) Get(id string) (Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.events[id]
	if !ok {
		return Event{}, false
	}
	return *e, true
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

// Between expands every event into the occurrences overlapping [from, to), sorted by start.
func (s *Store) Between(from, to time.Time, loc *time.Location) []Occurrence {
	s.mu.RLock()
	var out []Occurrence
	for _, e := range s.events {
		cp := *e
		out = append(out, cp.Occurrences(from, to, loc)...)
	}
	s.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		if out[i].AllDay != out[j].AllDay {
			return out[i].AllDay
		}
		return out[i].Event.Title < out[j].Event.Title
	})
	return out
}

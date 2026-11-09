// Package journal keeps who did what and when: every action accepted (or
// refused) by the server, appended to a JSON-lines file per month in the state
// dir. Read back for the interface (last N entries) — nothing is ever edited.
package journal

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	At     time.Time `json:"at"`
	User   string    `json:"user"`             // who (session label; "Abdelhak" while there is one user)
	Via    string    `json:"via,omitempty"`    // device label ("iPhone · Safari")
	Scope  string    `json:"scope"`            // device | house | security | push
	Target string    `json:"target,omitempty"` // device id ("bureau.hue1") or room
	Action string    `json:"action"`           // action name ("power", "set_target", "alarm")
	Params string    `json:"params,omitempty"` // compact JSON of the params, if any
	Text   string    `json:"text"`             // what the interface said ("Lampe canapé allumée")
	OK     bool      `json:"ok"`
	Error  string    `json:"error,omitempty"`
}

type Store struct {
	dir string
	mu  sync.Mutex
	// recent keeps the tail in memory so reads do not hit the disk each time
	recent []Entry
	max    int
}

const keep = 500

// Open loads the tail of the current and previous month.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, max: keep}
	now := time.Now()
	for _, m := range []time.Time{now.AddDate(0, -1, 0), now} {
		s.recent = append(s.recent, s.readFile(s.file(m))...)
	}
	sort.SliceStable(s.recent, func(i, j int) bool { return s.recent[i].At.Before(s.recent[j].At) })
	if len(s.recent) > s.max {
		s.recent = s.recent[len(s.recent)-s.max:]
	}
	return s, nil
}

func (s *Store) file(t time.Time) string {
	return filepath.Join(s.dir, t.Format("2006-01")+".jsonl")
}

func (s *Store) readFile(p string) []Entry {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil && !e.At.IsZero() {
			out = append(out, e)
		}
	}
	return out
}

// Add appends an entry (At is set if zero). Errors writing are swallowed: the
// journal must never block an action.
func (s *Store) Add(e Entry) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if len(e.Params) > 300 {
		e.Params = e.Params[:300] + "…"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recent = append(s.recent, e)
	if len(s.recent) > s.max {
		s.recent = s.recent[len(s.recent)-s.max:]
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	f, err := os.OpenFile(s.file(e.At), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// Recent returns the last n entries, newest first.
func (s *Store) Recent(n int) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 || n > len(s.recent) {
		n = len(s.recent)
	}
	out := make([]Entry, n)
	for i := 0; i < n; i++ {
		out[i] = s.recent[len(s.recent)-1-i]
	}
	return out
}

// Compact renders params for the journal line, without the entity ids.
func Compact(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	t := strings.TrimSpace(string(b))
	if t == "null" || t == "{}" {
		return ""
	}
	return t
}

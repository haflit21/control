package journal

import (
	"testing"
	"time"
)

func TestJournal(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Add(Entry{Scope: "device", Target: "bureau.hue1", Action: "power", Params: Compact(map[string]bool{"on": true}), Text: "Allumée", OK: true, User: "Abdelhak"})
	time.Sleep(2 * time.Millisecond)
	s.Add(Entry{Scope: "security", Action: "alarm", Text: "Alarme armée", OK: true})
	// previous month too: loaded at Open
	s.Add(Entry{At: time.Now().AddDate(0, -1, 0), Scope: "house", Action: "heating", Text: "Vieux"})
	// an entry two months old is not loaded back
	s.Add(Entry{At: time.Now().AddDate(0, -2, 0), Scope: "house", Action: "heating", Text: "Très vieux"})

	r := s.Recent(2)
	if len(r) != 2 || r[0].Text != "Très vieux" {
		// in memory, order is insertion order: the last added is first
		t.Fatalf("recent: %+v", r)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	all := s2.Recent(0)
	if len(all) != 3 {
		t.Fatalf("après rechargement : %d entrées (%+v)", len(all), all)
	}
	if all[0].Text != "Alarme armée" || all[1].Text != "Allumée" || all[2].Text != "Vieux" {
		t.Fatalf("ordre : %s / %s / %s", all[0].Text, all[1].Text, all[2].Text)
	}
	if all[1].Params != `{"on":true}` || all[1].User != "Abdelhak" {
		t.Fatalf("champs : %+v", all[1])
	}
	if Compact(nil) != "" || Compact(map[string]any{}) != "" {
		t.Fatal("Compact vide")
	}
}

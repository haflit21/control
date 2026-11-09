package agenda

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var paris, _ = time.LoadLocation("Europe/Paris")

func day(s string) time.Time { t, _ := time.ParseInLocation(dateLayout, s, paris); return t }

func dates(occ []Occurrence) []string {
	var out []string
	for _, o := range occ {
		out = append(out, o.Start.Format("2006-01-02 15:04"))
	}
	return out
}

func eq(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	cals := []string{"famille", "poubelles"}
	e := Event{Title: "  Pédiatre ", Date: "2026-11-12", Start: "9:30"}
	if err := e.Validate(cals); err != nil {
		t.Fatal(err)
	}
	if e.Title != "Pédiatre" || e.Calendar != "famille" || e.Start != "09:30" || e.End != "10:30" {
		t.Fatalf("normalisation : %+v", e)
	}
	bad := []Event{
		{Title: "", Date: "2026-11-12", AllDay: true},
		{Title: "x", Date: "12/11/2026", AllDay: true},
		{Title: "x", Date: "2026-11-12", Start: "25:00"},
		{Title: "x", Date: "2026-11-12", AllDay: true, Calendar: "boulot"},
		{Title: "x", Date: "2026-11-12", AllDay: true, EndDate: "2026-11-10"},
		{Title: "x", Date: "2026-11-12", AllDay: true, Repeat: &Repeat{Freq: "daily"}},
		{Title: "x", Date: "2026-11-12", AllDay: true, Repeat: &Repeat{Freq: Weekly, Until: "2026-11-01"}},
	}
	for i, b := range bad {
		if err := b.Validate(cals); err == nil {
			t.Errorf("cas %d accepté : %+v", i, b)
		}
	}
	none := Event{Title: "x", Date: "2026-11-12", AllDay: true, Start: "10:00", Repeat: &Repeat{Freq: "none"}}
	if err := none.Validate(cals); err != nil || none.Repeat != nil || none.Start != "" {
		t.Fatalf("journée entière / sans répétition : %+v %v", none, err)
	}
}

// A weekly 09:30 stays at 09:30 local time across the end of DST (25 Oct 2026).
func TestWeeklyAcrossDST(t *testing.T) {
	e := Event{ID: "c", Title: "Crèche", Date: "2026-10-12", Start: "09:30", End: "10:00", Repeat: &Repeat{Freq: Weekly}}
	occ := e.Occurrences(day("2026-10-12"), day("2026-11-03"), paris)
	eq(t, dates(occ), "2026-10-12 09:30", "2026-10-19 09:30", "2026-10-26 09:30", "2026-11-02 09:30")
	if occ[2].ID != "c@2026-10-26" || !occ[2].Repeats {
		t.Fatalf("id : %+v", occ[2])
	}
	if _, off := occ[2].Start.Zone(); off != 3600 {
		t.Fatalf("après le changement d'heure, offset %d", off)
	}
}

func TestBiweeklyUntilAndSkipAhead(t *testing.T) {
	e := Event{ID: "v", Title: "Verre", Date: "2026-01-02", AllDay: true, Repeat: &Repeat{Freq: Biweekly, Until: "2027-01-15"}}
	// a window far from the first date: the skip-ahead must not miss anything
	eq(t, dates(e.Occurrences(day("2026-11-09"), day("2026-12-14"), paris)), "2026-11-20 00:00", "2026-12-04 00:00")
	// until is inclusive: 1 and 15 January 2027 (2 Jan 2026 + 26 and 27 fortnights), nothing after
	eq(t, dates(e.Occurrences(day("2027-01-01"), day("2027-03-01"), paris)), "2027-01-01 00:00", "2027-01-15 00:00")
}

func TestMonthly31AndYearly29Feb(t *testing.T) {
	m := Event{ID: "m", Title: "Loyer", Date: "2026-01-31", AllDay: true, Repeat: &Repeat{Freq: Monthly}}
	eq(t, dates(m.Occurrences(day("2026-01-01"), day("2026-06-01"), paris)), "2026-01-31 00:00", "2026-03-31 00:00", "2026-05-31 00:00")
	y := Event{ID: "y", Title: "Anniv", Date: "2024-02-29", AllDay: true, Repeat: &Repeat{Freq: Yearly}}
	eq(t, dates(y.Occurrences(day("2025-01-01"), day("2029-01-01"), paris)), "2028-02-29 00:00")
	// monthly far ahead
	eq(t, dates(m.Occurrences(day("2030-07-01"), day("2030-09-01"), paris)), "2030-07-31 00:00", "2030-08-31 00:00")
}

func TestOvernightAndMultiDay(t *testing.T) {
	n := Event{ID: "n", Title: "Garde", Date: "2026-11-12", Start: "22:00", End: "01:00"}
	occ := n.Occurrences(day("2026-11-13"), day("2026-11-14"), paris) // only the tail is in the window
	if len(occ) != 1 || occ[0].End.Format("2006-01-02 15:04") != "2026-11-13 01:00" {
		t.Fatalf("nuit : %+v", occ)
	}
	v := Event{ID: "w", Title: "Vacances", Date: "2026-12-19", EndDate: "2027-01-03", AllDay: true}
	occ = v.Occurrences(day("2026-12-28"), day("2026-12-29"), paris)
	if len(occ) != 1 || occ[0].End.Format(dateLayout) != "2027-01-04" {
		t.Fatalf("plusieurs jours : %+v", occ)
	}
}

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	changes := 0
	s.OnChange = func() { changes++ }
	a, err := s.Add(Event{Title: "Pédiatre", Calendar: "famille", Date: "2026-11-12", Start: "09:30", End: "10:00"}, "Abdelhak")
	if err != nil || a.ID == "" || a.CreatedBy != "Abdelhak" {
		t.Fatalf("ajout : %+v %v", a, err)
	}
	b, _ := s.Add(Event{Title: "Poubelle jaune", Calendar: "poubelles", Date: "2026-11-10", AllDay: true, Repeat: &Repeat{Freq: Weekly}}, "")
	upd := a
	upd.Title = "Pédiatre · vaccin"
	if _, err := s.Update(a.ID, upd); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update("nope", upd); err != ErrNotFound {
		t.Fatalf("mise à jour inconnue : %v", err)
	}
	s2, err := Open(dir)
	if err != nil || s2.Count() != 2 {
		t.Fatalf("rechargement : %d %v", s2.Count(), err)
	}
	if g, _ := s2.Get(a.ID); g.Title != "Pédiatre · vaccin" || g.CreatedBy != "Abdelhak" || !g.CreatedAt.Equal(a.CreatedAt) {
		t.Fatalf("relu : %+v", g)
	}
	occ := s2.Between(day("2026-11-09"), day("2026-11-23"), paris)
	got := []string{}
	for _, o := range occ {
		got = append(got, o.Event.Title+" "+o.Start.Format("01-02 15:04"))
	}
	eq(t, got, "Poubelle jaune 11-10 00:00", "Pédiatre · vaccin 11-12 09:30", "Poubelle jaune 11-17 00:00")
	if _, err := s.Delete(b.ID); err != nil || s.Count() != 1 {
		t.Fatalf("suppression : %v", err)
	}
	if changes != 4 { // add, add, update, delete (the failed update does not count)
		t.Fatalf("OnChange appelé %d fois", changes)
	}
	if _, err := os.Stat(filepath.Join(dir, "events.json.tmp")); !os.IsNotExist(err) {
		t.Fatal("fichier temporaire laissé")
	}
	mem, _ := Open("")
	if _, err := mem.Add(Event{Title: "x", Date: "2026-11-12", AllDay: true}, ""); err != nil || mem.Count() != 1 {
		t.Fatal("mémoire seule")
	}
}

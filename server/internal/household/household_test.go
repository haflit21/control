package household

import (
	"path/filepath"
	"testing"
)

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Zoé Martin": "zoe-martin", "  Idris ": "idris", "Œdipe": "oedipe", "!!!": "personne", "Jean-Éric": "jean-eric"} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func TestAddRemovePersist(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, []Member{{ID: "abdelhak", Name: "Abdelhak"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Stored() {
		t.Fatal("people.json ne devrait pas exister avant un changement")
	}
	taken := func(id string) bool { return id == "famille" }
	m, err := s.Add("Famille", "#AABBCC", taken)
	if err != nil || m.ID != "famille-2" || m.Color != "#aabbcc" {
		t.Fatalf("Add = %+v, %v", m, err)
	}
	if _, err := s.Add("famille", "", taken); err == nil {
		t.Fatal("doublon accepté")
	}
	if _, err := s.Add("Zoé", "red", taken); err == nil {
		t.Fatal("couleur invalide acceptée")
	}
	if _, err := s.Remove("abdelhak"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Remove("abdelhak"); err != ErrNotFound {
		t.Fatalf("Remove absent : %v", err)
	}
	// reopened with another seed: people.json wins
	s2, err := Open(dir, []Member{{ID: "x", Name: "X"}})
	if err != nil {
		t.Fatal(err)
	}
	if l := s2.List(); len(l) != 1 || l[0].ID != "famille-2" || !s2.Stored() {
		t.Fatalf("relu : %+v", l)
	}
	if _, err := Open(filepath.Join(dir, "absent"), nil); err != nil {
		t.Fatal(err)
	}
}

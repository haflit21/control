package favorites

import (
	"testing"

	"ahomelab.io/control/internal/model"
)

func TestFavorites(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := model.Track{URI: "spotify://track/1", Title: "Golden", Artists: "HUNTR/X"}
	b := model.Track{URI: "spotify://track/2", Title: "Soda Pop", Artists: "Saja Boys"}
	if _, err := s.SetTrack(a, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetTrack(b, true); err != nil {
		t.Fatal(err)
	}
	r, _ := s.SetTrack(a, true) // again: moves to the top, no duplicate
	if len(r.Tracks) != 2 || r.Tracks[0].URI != a.URI {
		t.Fatalf("ordre : %+v", r.Tracks)
	}
	if _, err := s.SetAlbum(model.Album{URI: "spotify://album/k", Title: "KPop Demon Hunters"}, true); err != nil {
		t.Fatal(err)
	}
	// reopened from disk
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l := s2.List(); len(l.Tracks) != 2 || len(l.Albums) != 1 {
		t.Fatalf("relu : %+v", l)
	}
	r, _ = s2.SetTrack(a, false)
	if len(r.Tracks) != 1 || r.Tracks[0].URI != b.URI {
		t.Fatalf("retrait : %+v", r.Tracks)
	}
	for _, bad := range []model.Track{{URI: "javascript:alert(1)", Title: "x"}, {URI: "spotify://track/3"}, {URI: "spotify://track/4", Title: "x", Cover: "http://lan/x.jpg"}} {
		if _, err := s2.SetTrack(bad, true); err == nil {
			t.Errorf("accepté : %+v", bad)
		}
	}
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ahomelab.io/control/internal/agenda"
	"ahomelab.io/control/internal/face"
	"ahomelab.io/control/internal/journal"
)

// fakeFaces: the photo's bytes after the JPEG marker say who is on it.
type fakeFaces struct {
	prints map[string]int
	down   bool
}

var jpeg = []byte{0xff, 0xd8, 0xff, 0xe0}

func photoOf(who string) []byte { return append(append([]byte{}, jpeg...), who...) }

func (f *fakeFaces) Identify(_ context.Context, p []byte) (face.Result, error) {
	if f.down {
		return face.Result{}, errors.New("connexion refusée")
	}
	who := string(p[len(jpeg):])
	r := face.Result{Faces: 1, Threshold: 0.363}
	switch {
	case who == "":
		r.Faces = 0
	case f.prints[who] > 0:
		r.Match = &face.Score{ID: who, Score: 0.74}
		r.Best = r.Match
	default:
		r.Best = &face.Score{ID: "idris", Score: 0.18}
	}
	return r, nil
}

func (f *fakeFaces) Enroll(_ context.Context, id string, p []byte) (face.Enrolled, error) {
	if string(p[len(jpeg):]) == "" {
		return face.Enrolled{}, &face.Error{Status: 422, Message: "aucun visage sur la photo"}
	}
	f.prints[id]++
	return face.Enrolled{ID: id, Samples: f.prints[id]}, nil
}

func (f *fakeFaces) Forget(_ context.Context, id string) error {
	delete(f.prints, id)
	return nil
}

func (f *fakeFaces) People(context.Context) ([]face.Person, error) {
	if f.down {
		return nil, errors.New("connexion refusée")
	}
	var out []face.Person
	for id, n := range f.prints {
		out = append(out, face.Person{ID: id, Samples: n})
	}
	return out, nil
}

func TestFaces(t *testing.T) {
	s, _ := newTestServer(t)
	js, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Journal = js
	h := s.Handler()
	call := func(method, path string, body []byte) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.AddCookie(&http.Cookie{Name: cookieName, Value: s.Token})
		req.Header.Set("X-Control", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	// not configured: nothing offered
	if code, _ := call("POST", "/api/face/identify", photoOf("idris")); code != http.StatusNotImplemented {
		t.Fatalf("sans service : %d", code)
	}
	code, out := call("GET", "/api/people", nil)
	if code != 200 || out["face"] != false || len(out["people"].([]any)) != len(s.Cfg.People) {
		t.Fatalf("liste sans service : %d %v", code, out)
	}
	if p := out["people"].([]any)[0].(map[string]any); p["samples"] != nil {
		t.Fatalf("sans service, pas de compte : %v", p)
	}

	fake := &fakeFaces{prints: map[string]int{}}
	s.Face = fake
	s.rebuild(nil)
	home := s.Home()
	if !home.Face || len(home.People) != 3 || home.People[2].ID != "idris" {
		t.Fatalf("home : face=%v people=%+v", home.Face, home.People)
	}
	if s.Agenda, err = agenda.Open(""); err != nil {
		t.Fatal(err)
	}
	personal := 0
	for _, c := range s.AgendaModel(s.Home().UpdatedAt).Calendars {
		if c.Owner != "" {
			personal++
		}
	}
	if personal != 3 {
		t.Fatalf("agendas personnels publiés : %d", personal)
	}

	// enrolment
	if code, out := call("POST", "/api/people/idris/faces", photoOf("idris")); code != 200 || out["samples"] != 1.0 {
		t.Fatalf("enrôlement : %d %v", code, out)
	}
	if code, out := call("POST", "/api/people/idris/faces", photoOf("")); code != http.StatusUnprocessableEntity || out["detail"] != "aucun visage sur la photo" {
		t.Fatalf("photo sans visage : %d %v", code, out)
	}
	if code, _ := call("POST", "/api/people/nora/faces", photoOf("nora")); code != http.StatusNotFound {
		t.Fatalf("personne hors du foyer : %d", code)
	}
	if code, _ := call("POST", "/api/people/idris/faces", []byte("GIF89a")); code != http.StatusUnsupportedMediaType {
		t.Fatalf("pas une photo : %d", code)
	}
	if code, _ := call("POST", "/api/people/idris/faces", append(photoOf("x"), bytes.Repeat([]byte{0}, face.MaxPhoto)...)); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("photo trop grosse : %d", code)
	}
	code, out = call("GET", "/api/people", nil)
	if p := out["people"].([]any)[2].(map[string]any); code != 200 || p["id"] != "idris" || p["samples"] != 1.0 || p["name"] != "Idris" {
		t.Fatalf("liste : %d %v", code, out)
	}

	// identification
	code, out = call("POST", "/api/face/identify", photoOf("idris"))
	if p, _ := out["person"].(map[string]any); code != 200 || p["id"] != "idris" || p["name"] != "Idris" || out["score"] != 0.74 {
		t.Fatalf("reconnu : %d %v", code, out)
	}
	code, out = call("POST", "/api/face/identify", photoOf("marion"))
	if code != 200 || out["person"] != nil || out["score"] != 0.18 || out["faces"] != 1.0 {
		t.Fatalf("inconnu : %d %v", code, out)
	}
	code, out = call("POST", "/api/face/identify", photoOf(""))
	if code != 200 || out["person"] != nil || out["faces"] != 0.0 {
		t.Fatalf("aucun visage : %d %v", code, out)
	}
	fake.prints["ancien"] = 3 // known to the service, no longer in control.yaml
	if code, out = call("POST", "/api/face/identify", photoOf("ancien")); code != 200 || out["person"] != nil {
		t.Fatalf("hors du foyer : %d %v", code, out)
	}

	// CSRF: the photo endpoints are mutating
	req := httptest.NewRequest("POST", "/api/face/identify", bytes.NewReader(photoOf("idris")))
	req.AddCookie(&http.Cookie{Name: cookieName, Value: s.Token})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("sans X-Control : %d", w.Code)
	}

	// service down
	fake.down = true
	if code, out := call("POST", "/api/face/identify", photoOf("idris")); code != http.StatusBadGateway || strings.Contains(out["detail"].(string), "refusée") {
		t.Fatalf("service en panne : %d %v", code, out)
	}
	code, out = call("GET", "/api/people", nil)
	if code != 200 || out["error"] == nil || out["people"].([]any)[2].(map[string]any)["samples"] != nil {
		t.Fatalf("liste, service en panne : %d %v", code, out)
	}
	fake.down = false

	// forget
	if code, _ := call("DELETE", "/api/people/idris/faces", nil); code != 200 || fake.prints["idris"] != 0 {
		t.Fatalf("oubli : %d %v", code, fake.prints)
	}
	if code, out = call("POST", "/api/face/identify", photoOf("idris")); out["person"] != nil {
		t.Fatalf("reconnu après l'oubli : %v", out)
	}
	var texts []string
	for _, e := range js.Recent(10) {
		if e.Scope == "people" {
			texts = append(texts, e.Text)
		}
	}
	if strings.Join(texts, " | ") != "Visage oublié : Idris | Visage ajouté : Idris (photo 1)" {
		t.Fatalf("journal : %q", texts)
	}
}

func TestVoiceKnowsPersonalCalendars(t *testing.T) {
	s, _ := newTestServer(t)
	c, known := s.voiceContext(s.Home().UpdatedAt)
	if strings.Join(known, ",") != "famille,poubelles,abdelhak,marion,idris" {
		t.Fatalf("calendriers connus : %v", known)
	}
	if strings.Join(c.People, ",") != "Abdelhak,Marion,Idris" {
		t.Fatalf("personnes : %v", c.People)
	}
	if last := c.Calendars[len(c.Calendars)-1]; last.ID != "idris" || !strings.Contains(last.Hint, "Idris") {
		t.Fatalf("indice : %+v", last)
	}
}

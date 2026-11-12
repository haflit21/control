package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"ahomelab.io/control/internal/face"
	"ahomelab.io/control/internal/journal"
	"ahomelab.io/control/internal/model"
)

// The household and its faces. The people (people.go) each have a personal
// calendar; their face prints are kept by the homelab's face
// service, filled from the app (one photo at a time, taken by the device that
// will recognise them). Waking the wall tablet sends one photo to
// /api/face/identify: the screen greets the person and shows their agenda.
// Recognition only personalises the screen; it never authorises anything.
// Photos only pass through memory: never stored, never logged.

// de: « de Marion », « d’Idris ».
func de(name string) string {
	if name != "" && strings.ContainsRune("aeiouyhàâéèêëîïôûüAEIOUYHÀÂÉÈÊËÎÏÔÛÜ", []rune(name)[0]) {
		return "d’" + name
	}
	return "de " + name
}


// readPhoto reads a JPEG or PNG body (face.MaxPhoto at most).
func readPhoto(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	b, err := io.ReadAll(io.LimitReader(r.Body, face.MaxPhoto+1))
	switch {
	case err != nil:
		fail(w, http.StatusBadRequest, "Photo illisible", err.Error())
	case len(b) > face.MaxPhoto:
		fail(w, http.StatusRequestEntityTooLarge, "Photo trop grosse", "8 Mo au plus")
	case !face.Sniff(b):
		fail(w, http.StatusUnsupportedMediaType, "Photo JPEG ou PNG attendue", "")
	default:
		return b, true
	}
	return nil, false
}

func (s *Server) faceReady(w http.ResponseWriter) bool {
	if s.Face == nil {
		fail(w, http.StatusNotImplemented, "Reconnaissance de visage non configurée", "face.url dans control.yaml et FACE_API_KEY dans les secrets")
		return false
	}
	return true
}

// faceFail turns a service error into an answer: a refusal about the photo is
// shown as is (no face, two faces…), anything else is the service being away.
func faceFail(w http.ResponseWriter, title string, err error) {
	var fe *face.Error
	if errors.As(err, &fe) && fe.ForPerson() {
		code := http.StatusUnprocessableEntity
		if fe.Status == http.StatusConflict {
			code = http.StatusConflict
		}
		fail(w, code, title, fe.Message)
		return
	}
	log.Printf("visages : %s : %v", title, err)
	fail(w, http.StatusBadGateway, title, "service de reconnaissance injoignable ou en erreur")
}

type faceMatch struct {
	Person    *model.Member `json:"person"` // nil: nobody recognised (no face, unknown face, under the threshold)
	Score     float64       `json:"score"`  // likeness with the closest person (even under the threshold)
	Faces     int           `json:"faces"`
	Threshold float64       `json:"threshold"`
}

// faceIdentify: POST /api/face/identify (body = one photo) → who is it.
func (s *Server) faceIdentify(w http.ResponseWriter, r *http.Request) {
	if !s.faceReady(w) {
		return
	}
	photo, ok := readPhoto(w, r)
	if !ok {
		return
	}
	t0 := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	res, err := s.Face.Identify(ctx, photo)
	cancel()
	if err != nil {
		faceFail(w, "Reconnaissance impossible", err)
		return
	}
	out := faceMatch{Faces: res.Faces, Threshold: res.Threshold}
	if res.Best != nil {
		out.Score = res.Best.Score
	}
	who := "personne"
	if res.Match != nil {
		// an id the service knows but control.yaml does not (person removed from the household) stays unknown
		if m, ok := s.member(res.Match.ID); ok {
			out.Person = &m
			who = m.ID
		}
	} else if res.Faces == 0 {
		who = "aucun visage"
	}
	log.Printf("visages : réveil, %s (%.2f) en %s", who, out.Score, time.Since(t0).Round(time.Millisecond))
	writeJSON(w, http.StatusOK, out)
}

type personFaces struct {
	model.Member
	Samples *int `json:"samples"` // photos enrolled; nil when the service could not say
}

// peopleList: GET /api/people → the household and how many photos each has.
func (s *Server) peopleList(w http.ResponseWriter, r *http.Request) {
	out := struct {
		People []personFaces `json:"people"`
		Face   bool          `json:"face"`
		Error  string        `json:"error,omitempty"`
	}{People: []personFaces{}, Face: s.Face != nil}
	var counts map[string]int // nil: unknown (no service, or the service is away)
	if s.Face != nil {
		counts = map[string]int{}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		ps, err := s.Face.People(ctx)
		cancel()
		if err != nil {
			log.Printf("visages : liste : %v", err)
			out.Error = "Service de reconnaissance injoignable : les photos ne peuvent pas être comptées."
			counts = nil
		}
		for _, p := range ps {
			counts[p.ID] = p.Samples
		}
	}
	for _, m := range s.members() {
		pf := personFaces{Member: m}
		if counts != nil {
			n := counts[m.ID]
			pf.Samples = &n
		}
		out.People = append(out.People, pf)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) memberOr404(w http.ResponseWriter, r *http.Request) (model.Member, bool) {
	m, ok := s.member(r.PathValue("id"))
	if !ok {
		fail(w, http.StatusNotFound, "Personne inconnue", "elle a peut-être été retirée du foyer")
		return model.Member{}, false
	}
	return m, true
}

// faceEnroll: POST /api/people/{id}/faces (body = one photo) → one more print.
func (s *Server) faceEnroll(w http.ResponseWriter, r *http.Request) {
	if !s.faceReady(w) {
		return
	}
	m, ok := s.memberOr404(w, r)
	if !ok {
		return
	}
	photo, ok := readPhoto(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	e, err := s.Face.Enroll(ctx, m.ID, photo)
	cancel()
	if err != nil {
		faceFail(w, "Photo refusée", err)
		return
	}
	s.record(r, journal.Entry{Scope: "people", Target: m.ID, Action: "face_add", Text: fmt.Sprintf("Visage ajouté : %s (photo %d)", m.Name, e.Samples), OK: true})
	writeJSON(w, http.StatusOK, map[string]any{"samples": e.Samples, "own": e.Own, "message": fmt.Sprintf("Photo %d enregistrée pour %s", e.Samples, m.Name)})
}

// faceForget: DELETE /api/people/{id}/faces → every print of the person removed.
func (s *Server) faceForget(w http.ResponseWriter, r *http.Request) {
	if !s.faceReady(w) {
		return
	}
	m, ok := s.memberOr404(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	err := s.Face.Forget(ctx, m.ID)
	cancel()
	if err != nil {
		faceFail(w, "Oubli impossible", err)
		return
	}
	s.record(r, journal.Entry{Scope: "people", Target: m.ID, Action: "face_forget", Text: "Visage oublié : " + m.Name, OK: true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Le visage " + de(m.Name) + " est oublié"})
}

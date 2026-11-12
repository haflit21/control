package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"ahomelab.io/control/internal/config"
	"ahomelab.io/control/internal/household"
	"ahomelab.io/control/internal/journal"
	"ahomelab.io/control/internal/model"
)

// The household, edited from Réglages › Personnes et visages. control.yaml's
// people: is the starting list; household.Store keeps the changes. Each person
// has a personal calendar after the shared ones and, with the face service,
// face prints. Removing someone also removes their calendar's events and their
// prints.

func seedPeople(cfg *config.Config) []household.Member {
	out := make([]household.Member, 0, len(cfg.People))
	for _, m := range cfg.People {
		out = append(out, household.Member{ID: m.ID, Name: m.Name, Color: m.Color})
	}
	return out
}

// SeedPeople: control.yaml's people, the list a new household store starts from.
func SeedPeople(cfg *config.Config) []household.Member { return seedPeople(cfg) }

func (s *Server) members() []model.Member {
	ps := s.People.List()
	out := make([]model.Member, 0, len(ps))
	for _, m := range ps {
		out = append(out, model.Member{ID: m.ID, Name: m.Name, Color: m.Color})
	}
	return out
}

func (s *Server) member(id string) (model.Member, bool) {
	m, ok := s.People.Get(id)
	return model.Member{ID: m.ID, Name: m.Name, Color: m.Color}, ok
}

// calendars: the shared ones (control.yaml), then one per person.
func (s *Server) calendars() []config.Calendar {
	out := s.Cfg.Agenda.Shared()
	for _, m := range s.People.List() {
		out = append(out, config.PersonalCalendar(config.Member{ID: m.ID, Name: m.Name, Color: m.Color}))
	}
	return out
}

func (s *Server) calendarIDs() []string {
	cs := s.calendars()
	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.ID)
	}
	return ids
}

// peopleChanged republishes what depends on the household: home.people and the
// agenda's calendars.
func (s *Server) peopleChanged() {
	if s.Agenda != nil {
		ag := s.AgendaModel(time.Now().In(s.Loc))
		s.mu.Lock()
		s.agenda = &ag
		s.mu.Unlock()
	}
	s.rebuild(s.publish)
	s.publishHome()
}

// peopleAdd: POST /api/people {name, color} → the new person.
func (s *Server) peopleAdd(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "Requête illisible", err.Error())
		return
	}
	shared := map[string]bool{}
	for _, c := range s.Cfg.Agenda.Shared() {
		shared[c.ID] = true
	}
	m, err := s.People.Add(in.Name, in.Color, func(id string) bool { return shared[id] })
	if err != nil {
		fail(w, http.StatusBadRequest, capitalize(err.Error()), "")
		return
	}
	s.peopleChanged()
	s.record(r, journal.Entry{Scope: "people", Target: m.ID, Action: "add", Text: "Personne ajoutée : " + m.Name, OK: true})
	writeJSON(w, http.StatusOK, map[string]any{"person": model.Member{ID: m.ID, Name: m.Name, Color: m.Color}, "message": m.Name + " fait partie du foyer"})
}

// peopleRemove: DELETE /api/people/{id} → the person, their personal agenda
// and their face prints are gone.
func (s *Server) peopleRemove(w http.ResponseWriter, r *http.Request) {
	m, ok := s.member(r.PathValue("id"))
	if !ok {
		fail(w, http.StatusNotFound, "Personne inconnue", "")
		return
	}
	if _, err := s.People.Remove(m.ID); err != nil {
		if errors.Is(err, household.ErrNotFound) {
			fail(w, http.StatusNotFound, "Personne inconnue", "")
			return
		}
		fail(w, http.StatusInternalServerError, "Suppression impossible", err.Error())
		return
	}
	msg := m.Name + " ne fait plus partie du foyer"
	if s.Agenda != nil {
		if n, err := s.Agenda.DeleteCalendar(m.ID); err != nil {
			log.Printf("foyer : agenda de %s : %v", m.ID, err)
			msg += " · son agenda n’a pas pu être vidé"
		} else if n > 0 {
			msg += fmt.Sprintf(" · %d événement%s supprimé%s", n, plural(n), plural(n))
		}
	}
	if s.Face != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		err := s.Face.Forget(ctx, m.ID)
		cancel()
		if err != nil {
			log.Printf("foyer : visage de %s : %v", m.ID, err)
			msg += " · photos non effacées (service de reconnaissance injoignable)"
		}
	}
	s.peopleChanged()
	s.record(r, journal.Entry{Scope: "people", Target: m.ID, Action: "remove", Text: "Personne retirée : " + m.Name, OK: true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func plural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

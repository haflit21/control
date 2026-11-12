package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"

	"ahomelab.io/control/internal/agenda"
	"ahomelab.io/control/internal/journal"
	"ahomelab.io/control/internal/model"
	"ahomelab.io/control/internal/voice"
)

// The family calendar: kept by the app (agenda.Store), published to every
// screen as home.agenda (current week → +5 weeks), edited from the form, and
// filled by voice or a typed sentence through the voice package.

const agendaWeeks = 5

// runAgenda republishes the agenda when the store changes, and every 10 min
// (so the window follows the days even in a quiet house).
func (s *Server) runAgenda(ctx context.Context) {
	if s.Agenda == nil {
		return
	}
	s.refreshAgenda()
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.kick():
		case <-t.C:
		}
		s.refreshAgenda()
	}
}

func (s *Server) kick() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agendaKick == nil {
		s.agendaKick = make(chan struct{}, 1)
	}
	return s.agendaKick
}

// AgendaChanged is the store's OnChange: wake runAgenda (non-blocking).
func (s *Server) AgendaChanged() {
	select {
	case s.kick() <- struct{}{}:
	default:
	}
}

// AgendaModel builds what the screens get: calendars, occurrences, input modes.
func (s *Server) AgendaModel(now time.Time) model.Agenda {
	from := now
	for from.Weekday() != time.Monday {
		from = from.AddDate(0, 0, -1)
	}
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, s.Loc)
	to := from.AddDate(0, 0, 7*agendaWeeks)
	ag := model.Agenda{Available: true, From: from, To: to, UpdatedAt: now, Calendars: []model.Calendar{}, Events: []model.CalEvent{},
		Input: model.AgendaInput{Text: s.Understander != nil, Audio: s.Understander != nil && s.Transcriber != nil}}
	known := map[string]bool{}
	for _, c := range s.calendars() {
		known[c.ID] = true
		ag.Calendars = append(ag.Calendars, model.Calendar{ID: c.ID, Name: c.Name, Color: c.Color, Owner: c.Owner})
	}
	for _, o := range s.Agenda.Between(from, to, s.Loc) {
		if !known[o.Event.Calendar] {
			continue
		}
		ag.Events = append(ag.Events, model.CalEvent{ID: o.ID, EventID: o.Event.ID, Repeats: o.Repeats, Calendar: o.Event.Calendar, Title: o.Event.Title,
			Start: o.Start, End: o.End, AllDay: o.AllDay, Location: o.Event.Location})
	}
	return ag
}

func (s *Server) refreshAgenda() {
	ag := s.AgendaModel(time.Now().In(s.Loc))
	s.mu.Lock()
	changed := s.agenda == nil || !reflect.DeepEqual(s.agenda.Events, ag.Events) || !reflect.DeepEqual(s.agenda.Calendars, ag.Calendars) || s.agenda.From != ag.From || s.agenda.Input != ag.Input
	if changed {
		s.agenda = &ag
	}
	s.mu.Unlock()
	if changed {
		s.rebuild(s.publish)
		s.publishHome()
	}
}

// ---------- CRUD ----------

func (s *Server) agendaReady(w http.ResponseWriter) bool {
	if s.Agenda == nil {
		fail(w, http.StatusServiceUnavailable, "Agenda indisponible", "le dossier d'état n'est pas accessible en écriture")
		return false
	}
	return true
}

func (s *Server) agendaGet(w http.ResponseWriter, r *http.Request) {
	if !s.agendaReady(w) {
		return
	}
	e, ok := s.Agenda.Get(r.PathValue("id"))
	if !ok {
		fail(w, http.StatusNotFound, "Événement introuvable", "")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) readEvent(w http.ResponseWriter, r *http.Request) (agenda.Event, bool) {
	var e agenda.Event
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&e); err != nil {
		fail(w, http.StatusBadRequest, "Requête illisible", err.Error())
		return e, false
	}
	if err := e.Validate(s.calendarIDs()); err != nil {
		fail(w, http.StatusBadRequest, capitalize(err.Error()), "")
		return e, false
	}
	return e, true
}

func (s *Server) agendaCreate(w http.ResponseWriter, r *http.Request) {
	if !s.agendaReady(w) {
		return
	}
	e, ok := s.readEvent(w, r)
	if !ok {
		return
	}
	saved, err := s.Agenda.Add(e, s.userOf(r))
	if err != nil {
		fail(w, http.StatusInternalServerError, "Événement non enregistré", err.Error())
		return
	}
	s.recordAgenda(r, "add", saved, "Événement ajouté : ")
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "message": "Ajouté à l'agenda", "event": saved})
}

func (s *Server) agendaUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.agendaReady(w) {
		return
	}
	e, ok := s.readEvent(w, r)
	if !ok {
		return
	}
	saved, err := s.Agenda.Update(r.PathValue("id"), e)
	if errors.Is(err, agenda.ErrNotFound) {
		fail(w, http.StatusNotFound, "Événement introuvable", "il a peut-être été supprimé depuis un autre écran")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "Événement non enregistré", err.Error())
		return
	}
	s.recordAgenda(r, "update", saved, "Événement modifié : ")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Événement modifié", "event": saved})
}

func (s *Server) agendaDelete(w http.ResponseWriter, r *http.Request) {
	if !s.agendaReady(w) {
		return
	}
	old, err := s.Agenda.Delete(r.PathValue("id"))
	if errors.Is(err, agenda.ErrNotFound) {
		fail(w, http.StatusNotFound, "Événement introuvable", "")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "Suppression impossible", err.Error())
		return
	}
	s.recordAgenda(r, "delete", old, "Événement supprimé : ")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Événement supprimé"})
}

func (s *Server) recordAgenda(r *http.Request, action string, e agenda.Event, prefix string) {
	when := e.Date
	if d, err := time.Parse("2006-01-02", e.Date); err == nil {
		when = d.Format("02/01")
	}
	if !e.AllDay {
		when += " " + e.Start
	}
	if e.Repeat != nil {
		when += " · " + map[string]string{agenda.Weekly: "chaque semaine", agenda.Biweekly: "toutes les 2 semaines", agenda.Monthly: "chaque mois", agenda.Yearly: "chaque année"}[e.Repeat.Freq]
	}
	s.record(r, journal.Entry{Scope: "agenda", Target: e.ID, Action: action, Text: prefix + e.Title + " (" + when + ")", OK: true})
}

// ---------- quick entry: typed sentence or voice ----------

func (s *Server) voiceContext(now time.Time) (voice.Context, []string) {
	c := voice.Context{Now: now}
	for _, cal := range s.calendars() {
		c.Calendars = append(c.Calendars, voice.CalendarInfo{ID: cal.ID, Name: cal.Name, Hint: cal.Hint})
	}
	seen := map[string]bool{}
	for _, p := range s.Cfg.Globals.Presence {
		if p.Name != "" && !seen[p.Name] {
			seen[p.Name] = true
			c.People = append(c.People, p.Name)
		}
	}
	for _, m := range s.People.List() {
		if !seen[m.Name] {
			seen[m.Name] = true
			c.People = append(c.People, m.Name)
		}
	}
	return c, s.calendarIDs()
}

func (s *Server) understand(ctx context.Context, text string) (voice.Draft, error) {
	vc, known := s.voiceContext(time.Now().In(s.Loc))
	uctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	d, err := s.Understander.Understand(uctx, text, vc)
	if err != nil {
		return voice.Draft{}, err
	}
	d.Transcript = text
	return voice.Normalize(d, vc, known), nil
}

// agendaParse: POST /api/agenda/parse {text} → {draft}. Nothing is saved.
func (s *Server) agendaParse(w http.ResponseWriter, r *http.Request) {
	if s.Understander == nil {
		fail(w, http.StatusNotImplemented, "Saisie rapide non configurée", "agenda.voice.understand dans control.yaml")
		return
	}
	var q struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&q); err != nil || strings.TrimSpace(q.Text) == "" {
		fail(w, http.StatusBadRequest, "Phrase vide", "")
		return
	}
	t0 := time.Now()
	d, err := s.understand(r.Context(), strings.TrimSpace(q.Text))
	if err != nil {
		log.Printf("agenda : compréhension impossible : %v", err)
		fail(w, http.StatusBadGateway, "Phrase non comprise", err.Error())
		return
	}
	log.Printf("agenda : phrase comprise en %s", time.Since(t0).Round(100*time.Millisecond))
	writeJSON(w, http.StatusOK, map[string]any{"draft": d})
}

const maxAudio = 8 << 20 // ~ 1 min of compressed speech, far more than a sentence

// agendaVoice: POST /api/agenda/voice (body = the recording, Content-Type audio/…)
// → {draft}. The audio is only held in memory for the request, never stored.
func (s *Server) agendaVoice(w http.ResponseWriter, r *http.Request) {
	if s.Transcriber == nil || s.Understander == nil {
		fail(w, http.StatusNotImplemented, "Saisie vocale non configurée", "agenda.voice.transcribe et understand dans control.yaml")
		return
	}
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "audio/") && !strings.HasPrefix(ct, "video/") { // Safari labels m4a recordings video/mp4 at times
		fail(w, http.StatusUnsupportedMediaType, "Format audio attendu", ct)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxAudio+1))
	if err != nil {
		fail(w, http.StatusBadRequest, "Enregistrement illisible", err.Error())
		return
	}
	if len(body) > maxAudio {
		fail(w, http.StatusRequestEntityTooLarge, "Enregistrement trop long", "une phrase suffit")
		return
	}
	if len(body) < 200 {
		fail(w, http.StatusBadRequest, "Enregistrement vide", "")
		return
	}
	t0 := time.Now()
	tctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	text, err := s.Transcriber.Transcribe(tctx, bytes.NewReader(body), ct)
	cancel()
	if err != nil {
		log.Printf("agenda : transcription impossible : %v", err)
		fail(w, http.StatusBadGateway, "Transcription impossible", err.Error())
		return
	}
	if strings.TrimSpace(text) == "" {
		fail(w, http.StatusUnprocessableEntity, "Rien entendu", "réessaie un peu plus près du micro")
		return
	}
	t1 := time.Now()
	d, err := s.understand(r.Context(), text)
	if err != nil {
		log.Printf("agenda : compréhension impossible : %v", err)
		// the transcript is still useful: the form opens with it as the title
		writeJSON(w, http.StatusOK, map[string]any{"draft": voice.Normalize(voice.Draft{Transcript: text}, mustCtx(s), s.calendarIDs()), "warning": "Phrase entendue mais pas comprise : " + err.Error()})
		return
	}
	log.Printf("agenda : voix transcrite en %s, comprise en %s", t1.Sub(t0).Round(100*time.Millisecond), time.Since(t1).Round(100*time.Millisecond))
	writeJSON(w, http.StatusOK, map[string]any{"draft": d})
}

func mustCtx(s *Server) voice.Context {
	c, _ := s.voiceContext(time.Now().In(s.Loc))
	return c
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// Package voice turns a sentence ("dentiste de Marion jeudi prochain à 10h")
// into a draft event that the interface shows in a pre-filled form.
//
// Two independent steps, each behind an interface so a provider can be swapped
// by configuration:
//   - Transcriber: audio → text. OpenAI-compatible /v1/audio/transcriptions
//     (speaches / faster-whisper on the NAS, whisper.cpp, OpenAI…).
//   - Understander: text → draft. Anthropic Messages API, or any
//     OpenAI-compatible chat endpoint (Ollama, llama.cpp, vLLM…) for a local model.
//
// Nothing here writes to the calendar: the person confirms the form.
package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"ahomelab.io/maison/internal/agenda"
)

// Draft is what a sentence was understood as. Same fields as agenda.Event.
type Draft struct {
	Title      string         `json:"title"`
	Calendar   string         `json:"calendar,omitempty"`
	Location   string         `json:"location,omitempty"`
	AllDay     bool           `json:"allDay"`
	Date       string         `json:"date"`
	EndDate    string         `json:"endDate,omitempty"`
	Start      string         `json:"start,omitempty"`
	End        string         `json:"end,omitempty"`
	Repeat     *agenda.Repeat `json:"repeat,omitempty"`
	Transcript string         `json:"transcript,omitempty"` // what was heard / typed
}

type Transcriber interface {
	Transcribe(ctx context.Context, audio io.Reader, contentType string) (string, error)
}

type Understander interface {
	Understand(ctx context.Context, text string, c Context) (Draft, error)
}

// CalendarInfo is what the model needs to know about one calendar.
type CalendarInfo struct {
	ID, Name, Hint string
}

// Context gives the model "now" and the calendars to choose from.
type Context struct {
	Now       time.Time // in the house's time zone
	Calendars []CalendarInfo
	People    []string // first names of the household (help for "Marion", "chez nous"…)
}

var frDays = []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}
var frMonths = []string{"", "janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}

func frDate(t time.Time) string {
	return fmt.Sprintf("%s %d %s %d", frDays[t.Weekday()], t.Day(), frMonths[t.Month()], t.Year())
}

// SystemPrompt is shared by every provider so they all answer the same way.
func SystemPrompt(c Context) string {
	var b strings.Builder
	now := c.Now
	fmt.Fprintf(&b, "Tu transformes une phrase en français, dictée ou tapée pour l'agenda familial, en un événement.\n")
	fmt.Fprintf(&b, "Nous sommes le %s, il est %s (heure de Paris).\n\n", frDate(now), now.Format("15:04"))
	b.WriteString("Les 21 prochains jours, pour résoudre « jeudi », « jeudi prochain », « dans 10 jours »… :\n")
	for i := 0; i < 21; i++ {
		d := now.AddDate(0, 0, i)
		label := ""
		switch i {
		case 0:
			label = " (aujourd'hui)"
		case 1:
			label = " (demain)"
		case 2:
			label = " (après-demain)"
		}
		fmt.Fprintf(&b, "- %s = %s%s\n", frDate(d), d.Format("2006-01-02"), label)
	}
	b.WriteString("\nCalendriers (champ calendar) :\n")
	for _, cal := range c.Calendars {
		fmt.Fprintf(&b, "- %s : %s", cal.ID, cal.Name)
		if cal.Hint != "" {
			fmt.Fprintf(&b, " (%s)", cal.Hint)
		}
		b.WriteString("\n")
	}
	if len(c.People) > 0 {
		fmt.Fprintf(&b, "\nPersonnes du foyer : %s.\n", strings.Join(c.People, ", "))
	}
	b.WriteString(`
Règles :
- title : court, avec majuscule, sans la date ni l'heure (« Dentiste · Marion », « Poubelle jaune », « Anniversaire de Nora »). Garde les prénoms.
- date : AAAA-MM-JJ. « jeudi » = le prochain jeudi à venir (aujourd'hui compris si c'est jeudi et que l'heure n'est pas passée) ; « jeudi prochain » = idem sauf si on est mercredi ou jeudi, alors celui de la semaine suivante. Une date du calendrier déjà passée cette année (« le 3 mars ») = l'année suivante. Sans aucune date : aujourd'hui.
- start / end : HH:MM sur 24 h. « 10h » = 10:00, « 14h30 » = 14:30, « midi » = 12:00, « le matin » = 09:00, « l'après-midi » = 14:00, « le soir » = 19:00. Sans heure de fin : une heure après le début (30 minutes pour un rendez-vous médical).
- allDay : true s'il n'y a aucune heure (anniversaires, poubelles, vacances, « toute la journée »). Alors start et end sont vides. Plusieurs jours (« du 20 au 23 ») : endDate = dernier jour inclus.
- repeat : seulement si la phrase le dit (« tous les mardis », « chaque semaine » → weekly ; « une semaine sur deux », « toutes les deux semaines » → biweekly ; « tous les mois », « le 5 de chaque mois » → monthly ; « tous les ans », un anniversaire → yearly). until (AAAA-MM-JJ) seulement si une fin est dite. date = la première occurrence à venir.
- location : seulement si un lieu est dit (« chez le Dr Martin », « à la piscine »).
- calendar : l'identifiant qui convient le mieux ; à défaut le premier.
Si la phrase n'est pas un événement, mets quand même ton meilleur titre.
`)
	return b.String()
}

// Schema is the JSON schema of the answer (tool input for Anthropic, instructions for others).
var Schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"title":    map[string]any{"type": "string"},
		"calendar": map[string]any{"type": "string"},
		"location": map[string]any{"type": "string"},
		"allDay":   map[string]any{"type": "boolean"},
		"date":     map[string]any{"type": "string", "description": "AAAA-MM-JJ"},
		"endDate":  map[string]any{"type": "string", "description": "AAAA-MM-JJ, journées entières sur plusieurs jours"},
		"start":    map[string]any{"type": "string", "description": "HH:MM"},
		"end":      map[string]any{"type": "string", "description": "HH:MM"},
		"repeat": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"freq":  map[string]any{"type": "string", "enum": []string{"weekly", "biweekly", "monthly", "yearly"}},
				"until": map[string]any{"type": "string", "description": "AAAA-MM-JJ"},
			},
			"required": []string{"freq"},
		},
	},
	"required": []string{"title", "allDay", "date"},
}

// Normalize turns whatever the model answered into something the form can show:
// valid date, times, calendar. It never fails — a bad field is reset, and the
// person sees and fixes it in the form.
func Normalize(d Draft, c Context, known []string) Draft {
	d.Title = strings.TrimSpace(d.Title)
	if d.Title == "" {
		d.Title = strings.TrimSpace(d.Transcript)
		if r := []rune(d.Title); len(r) > 80 {
			d.Title = string(r[:80])
		}
	}
	if _, err := time.Parse("2006-01-02", d.Date); err != nil {
		d.Date = c.Now.Format("2006-01-02")
	}
	normTime := func(s string) string {
		s = strings.TrimSpace(strings.ReplaceAll(strings.ToLower(s), "h", ":"))
		if strings.HasSuffix(s, ":") {
			s += "00"
		}
		t, err := time.Parse("15:04", s)
		if err != nil {
			return ""
		}
		return t.Format("15:04")
	}
	d.Start, d.End = normTime(d.Start), normTime(d.End)
	if d.Start == "" {
		d.AllDay, d.End = true, ""
	}
	if d.AllDay {
		d.Start, d.End = "", ""
		if _, err := time.Parse("2006-01-02", d.EndDate); err != nil || d.EndDate <= d.Date {
			d.EndDate = ""
		}
	} else {
		d.EndDate = ""
	}
	ok := false
	for _, k := range known {
		ok = ok || k == d.Calendar
	}
	if !ok && len(known) > 0 {
		d.Calendar = known[0]
	}
	if d.Repeat != nil {
		switch d.Repeat.Freq {
		case agenda.Weekly, agenda.Biweekly, agenda.Monthly, agenda.Yearly:
			if _, err := time.Parse("2006-01-02", d.Repeat.Until); err != nil || d.Repeat.Until < d.Date {
				d.Repeat.Until = ""
			}
		default:
			d.Repeat = nil
		}
	}
	return d
}

// decodeDraft reads a JSON object out of a model's text answer (tolerates ```json fences and prose around it).
func decodeDraft(s string) (Draft, error) {
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j <= i {
		return Draft{}, errors.New("réponse sans objet JSON")
	}
	var d Draft
	if err := json.Unmarshal([]byte(s[i:j+1]), &d); err != nil {
		return Draft{}, fmt.Errorf("réponse illisible : %w", err)
	}
	return d, nil
}

// clip keeps an error body short for logs and toasts.
func clip(b []byte) string {
	s := strings.TrimSpace(string(b))
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300]) + "…"
	}
	return s
}

package voice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ahomelab.io/maison/internal/agenda"
)

var paris, _ = time.LoadLocation("Europe/Paris")

func ctxNow() Context {
	return Context{
		Now:       time.Date(2026, 11, 9, 15, 20, 0, 0, paris), // a Monday
		Calendars: []CalendarInfo{{ID: "famille", Name: "Famille"}, {ID: "poubelles", Name: "Poubelles", Hint: "tri, verre"}},
		People:    []string{"Abdelhak", "Marion"},
	}
}

func TestSystemPrompt(t *testing.T) {
	p := SystemPrompt(ctxNow())
	for _, want := range []string{"lundi 9 novembre 2026", "15:20", "jeudi 12 novembre 2026 = 2026-11-12", "mardi 10 novembre 2026 = 2026-11-10 (demain)", "poubelles : Poubelles (tri, verre)", "Abdelhak, Marion"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt sans %q", want)
		}
	}
}

func TestNormalize(t *testing.T) {
	c := ctxNow()
	known := []string{"famille", "poubelles"}
	d := Normalize(Draft{Title: " Dentiste · Marion ", Date: "2026-11-12", Start: "10h", End: "10h30", Calendar: "boulot", Repeat: &agenda.Repeat{Freq: "daily"}}, c, known)
	if d.Title != "Dentiste · Marion" || d.Start != "10:00" || d.End != "10:30" || d.Calendar != "famille" || d.Repeat != nil || d.AllDay {
		t.Fatalf("normalisé : %+v", d)
	}
	d = Normalize(Draft{Transcript: "anniversaire de Nora", Date: "n'importe", Start: "", Repeat: &agenda.Repeat{Freq: "yearly", Until: "2020-01-01"}}, c, known)
	if d.Title != "anniversaire de Nora" || d.Date != "2026-11-09" || !d.AllDay || d.Repeat == nil || d.Repeat.Until != "" {
		t.Fatalf("défauts : %+v", d)
	}
	d = Normalize(Draft{Title: "Vacances", AllDay: true, Date: "2026-12-19", EndDate: "2026-12-10", Start: "09:00"}, c, known)
	if d.EndDate != "" || d.Start != "" {
		t.Fatalf("journée entière : %+v", d)
	}
}

func TestDecodeDraft(t *testing.T) {
	d, err := decodeDraft("Voici :\n```json\n{\"title\":\"Courses\",\"allDay\":false,\"date\":\"2026-11-14\",\"start\":\"10:00\"}\n```")
	if err != nil || d.Title != "Courses" || d.Start != "10:00" {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := decodeDraft("pas de json"); err == nil {
		t.Fatal("texte sans JSON accepté")
	}
}

func TestOpenAITranscriber(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Errorf("chemin %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		f, h, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(f)
		if string(b) != "AUDIO" || h.Filename != "audio.m4a" || r.FormValue("model") != "Systran/faster-whisper-small" || r.FormValue("language") != "fr" {
			t.Errorf("formulaire : %q %s %s %s", b, h.Filename, r.FormValue("model"), r.FormValue("language"))
		}
		_, _ = w.Write([]byte(`{"text":" Dentiste de Marion jeudi à 10h. "}`))
	}))
	defer srv.Close()
	for _, base := range []string{srv.URL, srv.URL + "/v1", srv.URL + "/v1/audio/transcriptions"} {
		tr := &OpenAITranscriber{URL: base, Model: "Systran/faster-whisper-small", Language: "fr"}
		txt, err := tr.Transcribe(context.Background(), strings.NewReader("AUDIO"), "audio/mp4")
		if err != nil || txt != "Dentiste de Marion jeudi à 10h." {
			t.Fatalf("%s : %q %v", base, txt, err)
		}
	}
}

func TestAnthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("requête : %s %v", r.URL.Path, r.Header)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "claude-haiku-5-5" || !strings.Contains(body["system"].(string), "2026-11-12") {
			t.Errorf("corps : model=%v", body["model"])
		}
		tc := body["tool_choice"].(map[string]any)
		if tc["type"] != "tool" || tc["name"] != "evenement" {
			t.Errorf("tool_choice : %v", tc)
		}
		msgs := body["messages"].([]any)
		if msgs[0].(map[string]any)["content"] != "dentiste de Marion jeudi à 10h" {
			t.Errorf("message : %v", msgs)
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"tool_use","name":"evenement","input":{"title":"Dentiste · Marion","calendar":"famille","allDay":false,"date":"2026-11-12","start":"10:00","end":"10:30"}}],"stop_reason":"tool_use"}`))
	}))
	defer srv.Close()
	a := &Anthropic{APIKey: "k", Model: "claude-haiku-5-5", URL: srv.URL}
	d, err := a.Understand(context.Background(), "dentiste de Marion jeudi à 10h", ctxNow())
	if err != nil || d.Title != "Dentiste · Marion" || d.Date != "2026-11-12" || d.End != "10:30" {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := (&Anthropic{Model: "x", URL: srv.URL}).Understand(context.Background(), "x", ctxNow()); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("sans clé : %v", err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer bad.Close()
	if _, err := (&Anthropic{APIKey: "x", Model: "x", URL: bad.URL}).Understand(context.Background(), "x", ctxNow()); err == nil || !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Fatalf("erreur API : %v", err)
	}
}

func TestOpenAIChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("chemin %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "qwen2.5:3b" {
			t.Errorf("modèle %v", body["model"])
		}
		sys := body["messages"].([]any)[0].(map[string]any)["content"].(string)
		if !strings.Contains(sys, `"required":["title","allDay","date"]`) {
			t.Errorf("schéma absent du prompt système")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"title\":\"Poubelle jaune\",\"calendar\":\"poubelles\",\"allDay\":true,\"date\":\"2026-11-10\",\"repeat\":{\"freq\":\"weekly\"}}"}}]}`))
	}))
	defer srv.Close()
	o := &OpenAIChat{URL: srv.URL + "/v1", Model: "qwen2.5:3b"}
	d, err := o.Understand(context.Background(), "la poubelle jaune tous les mardis", ctxNow())
	if err != nil || d.Calendar != "poubelles" || d.Repeat == nil || d.Repeat.Freq != "weekly" || !d.AllDay {
		t.Fatalf("%+v %v", d, err)
	}
}

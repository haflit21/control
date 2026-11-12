package face

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = append(got, r.Method+" "+r.URL.EscapedPath()+" "+r.Header.Get("Authorization")+" "+string(body))
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/identify":
			_, _ = io.WriteString(w, `{"faces":1,"match":{"id":"idris","score":0.71},"best":{"id":"idris","score":0.71},"second":{"id":"abdelhak","score":0.12},"threshold":0.363,"ms":24.5,"model":"sface"}`)
		case "POST /v1/people/idris/samples":
			_, _ = io.WriteString(w, `{"id":"idris","samples":2,"own":0.8}`)
		case "POST /v1/people/marion/samples":
			w.WriteHeader(422)
			_, _ = io.WriteString(w, `{"error":{"message":"aucun visage sur la photo","type":"invalid_request_error"},"faces":0}`)
		case "DELETE /v1/people/idris":
			_, _ = io.WriteString(w, `{"id":"idris","removed":true}`)
		case "DELETE /v1/people/marion":
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"error":{"message":"personne inconnue"}}`)
		case "GET /v1/people":
			_, _ = io.WriteString(w, `{"people":[{"id":"idris","samples":2}],"model":"sface","threshold":0.363}`)
		case "GET /health":
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		default:
			w.WriteHeader(500)
			_, _ = io.WriteString(w, "boum")
		}
	}))
	defer srv.Close()
	c := &Client{URL: srv.URL + "/", Token: "jeton"}
	ctx := context.Background()

	r, err := c.Identify(ctx, []byte("photo"))
	if err != nil || r.Match == nil || r.Match.ID != "idris" || r.Second.ID != "abdelhak" || r.Faces != 1 || r.Threshold != 0.363 {
		t.Fatalf("identify : %+v %v", r, err)
	}
	if got[0] != "POST /v1/identify Bearer jeton photo" {
		t.Fatalf("requête : %q", got[0])
	}
	e, err := c.Enroll(ctx, "idris", []byte("p"))
	if err != nil || e.Samples != 2 || e.Own == nil || *e.Own != 0.8 {
		t.Fatalf("enroll : %+v %v", e, err)
	}
	_, err = c.Enroll(ctx, "marion", []byte("p"))
	var fe *Error
	if !errors.As(err, &fe) || fe.Status != 422 || fe.Message != "aucun visage sur la photo" || !fe.ForPerson() {
		t.Fatalf("refus : %v", err)
	}
	if err := c.Forget(ctx, "idris"); err != nil {
		t.Fatal(err)
	}
	if err := c.Forget(ctx, "marion"); err != nil { // nothing to forget is not an error
		t.Fatal(err)
	}
	p, err := c.People(ctx)
	if err != nil || len(p) != 1 || p[0].Samples != 2 {
		t.Fatalf("people : %+v %v", p, err)
	}
	if code, err := c.Health(ctx); err != nil || code != 200 {
		t.Fatalf("health : %d %v", code, err)
	}
	_, err = c.Enroll(ctx, "a/../b", []byte("p"))
	if !errors.As(err, &fe) || fe.Status != 500 || fe.ForPerson() {
		t.Fatalf("id échappé, erreur serveur : %v", err)
	}
	if want := "POST /v1/people/a%2F..%2Fb/samples Bearer jeton p"; got[len(got)-1] != want {
		t.Fatalf("échappement : %q", got[len(got)-1])
	}
	if _, err := (&Client{URL: "http://127.0.0.1:1"}).Identify(ctx, []byte("p")); err == nil {
		t.Fatal("service injoignable : pas d'erreur")
	}
}

func TestSniff(t *testing.T) {
	for b, want := range map[string]bool{"\xff\xd8\xff\xe0": true, "\x89PNG\r\n\x1a\n..": true, "GIF89a": false, "": false, "<svg": false} {
		if Sniff([]byte(b)) != want {
			t.Errorf("Sniff(%q) != %v", b, want)
		}
	}
}

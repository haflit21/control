package config

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func parse(t *testing.T, text string) (*Config, error) {
	t.Helper()
	var c Config
	if err := yaml.Unmarshal([]byte(text), &c); err != nil {
		t.Fatal(err)
	}
	return &c, c.Validate()
}

func TestPeopleCalendars(t *testing.T) {
	c, err := parse(t, `
agenda:
  calendars:
    - { id: famille, name: Famille }
    - { id: poubelles, name: Poubelles }
people:
  - { id: abdelhak, name: Abdelhak, color: "#7aa7e0" }
  - { id: idris }
face:
  url: http://face.home.arpa
`)
	if err != nil {
		t.Fatal(err)
	}
	ids := strings.Join(c.Agenda.CalendarIDs(), ",")
	if ids != "famille,poubelles,abdelhak,idris" {
		t.Fatalf("calendriers : %s", ids)
	}
	idris := c.Agenda.Calendars[3]
	if idris.Owner != "idris" || idris.Name != "Idris" || !strings.Contains(idris.Hint, "Idris") {
		t.Fatalf("agenda personnel : %+v", idris)
	}
	if c.Agenda.Calendars[2].Color != "#7aa7e0" || c.Agenda.Calendars[0].Owner != "" {
		t.Fatalf("couleur ou propriétaire : %+v", c.Agenda.Calendars)
	}
	if m, ok := c.Member("abdelhak"); !ok || m.Name != "Abdelhak" {
		t.Fatalf("Member : %+v %v", m, ok)
	}
	if _, ok := c.Member("nora"); ok {
		t.Fatal("personne inconnue trouvée")
	}
	// a second Validate (tests, reload) does not add the personal calendars twice
	if err := c.Validate(); err != nil || len(c.Agenda.Calendars) != 4 {
		t.Fatalf("deuxième Validate : %v, %d calendriers", err, len(c.Agenda.Calendars))
	}
}

func TestPeopleRejected(t *testing.T) {
	for name, text := range map[string]string{
		"majuscules":     "people: [{ id: Abdelhak }]",
		"espace":         "people: [{ id: \"a b\" }]",
		"vide":           "people: [{ name: Marion }]",
		"doublon":        "people: [{ id: marion }, { id: marion }]",
		"calendrier":     "agenda: { calendars: [{ id: famille }] }\npeople: [{ id: famille }]",
		"url du service": "face: { url: face.home.arpa }",
	} {
		if _, err := parse(t, text); err == nil {
			t.Errorf("%s : accepté", name)
		}
	}
}

func TestRepoConfig(t *testing.T) {
	c, err := Load("../../../control.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.People) == 0 || c.Face.URL == "" {
		t.Fatalf("people ou face absents de control.yaml : %d, %q", len(c.People), c.Face.URL)
	}
	personal := c.Agenda.Calendars[len(c.Agenda.Calendars)-len(c.People):]
	for i, m := range c.People {
		if personal[i].ID != m.ID || personal[i].Owner != m.ID {
			t.Fatalf("agenda personnel de %s : %+v", m.ID, personal[i])
		}
	}
}

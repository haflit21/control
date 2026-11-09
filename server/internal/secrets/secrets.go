// Package secrets loads the service's secrets from a SOPS file committed in
// the repository (secrets.enc.yaml, values encrypted with age).
//
// The file is decrypted at start by the sops binary with an age private key
// (SOPS_AGE_KEY_FILE, on the NAS /run/secrets/age.key mounted read-only;
// on the Mac sops finds the owner's key by itself), and each top-level key
// becomes an environment variable of this process (HA_TOKEN,
// CLAUDE_GATEWAY_TOKEN…). Nothing is written to disk, nothing appears in
// Portainer or `docker inspect`. Same scheme as claude-gateway and Sentinel.
package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

var validName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Load decrypts path and sets its values in the environment, except the
// names keep says to leave alone (set explicitly by the caller, e.g.
// CONTROL_TOKEN=dev in the Makefile; nil: none). A missing file is not an
// error (nothing to load: secrets come from elsewhere); a file that cannot
// be decrypted is. It returns the names it set, never the values.
func Load(path, sopsBin string, keep func(name string) bool) ([]string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if sopsBin == "" {
		sopsBin = "sops"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, sopsBin, "decrypt", "--output-type", "json", path)
	cmd.Env = append(os.Environ(), "SOPS_DISABLE_VERSION_CHECK=1")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("déchiffrement de %s : %v : %s", path, err, firstLine(errb.String()))
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		return nil, fmt.Errorf("%s : contenu illisible (attendu : NOM: valeur)", path)
	}
	var names []string
	for k, v := range m {
		if k == "sops" {
			continue
		}
		s, ok := v.(string)
		if !validName.MatchString(k) || !ok {
			return nil, fmt.Errorf("%s : %s doit être une chaîne au nom en MAJUSCULES", path, k)
		}
		if keep != nil && keep(k) {
			continue
		}
		if err := os.Setenv(k, s); err != nil {
			return nil, err
		}
		names = append(names, k)
	}
	sort.Strings(names)
	return names, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

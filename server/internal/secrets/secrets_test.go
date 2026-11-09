package secrets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// End-to-end with the real sops and age-keygen when they are installed
// (Mac with brew, CI image); skipped otherwise.
func TestLoadRoundTrip(t *testing.T) {
	sops, err1 := exec.LookPath("sops")
	keygen, err2 := exec.LookPath("age-keygen")
	if err1 != nil || err2 != nil {
		t.Skip("sops ou age-keygen absent")
	}
	dir := t.TempDir()
	key := filepath.Join(dir, "age.key")
	out, err := exec.Command(keygen, "-o", key).CombinedOutput()
	if err != nil {
		t.Fatal(string(out))
	}
	pub := ""
	for _, l := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(l, "Public key: "); ok {
			pub = p
		}
	}
	plain := filepath.Join(dir, "secrets.enc.yaml")
	os.WriteFile(plain, []byte("SN_TEST_TOKEN: abc123\nFORGEJO_TOKEN: fj-x\n"), 0o600)
	enc := exec.Command(sops, "encrypt", "--age", pub, "-i", plain)
	enc.Env = append(os.Environ(), "SOPS_DISABLE_VERSION_CHECK=1")
	if out, err := enc.CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	raw, _ := os.ReadFile(plain)
	if strings.Contains(string(raw), "abc123") {
		t.Fatal("not encrypted")
	}
	t.Setenv("SOPS_AGE_KEY_FILE", key)
	names, err := Load(plain, sops, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "FORGEJO_TOKEN,SN_TEST_TOKEN" || os.Getenv("SN_TEST_TOKEN") != "abc123" {
		t.Errorf("names %v, value %q", names, os.Getenv("SN_TEST_TOKEN"))
	}
	t.Setenv("SN_TEST_TOKEN", "dev")
	if names, _ := Load(plain, sops, func(k string) bool { return k == "SN_TEST_TOKEN" }); os.Getenv("SN_TEST_TOKEN") != "dev" || len(names) != 1 {
		t.Errorf("kept name overwritten: %v %q", names, os.Getenv("SN_TEST_TOKEN"))
	}
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(dir, "missing"))
	if _, err := Load(plain, sops, nil); err == nil {
		t.Error("decrypting without the key must fail")
	}
}

func TestMissingFileIsFine(t *testing.T) {
	names, err := Load(filepath.Join(t.TempDir(), "none.yaml"), "sops", nil)
	if err != nil || names != nil {
		t.Errorf("%v %v", names, err)
	}
}

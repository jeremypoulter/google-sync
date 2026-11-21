package profile

import (
	"testing"

	"github.com/jeremypoulter/google-sync/internal/config"
)

func TestOpenSaveReopen(t *testing.T) {
	base := t.TempDir()
	p, err := Open(base, "jeremy")
	if err != nil {
		t.Fatal(err)
	}
	p.Accounts[config.RoleOld] = &Account{Email: "j@example.com", Domain: "example.com"}
	if err := p.SetCheck(CheckSignInOld, true, "ok"); err != nil {
		t.Fatal(err)
	}
	q, err := Open(base, "jeremy")
	if err != nil {
		t.Fatal(err)
	}
	if q.Accounts[config.RoleOld].Email != "j@example.com" || !q.Checks[CheckSignInOld].Passed {
		t.Fatalf("reopened profile lost data: %+v", q.Data)
	}
}

func TestInvalidName(t *testing.T) {
	for _, n := range []string{"", "../x", "a b", "-x"} {
		if _, err := Open(t.TempDir(), n); err == nil {
			t.Errorf("name %q accepted", n)
		}
	}
}

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

func TestResetChecksForSignIn(t *testing.T) {
	p, err := Open(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	pass := func() {
		for _, c := range CheckOrder {
			p.Checks[c.Name] = &Check{Passed: true}
		}
	}
	p.Accounts[config.RoleNew] = &Account{Email: "a@gmail.com"}

	pass()
	p.ResetChecksForSignIn(config.RoleNew, "A@gmail.com")
	if p.Checks[CheckTokenAge] != nil || p.Checks[CheckGmailImport] == nil {
		t.Fatalf("same account: %+v", p.Checks)
	}

	pass()
	p.ResetChecksForSignIn(config.RoleNew, "b@gmail.com")
	if p.Checks[CheckGmailImport] != nil || p.Checks[CheckDriveCopy] != nil || p.Checks[CheckSignInOld] == nil {
		t.Fatalf("changed account: %+v", p.Checks)
	}
}

func TestInvalidName(t *testing.T) {
	for _, n := range []string{"", "../x", "a b", "-x"} {
		if _, err := Open(t.TempDir(), n); err == nil {
			t.Errorf("name %q accepted", n)
		}
	}
}

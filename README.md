# GMigrate

A small tool to move a family's Google Workspace accounts to ordinary
personal Google accounts, so the Workspace subscription can be shut down.

Each person runs it, signs in to their old Workspace account and their new
personal account, and it copies their data across. It is one way (old to
new), never deletes anything in the new account, and can be re-run any number
of times to pick up only what changed.

> **Status: milestone M0 (spike).** This build is headless and only proves the
> design: sign-in to both accounts, copying one email, copying one Google Doc,
> and refresh tokens surviving 8+ days. See [docs/m0-spike.md](docs/m0-spike.md).
> The wizard GUI and full sync engine arrive in M1.

## What it will cover

| Service | How | Milestone |
|---------|-----|-----------|
| Gmail | API: messages.import with labels, de-duplicated by Message-ID | M1 |
| Drive (My Drive) | Share to new account + server-side copy; native Docs stay native | M1 |
| Calendar, Contacts, Tasks | API both sides, incremental | M2 |
| Photos, Keep, Chat, Maps... | Guided Google Takeout, then upload or archive | M3 |
| Passwords, YouTube, Blogger, "Sign in with Google" sites | Guided cutover checklist | M4 |

## Design in brief

- **Go**, single executable, no installer. Wails v2 GUI (WebView2, built into
  Windows 10/11) from M1; the same binary runs headless for scheduled nightly
  syncs. Linux build supported.
- **Per-person OAuth**, no service account or domain-wide delegation. One
  External OAuth client, published *In production* but unverified, which the
  Workspace admin marks as Trusted once. Its client ID is built into the exe.
- **Tokens** encrypted with Windows DPAPI. From M1, all sync state lives in one
  SQLite file per profile (pure-Go driver, so no C toolchain needed).
- **Idempotent copies**: every copy carries its source ID inside the new
  account (Drive `appProperties.srcId`, the Gmail Message-ID, Calendar
  extended properties, Contacts client data), so re-runs never duplicate and
  the mapping can be rebuilt if local state is lost.

## Getting started

1. Admin: do the one-time [admin setup](docs/admin-setup.md) (Google Cloud
   project, OAuth client, trust it in the Admin console) and build the exe.
2. Each person: run the [M0 checks](docs/m0-spike.md).

```text
gmigrate signin old|new     Sign in to the old Workspace or new personal account
gmigrate spike gmail        Copy one email
gmigrate spike drive        Copy one Google Doc
gmigrate token-check        Refresh both tokens; passes once they are 8+ days old
gmigrate status             Accounts and M0 progress
```

## Development

Requires Go 1.27 or later.

```sh
go test ./...
go run ./cmd/gmigrate --help
go run ./cmd/gmigrate signin old --client-file client_secret.json
```

```text
cmd/gmigrate/        CLI entry point (GUI entry point joins it in M1)
internal/config/     OAuth client, scopes per side
internal/auth/       Loopback sign-in with PKCE, account checks, refreshing clients
internal/tokenstore/ Encrypted token files (DPAPI on Windows)
internal/profile/    Per-person profile directory and M0 check results
internal/gmailcopy/  Copy one Gmail message with labels and de-duplication
internal/drivecopy/  Share + copy one Drive file, with download/upload fallback
scripts/             Build scripts that embed the OAuth client
docs/                Admin setup and M0 guide
```

## Roadmap

- **M0 Spike** (this): prove auth and the two core copies.
- **M1 MVP**: sync engine, SQLite state, resume, incremental cursors, rate
  limiting, storage preflight; Gmail + My Drive; wizard for one person.
- **M2**: whole family (profiles, address map), Calendar, Contacts, Tasks,
  Drive re-sharing, dashboard and nightly scheduled sync.
- **M3**: Takeout assistant, Photos upload, Keep export.
- **M4**: cutover checklist, shared drives, YouTube, code signing, release.

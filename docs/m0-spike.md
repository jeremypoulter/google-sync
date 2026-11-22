# M0 spike: proving the design

M0 answers one question before the GUI and sync engine are built: **does the
per-person OAuth design work?** It is headless only. It passes when these
five checks pass for at least one person:

| # | Check | Command |
|---|-------|---------|
| 1 | One unverified, in-production OAuth client signs in to a Workspace account (after the admin trusts it) | `gmigrate signin old` |
| 2 | The same client signs in to a consumer account | `gmigrate signin new` |
| 3 | One email is imported into the new account, with labels | `gmigrate spike gmail` |
| 4 | One Google Doc is copied server side and stays a native Doc | `gmigrate spike drive` |
| 5 | Both refresh tokens still work 8+ days later | `gmigrate token-check` |

`gmigrate status` shows which checks have passed. Results are stored in the
profile (`profile.json`), so the checks can be done on different days.

## Running it

Do the [admin setup](admin-setup.md) first, then in a terminal (PowerShell):

```powershell
.\gmigrate.exe signin old          # browser opens: pick the OLD Workspace account
.\gmigrate.exe signin new          # pick the NEW personal account
.\gmigrate.exe spike gmail         # copies the most recent email
.\gmigrate.exe spike drive         # copies your most recently edited Google Doc
.\gmigrate.exe status
```

Then, eight or more days later, without signing in again:

```powershell
.\gmigrate.exe token-check
```

Useful options:

- `--profile NAME` keeps each person separate (default `default`).
- `signin old --domain example.com` insists the old account is on that domain.
- `spike gmail` copies the most recent message by default, which is all M0
  needs. `--message ID` copies a specific message, but it needs the Gmail
  *API* message ID, which is not the code at the end of a Gmail web URL and not
  the `Message-ID` header. `--label-prefix "Old/"` puts the copied user
  labels under `Old/`.
- `spike drive --file ID` copies a specific file (the ID is in its URL).
  `--folder ID` puts the copy in a folder of the new account.
- Running a spike twice is safe: the second run finds the first copy (Gmail
  by the email's Message-ID header, Drive by the `srcId` app property) and
  says so instead of copying again. That also exercises the duplicate guard
  the sync engine will rely on. The rare email with no Message-ID header
  cannot be matched this way and would be imported again; M1's local state
  covers those.
- If a Drive copy succeeds but the temporary share cannot be removed, the
  profile remembers it and the next `spike drive` removes it first.

## What to look for

- **Sign-in**: the "Google hasn't verified this app" screen should appear,
  with an *Advanced* link. If the Workspace account instead says the app is
  **blocked by your organisation**, step 4 of the admin setup (Trusted) has not
  taken effect yet; it can take a few minutes.
- **Gmail**: the email appears in the new account with its original date,
  sender and labels. It is not marked as spam and does not re-notify anyone.
- **Drive**: the copy is a native Google Doc owned by the new account, and the
  old file no longer lists the new account in its sharing (the temporary share
  is removed).
- **Token check**: a refresh failing with `invalid_grant` can have ordinary
  causes: a password change on that account, removing GMigrate from its
  Third-party connections page, or the app being set back to Testing. Rule
  those out, sign in again and repeat the 8-day wait. If a fresh pair of
  tokens fails again, investigate whether "In production" publishing really
  avoids the 7-day expiry for this app before building M1 on it; if it does
  not, the fallback is a weekly re-sign-in prompt.

## Where things are stored

`%LOCALAPPDATA%\GMigrate\profiles\<profile>\` on Windows
(`~/.local/share/gmigrate/profiles/<profile>/` on Linux):

- `profile.json`: signed-in addresses and M0 check results.
- `tokens\old.tok`, `tokens\new.tok`: refresh tokens, encrypted with Windows
  DPAPI for the current Windows user (on Linux, plain files readable only by
  you, until keyring support lands).

Deleting the profile folder signs the tool out. To revoke access on Google's
side too, remove GMigrate from each account's
[Third-party connections](https://myaccount.google.com/connections) page.

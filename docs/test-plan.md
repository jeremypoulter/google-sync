# Test plan

This plan covers what CI cannot: running GMigrate against real Google
accounts, on a real Windows PC, before it touches the family's data. The
step-by-step version to tick off is [manual-test-checklist.md](manual-test-checklist.md).

CI already runs the unit tests (fake Google servers, Linux and Windows) plus
lint and builds. Those prove the code does what the author meant. Manual
testing proves that Google's real APIs, the Workspace admin settings and a
real Windows machine behave the way the code assumes.

## Ground rules

1. **Throwaway accounts first, real accounts last.** Nothing runs against a
   family member's real account until every earlier stage has passed.
2. **The Workspace stays untouched until the copy is verified.** No account is
   suspended, deleted or has its licence removed until its person has checked
   their data in the new account and a final sync has run. The tool only
   reads the old account, apart from temporary Drive shares that it removes.
3. **Every finding becomes a test.** A bug found by hand gets a unit test
   (or a fake-server test) before it is fixed, so it cannot come back.
4. **Record evidence.** For each test, note the date, build (`gmigrate
   version` or commit), pass or fail, and anything odd, in the results log at
   the end of the checklist.

## What exists today versus later

Most of the outline depends on features that are not built yet. The plan is
written for the whole tool so the sync engine can be built to pass it, and
each test is tagged with the milestone that makes it runnable.

| Area | Runnable from | Today's command |
|------|---------------|-----------------|
| Sign-in, wrong-account checks, revoked tokens | **M0 (now)** | `signin old/new`, `token-check`, `status` |
| One email with labels, duplicate guard | **M0 (now)** | `spike gmail` |
| One Drive file, duplicate guard, temporary share cleanup | **M0 (now)** | `spike drive` |
| 8-day refresh token survival | **M0 (now)** | `token-check` |
| Whole mailbox and My Drive, incremental sync, resume, rate limits, storage preflight | M1 | (sync engine) |
| Calendar, Contacts, Tasks, Drive re-sharing, several profiles, scheduled sync | M2 | |
| Takeout, Photos, Keep | M3 | |
| Cutover checklist, shared drives, signed release | M4 | |

## Test accounts and seed data

### Accounts

| Name used here | What it is | Why |
|----------------|------------|-----|
| **TEST-OLD** | A new user in the family's real Workspace domain, e.g. `gmigrate-test@<domain>` | Tests the real admin setup (Trusted app, external sharing). Costs one licence for a month or two; delete it at the end. |
| **TEST-NEW** | A fresh personal `@gmail.com` account | The destination. Fresh means anything in it came from the tool. |
| **TEST-NEW-2** | A second fresh personal account (optional) | Cross-account checks: wrong account signed in, sharing to another family member. |
| **OUTSIDER** | Any other account (e.g. Jeremy's real personal one) | Owner of files and events shared *with* TEST-OLD, and sender of mail. |

A Workspace trial domain would also work, but it would need its own admin
setup and would not test the settings the family's accounts actually use.

### Seed data

Seed TEST-OLD by hand before the first run and give each item an ID (put the
ID in the subject, file name or event title) so it can be found and compared
later. Seeding is tedious by hand; a seeding script is listed under automation
gaps.

**Gmail** (G-series)

| ID | Item | What it tests |
|----|------|---------------|
| G01 | Plain text email received from OUTSIDER | Basic import, sender, date |
| G02 | Email with three attachments (PDF, JPEG, DOCX) | Attachments intact |
| G03 | Email with a 20 MB attachment | Large message upload path |
| G04 | Email with a 35+ MB message (send from a client that allows it, or import via another tool) | Size limits near Gmail's maximum |
| G05 | Thread of 5+ replies between TEST-OLD and OUTSIDER | Threading in the new account |
| G06 | Sent email from TEST-OLD | SENT label, appears under Sent |
| G07 | Email labelled with a nested label `Family/School/2025` | Nested labels recreated |
| G08 | Email with three labels at once, starred, marked important | Multiple labels and system labels |
| G09 | Unread email in Inbox; read email archived (no Inbox) | UNREAD and archive state kept |
| G10 | Email in each category tab (Social, Promotions, Updates, Forums) | Category labels |
| G11 | Email in Spam and one in Trash | Decide and record whether these should be copied |
| G12 | Draft | Drafts are not copied (expected), make sure nothing breaks |
| G13 | Email with non-ASCII subject and body (accents, emoji, Chinese) | Encoding |
| G14 | Email with a calendar invitation | Does not create an event or send a reply (`processForCalendar=false`) |
| G15 | Email sent from TEST-OLD **to TEST-NEW** | Message-ID duplicate guard: TEST-NEW already holds this message, see risk R1 |
| G16 | Very old email (or one with a bad/missing Date header) | Date shown in the new account |
| G17 | Label with a colour, and a label hidden from the label list | What label settings carry over |
| G18 | A Gmail filter and a signature | Not copied by design; note for the cutover checklist |

**Drive** (D-series)

| ID | Item | What it tests |
|----|------|---------------|
| D01 | Google Doc with comments, suggestions and a long revision history | Stays native; comments and history are lost on copy (expected, note it) |
| D02 | Google Sheet with formulas, a chart and a cross-sheet reference | Native copy, formulas intact |
| D03 | Google Slides with images | Native copy |
| D04 | Google Form with responses linked to a Sheet | Forms copy behaviour, response link |
| D05 | Google Drawing; Apps Script project; Sheet with a bound script | Less common native types |
| D06 | PDF, JPEG, MP4 (100+ MB), ZIP | Binary copy, checksum match |
| D07 | File with an apostrophe and accents in the name, e.g. `Mum's récipes.docx` | Query escaping |
| D08 | Folder tree three levels deep with files at each level | Folder structure (M1) |
| D09 | File shared by TEST-OLD with OUTSIDER (editor) and with "anyone with the link" | Sharing is not copied today; M2 re-sharing |
| D10 | File owned by OUTSIDER shared with TEST-OLD | Not owned: should not be copied; it should be re-shared to TEST-NEW (M2) or listed |
| D11 | File where the owner turned off download/copy for viewers | Fallback path and its limits |
| D12 | Shortcut to another file | Shortcut handling |
| D13 | File already shared with TEST-NEW before the run | The tool must not remove that existing share afterwards |
| D14 | File in Trash | Should not be copied |
| D15 | File in a shared drive (if the domain has one) | M4 |

**Calendar** (C-series, M2)

| ID | Item | What it tests |
|----|------|---------------|
| C01 | Single event with location, description, reminder | Basic copy |
| C02 | Weekly recurring event with an end date | Recurrence rule |
| C03 | Recurring event with one instance moved and one deleted | Exceptions kept |
| C04 | Event with OUTSIDER as a guest, and one TEST-OLD was invited to | Guests do **not** get new invitations or update emails |
| C05 | All-day multi-day event; event in another time zone | Dates and time zones |
| C06 | Event with an attachment (Drive file) and a Meet link | Links |
| C07 | A second calendar owned by TEST-OLD, and one subscribed from OUTSIDER | Secondary and subscribed calendars |
| C08 | Event with Busy/Free, private visibility and a colour | Event properties |

**Contacts** (P-series, M2)

| ID | Item | What it tests |
|----|------|---------------|
| P01 | Contact with several phones, emails, addresses, birthday, notes | Fields |
| P02 | Contact with a photo | Photo |
| P03 | Contacts in two labels (groups), one in both | Groups |
| P04 | "Other contacts" (auto-saved from email) | Whether these are copied, as decided |
| P05 | Two contacts that look like duplicates | Not merged or doubled |
| P06 | Contact with non-ASCII name | Encoding |

**Tasks** (T-series, M2): two lists, a task with a due date, a subtask, a
completed task.

## Test areas

Each area lists what to do and what "pass" means. Detailed steps are in the
checklist.

### A. Admin setup and build (M0)

- The exe built with `scripts/build.ps1` signs in with no `--client-file`
  (the client is embedded), and `gmigrate version` prints the expected build.
- The OAuth client is External, In production, unverified, and Trusted in
  the Admin console for the organisation.
- Drive sharing outside the domain is on for the test user.
- **Pass:** a family member could be handed the exe with no other setup.

### B. Sign-in and account checks (M0)

- Sign in old and new; the "Google hasn't verified this app" screen appears
  once per account and continues via *Advanced*.
- Workspace sign-in is **not** blocked "by your organisation" (proves Trusted).
- The consent screen lists only the expected scopes (compare with
  `internal/config/config.go`). Note that the old account is asked for full
  Drive access, which is needed to add and remove the temporary shares.
- Wrong account in each slot is rejected with a clear message: personal
  account for `old`, Workspace account for `new`, and `--domain` mismatch.
- Closing the browser without consenting, or clicking *Cancel*, ends the
  command with a clear error rather than hanging.
- Signing in again to a slot replaces the account and resets that slot's
  check results.
- Profiles are separate: `--profile alice` and `--profile bob` do not see each
  other's tokens.
- **Pass:** every path ends with a clear message, and `status` shows the right
  accounts.

### C. Token lifetime and revoked access (M0)

- `token-check` passes when both tokens are 8+ days old (proves In production
  avoids the 7-day Testing expiry). This is the M0 gate for the auth design.
- Revoking access is handled cleanly, for each of: removing GMigrate on
  [Third-party connections](https://myaccount.google.com/connections),
  changing the account password, the admin removing Trusted (old account
  only), and the admin suspending the test user. Each should produce a message
  that names the account and says to sign in again, not a stack trace or a
  silent retry loop. `signin` again then recovers.
- Token files are encrypted: copying `tokens\old.tok` to another Windows user
  or PC and pointing a profile at it fails to decrypt with a clear error.
- A corrupted token file (edit a byte) gives a clear error and recovers after
  `signin`.
- **Pass:** no way of losing access leaves the tool stuck or silent.

### D. Gmail copy (M0 spike, M1 full)

- Spike: copy G02, G07, G08, G13 by `--message` (Gmail API ID) and compare in
  the new account: sender, recipients, subject, date, body, attachments open
  and match (size and, for a few, checksum), labels and nesting, star,
  important, read state, Inbox/archive state.
- `--label-prefix "Old/"` puts user labels under `Old/` and leaves system
  labels alone.
- G14 creates no calendar event and sends no reply. G01 is not marked spam.
- Running the same message twice says "Already in the new account".
- M1 full mailbox: message counts per label match between old and new (allow
  for documented exclusions: drafts, chats, and spam/trash if excluded);
  thread counts roughly match; a sample of 20 random messages is checked by
  hand.
- **Risk R1 (Message-ID duplicate guard):** a message that already exists in
  the new account with the same Message-ID is skipped. That happens when the
  old and new accounts emailed each other (G15), both were on the same mailing
  list, or forwarding from old to new was switched on. The old account's copy
  (its labels, its Sent state) is then never imported. Decide whether that is
  acceptable, or whether a skipped message should at least get the old labels
  added. Test with G15 and with forwarding switched on for a day.
- **Pass:** nothing missing, nothing doubled, nothing sent to anyone.

### E. Drive copy (M0 spike, M1 full)

- Spike: copy D01, D02, D03, D06, D07 by `--file`. Native files stay native
  (open in Docs/Sheets/Slides, not as a download). Binary files' `md5Checksum`
  in the new account matches the source (Drive API or file properties).
- The copy is owned by TEST-NEW and the old file's sharing no longer lists
  TEST-NEW (temporary share removed). D13's pre-existing share is kept.
- Running the same file twice says "Already copied".
- A folder ID is refused with a clear message (spike only copies files).
- D11 (copying disabled): binary falls back to download and upload, or fails
  with a clear message; native files fail clearly. Record which.
- Ctrl+C during a large copy (D06 MP4) leaves no temporary share on the
  source; if it does, the next `spike drive` removes it first.
- M1 full My Drive: file and folder counts match; folder tree matches; D10,
  D14 not copied; storage preflight warns before exceeding TEST-NEW's 15 GB.
- M2: sharing recreated from the new account (D09), files shared with the old
  account are re-shared to the new one or listed (D10), and OUTSIDER gets no
  unexpected notification emails.
- **Pass:** every owned file is present, opens, and matches; the source has no
  leftover shares.

### F. Calendar, Contacts, Tasks (M2)

- Calendar: C01 to C08 compared field by field; recurring events show the
  same instances over the next year (compare a few weeks side by side);
  **guests receive no emails** (check OUTSIDER's inbox); time zones correct
  on a PC set to a different zone.
- Contacts: P01 to P06 compared; group membership correct; no doubles after
  two runs.
- Tasks: lists, due dates, subtasks, completed state.
- **Pass:** a person could switch to the new account without missing an
  appointment or a phone number.

### G. Incremental sync (M1 for Gmail and Drive, M2 for the rest)

Run a full sync, then change the source and run again.

| Change at source | Expected after second run |
|------------------|---------------------------|
| Nothing | Zero items copied, zero duplicates, finishes quickly |
| New email arrives | Copied once |
| Label added to or removed from an existing email | Label change mirrored, decide and document |
| Email deleted at source | Not deleted in new account (tool never deletes); reported |
| New Drive file; file edited | New file copied; edited file updated (the M0 spike skips edited files, M1 must use `srcModified`) |
| Drive file renamed, moved, deleted | Rename and move mirrored; delete not mirrored, reported |
| Copy deleted or trashed in the new account | Decide: re-copy or leave alone; document |
| New event, edited event, deleted event | Mirrored (deletions included, as decided for calendar) |
| Contact added and edited | Mirrored |
| Local state deleted (profile SQLite removed) | Next run rebuilds the mapping from the source IDs in the new account and copies nothing twice |

- Run the second sync on another day too, so tokens and history cursors age.
- **Pass:** after every change, counts match and there are no duplicates.

### H. Interruption, resume and rate limits (M1)

- Kill the process (Task Manager, *End task*) mid-Gmail and mid-Drive; pull
  the network cable or turn off Wi-Fi mid-run; put the laptop to sleep
  mid-run. Restart: the run resumes, no duplicates, no leftover temporary
  shares.
- Rate limits: a bulk run of a few thousand emails and a few hundred files
  should hit Google's per-user limits at least once. The tool slows down and
  retries (HTTP 429 / 403 `rateLimitExceeded` / `userRateLimitExceeded`), shows
  that it is waiting, and completes. To get there with test data, seed TEST-OLD
  in bulk (see automation gap A7) or forward a mailing list archive.
- Daily limits: Drive's ~750 GB/day upload limit and Gmail import limits are
  unlikely to be hit by this family, but a run that does should stop with a
  message saying when to resume, not fail item by item.
- Disk full or profile folder read-only: clear error, no corrupt state.
- **Pass:** any interruption is recoverable by running the tool again.

### I. Clean Windows machine (M0 headless, M1 GUI)

- A fresh Windows 11 VM (Hyper-V "Quick Create" or Windows Sandbox) with no
  Go, no developer tools, signed in as a standard (non-admin) user.
- Download the exe the way a family member will (email link, OneDrive, USB).
  SmartScreen and Defender: note the exact warnings until code signing (M4).
- Runs with no installer, no admin rights, no missing DLLs. Browser sign-in
  works with the default browser set to Edge, then Chrome. Loopback redirect
  works with Windows Firewall on (a firewall prompt for a loopback listener
  would confuse people; note if one appears).
- Profile lands in `%LOCALAPPDATA%\GMigrate`. Works from a path with spaces
  and from the Downloads folder.
- M1: WebView2 GUI opens on Windows 10 and 11; text readable at 150% scaling.
- M2: scheduled nightly sync via Task Scheduler runs while the user is logged
  out or the PC is locked, and its log is readable.
- Linux build: smoke test the same M0 commands once on Ubuntu.
- **Pass:** a non-technical person could run it from a download.

### J. Dry run on one family member, then the rest

1. Choose the family member with the smallest, least critical account. Check
   their sizes in the Admin console against 15 GB free in the new account.
2. Run M1/M2 sync (or the M0 spike if that is all that exists) against their
   real accounts on their own PC, with them present.
3. They spend a week using both accounts: everything they look for in the new
   account should be there. Keep a list of anything missing.
4. Run incremental syncs during that week; check no duplicates.
5. Only then repeat for the next person, largest account last.
6. Workspace accounts stay fully active until every person signs off and a
   final sync has run. Then follow the cutover checklist (M4) before any
   account is suspended. Suspend first, delete weeks later.

## Risks to watch

| ID | Risk | Covered by |
|----|------|------------|
| R1 | Message-ID duplicate guard skips messages that already exist in the new account (mail between the two accounts, shared mailing lists, forwarding) | D, G15 |
| R2 | In production unverified tokens still expire after 7 days | C (8-day check) |
| R3 | Admin settings (Trusted, external sharing) change or propagate slowly | B, E |
| R4 | Temporary shares left on source files | E, H |
| R5 | Calendar or Drive actions email outsiders | E, F |
| R6 | New account fills its 15 GB mid-run | E, H |
| R7 | Old account has full Drive scope and could change source files | E (check source modified times unchanged) |
| R8 | Native file features (comments, history, Forms responses, bound scripts) lost on copy | E, D01, D04, D05; document for users |

## Automation gaps

Things that could move from manual to automated, roughly in priority order.

| # | Gap | Suggested test |
|---|-----|----------------|
| A1 | `cmd/gmigrate/spike.go` has no tests: pending-unshare retry, the "share belongs to another old account" guard, recording a `CleanupError`, default message/file selection | Command-level tests with the existing fake Gmail/Drive servers |
| A2 | `token-check` logic: age threshold, `invalid_grant` wording (`describeRefreshError`), one token failing and one passing | Unit tests with a fake token endpoint |
| A3 | Drive download/upload fallback (`isCopyRefused`) and the native-file-refused error are not exercised; nor is retrying 404s until giving up, or the folder refusal | Extend `drivecopy` fake server tests |
| A4 | Gmail: message with no Message-ID (always re-imported), label reuse when the destination label already exists, nested labels, base64 with and without padding, R1 behaviour | Extend `gmailcopy` tests; fuzz `MessageIDHeader` |
| A5 | Built exe not smoke tested: CI builds it but never runs it, and never checks the client embedding in `scripts/build.ps1` / `build.sh` | CI step: run `gmigrate --help` and `status` on each built binary; build once with a dummy client JSON and check the embedded value |
| A6 | DPAPI round trip and "other user cannot decrypt" | Windows-only test in `tokenstore` (round trip is cheap; the other-user case stays manual) |
| A7 | Seeding test accounts by hand | A `scripts/seed` tool (dev only) that fills TEST-OLD with the G/D/C/P items and bulk mail for rate-limit tests, and a matching `verify` that compares old and new |
| A8 | Comparing old and new by hand | A `gmigrate verify` command (useful to family members too): counts per label/folder/calendar, missing items, Drive checksums |
| A9 | Live API tests | Optional manual-dispatch CI job that runs the spikes against TEST-OLD/TEST-NEW using their refresh tokens stored as GitHub secrets. Throwaway accounts only, never family ones |
| A10 | M1 sync engine | Design the fake servers as stateful in-memory Gmail/Drive (history IDs, change tokens, injected 429s and 5xx, crash points between "copied" and "recorded") so G and H above are mostly automated; manual runs then confirm the real APIs match the fakes |

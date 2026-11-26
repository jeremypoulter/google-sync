# Manual test checklist

Tick these off in order. Each stage has a **gate**: do not start the next
stage until the gate passes. The reasoning and the seed items (G01, D06 and
so on) are in [test-plan.md](test-plan.md). Stages marked *M1* or *M2* wait
for that milestone.

Commands are PowerShell in the folder holding `gmigrate.exe`. Use
`--profile test` on every command in stages 1 to 7 so test tokens never mix
with real ones.

---

## Stage 0: Prepare (about 2 hours, once)

- [ ] Admin setup done as in [admin-setup.md](admin-setup.md): Cloud project,
      APIs enabled, consent screen External and **In production**, Desktop
      client, client ID **Trusted** in Admin console, Drive external sharing on.
- [ ] Create **TEST-OLD**: a new user in the Workspace domain (Admin console >
      Directory > Users > Add new user), e.g. `gmigrate-test@<domain>`.
- [ ] Create **TEST-NEW**: a fresh personal Google account (and optionally
      **TEST-NEW-2**).
- [ ] Pick an **OUTSIDER** account to send mail and share files from.
- [ ] Seed TEST-OLD Gmail with G01 to G17 (put the ID in each subject).
- [ ] Seed TEST-OLD Drive with D01 to D14 (put the ID in each file name).
- [ ] *(For M2)* Seed Calendar C01 to C08, Contacts P01 to P06, Tasks.
- [ ] Note the starting counts in TEST-OLD: messages per label (Gmail web,
      label list), files in My Drive, events, contacts. Write them in the
      results log.
- [ ] Note, for D01, D02, D06: the last-modified time and sharing list in the
      old account (to prove later that the tool did not change them).
- [ ] Build: `.\scripts\build.ps1 -ClientFile <path>`; run
      `.\dist\gmigrate.exe version` and record it.

**Gate 0:** accounts exist, seed data in place, starting counts recorded.

---

## Stage 1: Sign-in (M0, 20 minutes)

- [ ] `gmigrate signin old --profile test --domain <domain>`; pick TEST-OLD.
  - [ ] "Google hasn't verified this app" appears; *Advanced > Go to GMigrate* works.
  - [ ] It does **not** say "blocked by your organisation".
  - [ ] Consent lists Gmail read, Drive, Calendar read, Contacts read, Tasks read.
  - [ ] Command reports TEST-OLD's address.
- [ ] `gmigrate signin new --profile test`; pick TEST-NEW. Consent lists
      Gmail insert, labels and read, Drive, Calendar, Contacts, Tasks.
- [ ] `gmigrate status --profile test` shows both addresses and checks 1 and 2 passed.
- [ ] Wrong account: `signin old --profile wrong`, pick TEST-NEW. Clear
      "not a Workspace account" error.
- [ ] Wrong account: `signin new --profile wrong`, pick TEST-OLD. Clear
      "is a Workspace account" error.
- [ ] Wrong domain: `signin old --profile wrong --domain example.com`, pick
      TEST-OLD. Clear domain error.
- [ ] Cancel: start `signin new --profile wrong`, click *Cancel* on Google's
      screen. Clear error, command ends.
- [ ] Abandon: start `signin new --profile wrong`, close the browser tab.
      Note what happens (does it wait forever? can Ctrl+C stop it cleanly?).
- [ ] Delete `%LOCALAPPDATA%\GMigrate\profiles\wrong`.

**Gate 1:** both sign-ins work and every wrong path gives a clear message.

---

## Stage 2: Spike copies (M0, 45 minutes)

`--message` needs the Gmail **API** message ID, which is not in the Gmail web
URL or in *Show original*. Get it from the
[API explorer](https://developers.google.com/gmail/api/reference/rest/v1/users.messages/list)
(signed in as TEST-OLD, `userId=me`, `q=subject:G02`), or send yourself the
seed email again so it is the most recent and run without `--message`.

**Gmail**

- [ ] `gmigrate spike gmail --profile test --message <G02 id>`.
  - [ ] Shows "Imported", Message-ID, labels.
  - [ ] In TEST-NEW: sender, date (original, not today), subject, body OK.
  - [ ] All three attachments open and match the originals.
  - [ ] Not in Spam; nobody received a notification.
- [ ] Run the same command again: says "Already in the new account".
- [ ] G07 with `--label-prefix "Old/"`: label appears as `Old/Family/School/2025`, nested.
- [ ] G08: all labels, star and important carried over.
- [ ] G09: unread stays unread, archived stays out of the Inbox.
- [ ] G13: accents, emoji and Chinese characters display correctly.
- [ ] G14: no event created in TEST-NEW's calendar, no reply sent to OUTSIDER.
- [ ] G03 (20 MB attachment) imports; record how long it took.
- [ ] G15 (sent from TEST-OLD to TEST-NEW): record what happens. Expected
      today: "Already in the new account" and the old Sent copy is **not**
      imported (risk R1). Decide whether that is acceptable.

**Drive**

- [ ] `gmigrate spike drive --profile test --file <D01 id>` (ID from the URL).
  - [ ] Shows "Copied ... server side".
  - [ ] In TEST-NEW: a native Google Doc, owned by TEST-NEW. Comments and
        version history are gone (expected; note it).
  - [ ] In TEST-OLD: D01's sharing no longer lists TEST-NEW; last-modified unchanged.
- [ ] Run again: says "Already copied".
- [ ] D02 (Sheet): formulas and chart work.
- [ ] D03 (Slides): images present.
- [ ] D06 PDF and JPEG: copy matches (compare size, or `md5Checksum` via
      [API explorer](https://developers.google.com/drive/api/reference/rest/v3/files/get)
      with `fields=md5Checksum`).
- [ ] D07 (apostrophe and accents in the name): copies, name intact, second
      run detects it.
- [ ] D11 (copy disabled): record whether it falls back to download and
      upload or fails, and that the message is clear.
- [ ] D13 (already shared with TEST-NEW): after the copy, TEST-OLD's sharing
      **still** lists TEST-NEW.
- [ ] A folder ID: clear "is a folder" message.
- [ ] D06 MP4: press Ctrl+C mid-copy. Check D06's sharing in TEST-OLD. If
      TEST-NEW is still listed, run `spike drive` again and confirm it says
      "Removed a leftover temporary share".
- [ ] `gmigrate status --profile test` shows checks 3 and 4 passed.

**Gate 2:** every copy matches, duplicates are detected, no shares left on
source files, nothing changed in TEST-OLD.

---

## Stage 3: Tokens and revoked access (M0, 8+ days elapsed)

- [ ] Day 0: note the date of the Stage 1 sign-ins.
- [ ] Day 1: `gmigrate token-check --profile test` says both work and when to run again.
- [ ] Day 8 or later, without signing in again: `token-check` says
      **Check passed**. If it fails with `invalid_grant`, follow
      [m0-spike.md](m0-spike.md#what-to-look-for) before going further. This
      is the most important M0 result.

Revocation, using a scratch profile each time (`--profile revoke`, signed in
fresh to both accounts), after the 8-day check has passed:

- [ ] Remove GMigrate from TEST-NEW's
      [Third-party connections](https://myaccount.google.com/connections).
      `token-check --profile revoke`: names the new account, says the token was
      rejected, suggests signing in again. `signin new` recovers.
- [ ] Change TEST-OLD's password. Same expectations for the old account.
- [ ] Admin console: set GMigrate from Trusted to Limited (or remove it).
      `token-check` fails clearly for the old account; a new `signin old` is
      blocked by the organisation. Set it back to Trusted; sign-in works again
      (may take a few minutes).
- [ ] Suspend TEST-OLD for a moment. Clear error. Unsuspend.
- [ ] Copy the `revoke` profile folder to another Windows user (or PC) and run
      `status --profile revoke` there: tokens fail to decrypt with a clear error.
- [ ] Edit one byte of `tokens\new.tok`: clear error; `signin new` recovers.
- [ ] Delete the `revoke` profile and remove GMigrate from both accounts'
      Third-party connections.

**Gate 3 (M0 done):** 8-day check passed and every revocation gives a clear,
recoverable message. Record the result in project notes.

---

## Stage 4: Clean Windows machine (M0 now, repeat for M1 GUI)

- [ ] Fresh Windows 11 VM or Windows Sandbox, standard (non-admin) user, no
      developer tools.
- [ ] Download `gmigrate.exe` from an email link or OneDrive into Downloads.
      Record every SmartScreen / Defender warning word for word.
- [ ] Run `.\gmigrate.exe version` and `status`: no missing DLL or runtime errors.
- [ ] Repeat Stage 1 (sign-in) and two spike copies with a new profile.
      - [ ] Works with Edge as default browser.
      - [ ] Works with Chrome as default browser.
      - [ ] No Windows Firewall prompt (record it if one appears).
- [ ] Move the exe to a folder with spaces (`C:\Users\<you>\My Tools\`); still works.
- [ ] Profile is under `%LOCALAPPDATA%\GMigrate\profiles\`.
- [ ] *(M1)* GUI opens on Windows 11 and Windows 10; readable at 150% scaling.
- [ ] *(M2)* Scheduled nightly sync runs with the screen locked; log readable.
- [ ] Linux: `gmigrate-linux-amd64 version`, sign-in and one spike on Ubuntu.

**Gate 4:** runs from a download on a clean PC with no extra steps beyond
the documented warnings.

---

## Stage 5: Full copy (M1 for Gmail and Drive, M2 for the rest)

- [ ] Full sync of TEST-OLD into a **fresh** TEST-NEW (or delete everything
      from TEST-NEW first).
- [ ] Gmail: messages per label match the Stage 0 counts, apart from
      documented exclusions (drafts, chats, spam/trash if excluded).
- [ ] Gmail: open 20 random messages in both accounts and compare.
- [ ] Drive: file and folder counts match; folder tree (D08) matches;
      D10 and D14 not copied; no temporary shares left on any source file.
- [ ] Drive: storage used in TEST-NEW is about what TEST-OLD's owned files use.
- [ ] Storage preflight: seed TEST-OLD over TEST-NEW's free space (or fill
      TEST-NEW first). The tool warns before starting, not halfway.
- [ ] *(M2)* Calendar C01 to C08 compared field by field; next 12 weeks of
      C02/C03 match instance by instance; OUTSIDER received **no** emails.
- [ ] *(M2)* Contacts P01 to P06 compared; groups correct.
- [ ] *(M2)* Tasks lists, due dates, subtasks, completed state.
- [ ] *(M2)* D09 sharing recreated from TEST-NEW; OUTSIDER got only the
      expected notification (or none, as designed).
- [ ] TEST-OLD unchanged: D01/D02/D06 modified times and sharing as recorded
      in Stage 0.

**Gate 5:** counts match, samples match, outsiders were not emailed, source unchanged.

---

## Stage 6: Incremental sync (M1/M2)

- [ ] Run sync again with no changes: **zero** items copied, finishes quickly.
- [ ] Make these changes in TEST-OLD, then sync:
  - [ ] New email from OUTSIDER: copied once.
  - [ ] Label added to an old email; label removed from another: as designed.
  - [ ] Email deleted: still present in TEST-NEW; reported.
  - [ ] New Drive file: copied once.
  - [ ] D02 edited: TEST-NEW copy updated.
  - [ ] A file renamed and another moved to a different folder: mirrored.
  - [ ] A file deleted: still present in TEST-NEW; reported.
  - [ ] *(M2)* New event, edited event, one instance of C02 cancelled: mirrored.
  - [ ] *(M2)* New contact, edited contact: mirrored.
- [ ] Delete a copied file in TEST-NEW, sync: behaves as designed (re-copy or skip).
- [ ] Delete the profile's local state (SQLite file), sync: **nothing**
      copied twice (mapping rebuilt from source IDs in TEST-NEW).
- [ ] Repeat a no-change sync a few days later: still zero copied.
- [ ] Counts still match Stage 0 counts plus the additions.

**Gate 6:** no duplicates anywhere after every change.

---

## Stage 7: Interruption, resume, rate limits (M1)

- [ ] Kill the process in Task Manager during Gmail copy; restart; resumes,
      no duplicates.
- [ ] Same during Drive copy; also check no temporary shares are left after
      the restarted run.
- [ ] Turn off Wi-Fi mid-run for 2 minutes, then back on: retries and
      continues (or stops clearly and resumes on restart).
- [ ] Sleep the laptop mid-run, wake it: same.
- [ ] Bulk seed TEST-OLD (thousands of emails, hundreds of files) and run:
      the tool shows it is being rate limited and waiting, and completes.
- [ ] Make the profile folder read-only, run: clear error, nothing corrupted;
      restore permissions and run: continues.

**Gate 7:** every interruption recovers by running the tool again.

---

## Stage 8: First real account (dry run)

- [ ] Pick the family member with the smallest, least critical account.
- [ ] Admin console: check their Gmail and Drive sizes fit in 15 GB (or that
      they have bought more storage).
- [ ] Their new personal account exists and they can sign in to it.
- [ ] On **their** PC, with them present, run the tool with their own profile
      name (not `test`).
- [ ] Check together: their 10 most important emails, folders, documents,
      events and contacts are in the new account.
- [ ] Gmail and Drive counts match (as in Stage 5).
- [ ] Workspace account left exactly as it is: no forwarding change, no
      suspension, no licence change.
- [ ] They use both accounts for a week and keep a list of anything missing.
- [ ] Incremental syncs during the week: no duplicates.
- [ ] Every item on their list is fixed or explained.

**Gate 8:** the first person signs off.

---

## Stage 9: Remaining accounts and cutover

- [ ] Repeat Stage 8 for each person, largest account last.
- [ ] Final sync for everyone on the same day.
- [ ] *(M4)* Cutover checklist done for each person (passwords, "Sign in with
      Google" sites, filters, signatures, YouTube, phone accounts).
- [ ] Suspend (not delete) the Workspace accounts. Wait at least 2 to 4 weeks.
- [ ] Delete TEST-OLD and remove GMigrate from Trusted apps when finished.
- [ ] Only then delete the Workspace accounts and cancel the subscription.

---

## Results log

Copy a row per test run.

| Date | Build | Stage / item | Pass / Fail | Notes |
|------|-------|--------------|-------------|-------|
| | | | | |

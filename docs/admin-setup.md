# One-time admin setup

Done once by the Workspace admin, before building the exe. About 20 minutes.
Family members never see any of this: they get an exe with the client built in.

Google renames console pages often. If a label below has moved, search the
console for the quoted name.

## 1. Google Cloud project and APIs

1. Create a project at <https://console.cloud.google.com/projectcreate>
   (any name, e.g. `gmigrate`). It can be owned by either the Workspace or a
   personal account.
2. Enable the APIs the tool uses (Gmail, Drive, Calendar, People, Tasks):
   <https://console.cloud.google.com/flows/enableapi?apiid=gmail.googleapis.com,drive.googleapis.com,calendar-json.googleapis.com,people.googleapis.com,tasks.googleapis.com>

## 2. OAuth consent screen ("Google Auth Platform")

1. Open **Google Auth Platform** in the Cloud console and click **Get started**.
2. **Branding**: app name `GMigrate`, your support email.
3. **Audience**: user type **External**. Then click **Publish app** so the
   status is **In production**. Do *not* submit for verification.
   - In production, refresh tokens do not expire after 7 days as they do in
     Testing mode. That is what M0's 8-day check proves.
   - The app stays "unverified": people see a "Google hasn't verified this
     app" screen once per account and click *Advanced > Go to GMigrate*.
     Unverified apps are limited to 100 users, which does not matter here.
4. **Data Access**: adding scopes here is optional for an unverified app; the
   tool requests them at sign-in. The list is in `internal/config/config.go`.

## 3. Desktop OAuth client

1. **Clients > Create client**, application type **Desktop app**, name
   `GMigrate desktop`.
2. **Download JSON**. Keep this file private (it is ignored by git as
   `client_secret*.json`).

Google does not treat desktop client secrets as confidential (the sign-in uses
PKCE and a loopback redirect), so building it into a family-only exe is fine.
Do not publish a build that contains it.

## 4. Trust the app in the Workspace Admin console

Without this, Workspace users are blocked from granting Gmail and Drive access
to an unverified app.

1. Admin console > **Security > Access and data control > API controls**
   (<https://admin.google.com/ac/owl>).
2. **Manage Third-Party App Access > Add app > OAuth App Name Or Client ID**.
3. Paste the client ID from the JSON, select it, choose the whole
   organisation, and set access to **Trusted**.

## 5. Allow Drive sharing to the new accounts

Drive files are copied by sharing each file to the person's new account and
copying it from there, so files must be shareable outside the domain.

Admin console > **Apps > Google Workspace > Drive and Docs > Sharing
settings > Sharing options**: set sharing outside the organisation to **On**
(for at least the family's users). The *Allowlisted domains* option does not
work here, because it only allows Workspace domains, not personal Gmail
accounts. Turn off "warn when sharing outside" if you prefer fewer prompts.

## 6. Takeout (needed from M3)

Admin console > **Data > Data import & export > Google Takeout**: make sure
Takeout is on for users. Not needed for M0.

## 7. Build the exe

With Go 1.26+ installed (<https://go.dev/dl/>):

```powershell
.\scripts\build.ps1 -ClientFile C:\path\to\client_secret_XXXX.json
```

or on Linux/macOS:

```sh
scripts/build.sh path/to/client_secret_XXXX.json
```

The result is `dist/gmigrate.exe`, a single file with no installer.
For development you can skip embedding and run with
`--client-file path/to/client_secret.json` instead.

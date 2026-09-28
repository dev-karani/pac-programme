# PAC Postgraduate Progress Platform

A local implementation of the PAC University postgraduate progress platform, using Go, PostgreSQL, Goose migrations and React + TypeScript. The supplied PAC logo is used on the sign-in page, workspaces and favicon. All seeded people, schools and programme policies are explicitly fictional demonstration data.

## Start locally

Requirements: Docker with Compose.

```bash
cp .env.example .env
docker compose up --build -d
docker compose exec api /app/pac seed
```

Open [http://localhost:8080](http://localhost:8080).

For this workstation, a clean native preview is available at [http://127.0.0.1:8094](http://127.0.0.1:8094). Restart it from the project directory with `bash scripts/local-preview.sh`. This requires PostgreSQL command-line tools, Go, npm, installed web dependencies and Goose. On a new installation, pass `--seed` explicitly to add fictional accounts. The native preview retains its database and uploads under ignored `server/data/`; its PostgreSQL port is 55433 and is loopback-only. It uses local trust authentication and is strictly a development setup.

Stop the application without deleting data:

```bash
docker compose down
```

PostgreSQL and uploaded files use named persistent volumes. To deliberately remove local demo data, use `docker compose down -v`.

## Demo walkthrough

All roles use the same sign-in address. Their assigned role determines the workspace; staff do not register through the student form.

### University-issued login (ERP deferred)

Public student registration is disabled, including the signup API. Students sign in using accounts provisioned by university IT; no ERP or SSO connection is claimed. Missing records are handled by the programme office, not a second student onboarding form. Staff maintenance tools retain verified historical baselines and programme configuration.

For a fictional provisioned newcomer, use `newstudent@demo.pac.test` after applying the optional pitch examples below. Brian has an active part-time plan and a prominent **Choose supervisor** action. Continuing student Amina retains her existing progress. All demo passwords are `Demo123!Change`.

### Optional pitch examples

After the ordinary seed and current migrations, run `go run ./cmd/pac seed-pitch` with the same DATABASE_URL and UPLOAD_DIR. This explicitly adds fictional tasks, approved/declined concept history, resources, sample submission/discussion records and notifications, and activates the provisioned newcomer example. It is idempotent and does not reset existing submissions or progress. Never run demo seed commands against real university data.

### In-app communication

The notification bell shows unread counts. Notifications and milestone discussions refresh every ten seconds; users can mark notifications read, dismiss them and open linked work. Comments notify the student, current supervisors and authorized staff already participating in the thread. Leave, extension and supervision changes generate in-app alerts; new support replies notify case participants, without putting private narratives in notification titles.

**Follow-ups** provides direct messaging between authorized participants in a student's academic record. Click a next-action owner in a staff report to compose a follow-up, or use the referral section beneath a milestone discussion. Only sender and recipient can read each direct message. Participants can reply from Follow-ups; no email is sent. Arbitrary external recipients and IT/finance email routing are intentionally deferred.

Staff milestones are grouped alphabetically by student with expandable stage lists and completion rings. Staff metrics use restrained urgency colours. Concept cards retain outcome, reviewer and decision time. The supplied horizontal university logo is used at sign-in and in the workspace.

### Research and support

The student **Overview** includes a connected-circle milestone tracker, progress percentage and deadline flags (red for overdue/due today; yellow within the configured warning window). Dates use Nairobi time. Work awaiting supervisor review is not labelled as a late student submission, and student deadline alerts pause on approved leave.

**My journey** groups milestones in a left-hand sidebar with actual programme week ranges. Each stage has Tasks, Resources, Meetings and Submissions sections. On mobile, a milestone selector replaces the sidebar. **Open milestone** retains the full record, comments and earlier academic decisions. Badges include readable status text and size to their contents.

Sign in as the fictional student:

- Email: `student@demo.pac.test`
- Password: `Demo123!Change`

Then:

1. Review your next actions and programme-specific journey.
2. Open **My journey**, select **Research proposal**, and inspect tasks and the revised approved date.
3. Submit `demo-fixtures/revised-proposal-demo.pdf`. The server verifies the type, stores the file under an opaque key, creates an immutable version and receipt, completes the student submission action, and creates a supervisor review assignment in one serializable transaction.
4. Open **Requests & support** to inspect an active case or create a new one. Selecting `personal/welfare` automatically applies restricted visibility.
5. Sign out and use `coordinator@demo.pac.test`, `hod@demo.pac.test`, or `support@demo.pac.test` with the same password to see a scoped staff summary.

Students propose their own **Research concepts**, including problem, objectives and methodology. The assigned supervisor approves, declines or requests revisions with feedback. Earlier versions remain visible. The old topic-application interface is not used.

For document review, attach the PDF similarity report to the exact submitted version. Supervisors can download both files, verify the evidence and record a decision. Where both-supervisor approval is configured, both decisions are required on the same version. A revision creates a new version and requires fresh evidence.

The supervisor dashboard focuses on assigned students and academic reviews; the dean dashboard groups school progress by department; graduate-school leadership sees institution-wide progress by school. Coordinator/HOD permissions remain scoped. Support officers see assigned cases; examiners see their examination assignments; administrators manage accounts, imports, organization and settings, without automatic academic or restricted-welfare access.

Other demo accounts use `Demo123!Change`:

| Role | Email |
|---|---|
| Supervisor | `supervisor@demo.pac.test` |
| Coordinator | `coordinator@demo.pac.test` |
| HOD | `hod@demo.pac.test` |
| Dean | `dean@demo.pac.test` |
| Graduate-school leadership | `leadership@demo.pac.test` |
| Examiner | `examiner@demo.pac.test` |
| Support officer | `support@demo.pac.test` |
| System administrator | `admin@demo.pac.test` |
| Provisioned new student (after pitch seed) | `newstudent@demo.pac.test` |
| Approved-leave student | `leave@demo.pac.test` |
| Defence-stage student | `defence@demo.pac.test` |

These credentials exist only after the explicit seed command and must not be used in production.

## Development

Run the database and migrations with Compose, then start the two development processes:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d db migrate

cd server
DATABASE_URL='postgres://pac:pac_dev_only@localhost:5432/pac_progress?sslmode=disable' \
UPLOAD_DIR='./data/uploads' APP_ADDR=':8080' go run ./cmd/pac

cd ../web
npm ci
npm run dev
```

The development override exposes PostgreSQL only on `127.0.0.1:5432`. The Vite development server proxies `/api` to port 8080. Use the explicit seed command on your local development database if demo data is needed.

## Verification

```bash
cd server && TEST_DATABASE_URL="$DATABASE_URL" go test ./...
cd ../web && npm run build
PLAYWRIGHT_BROWSERS_PATH="$HOME/.cache/ms-playwright" npm run test:e2e
docker compose config -q
```

Install Chromium first with `npx playwright install chromium` in `web`. Set `PAC_BASE_URL` when the running API is not on port 8080. Tests require a migrated, seeded **disposable test database** and mutate its records. Use a fresh seed for each complete browser suite. Do not point these tests at real student data. Backend integration tests are skipped unless `TEST_DATABASE_URL` is supplied.

Coverage includes registration privilege boundaries, verification and continuing-student baselines, distinct supervisor allocation, concurrent reservation capacity, immutable revisions and both-supervisor approval, meetings/actions, leave/extension schedule history, restricted-case access, scoped report variants, reminders, and browser flows for all nine role workspaces, provisioned login, requests, concept feedback, document review, notification delivery/read state, direct follow-ups and mobile navigation.

## Security and policy notes

- Sessions use random server-side tokens; only SHA-256 token hashes are stored. Cookies are HTTP-only and SameSite=Lax, and are Secure under TLS.
- Passwords use bcrypt-compatible hashes. Credential reset should revoke all existing sessions.
- File names are never used as storage paths. PDF signatures and Office ZIP contents are checked in addition to extension and size.
- Staff dashboard counts are derived through assigned organization scope. Student detail endpoints derive access from the authenticated student relationship, not client-supplied department IDs.
- Welfare-case narratives are intentionally absent from staff summary queries.
- Deadlines and milestone templates in the seed are **demonstration policy**, not official PAC policy.
- PostgreSQL is authoritative; the interface does not use browser storage for product records.

## Implemented release

- Accounts, password changes/resets, session revocation, deactivation, role assignments and scoped organization access.
- New-student onboarding verification, existing-student verified baselines, versioned programme templates, generated plans, dependencies and schedule revision history.
- Supervisor directory, programme eligibility, distinct primary/secondary requests, reservations, capacity-safe allocation, exceptions and replacement history.
- Student-proposed research concepts, immutable revisions and assigned-supervisor academic decisions.
- Responsive student journey, required/optional activities, resources, versioned submissions, receipts, manual similarity evidence, reviews and discussions.
- Meeting proposals, participants, confirmations, attendance, notes and named follow-up actions.
- Durable reminder jobs, grouped alerts, overdue escalation, notifications and action lists.
- Restricted welfare cases, replies, follow-ups, resolution requirements, leave, extensions and preserved schedule changes.
- Committees, conflict declarations/resolution, availability, overlap suggestions, defence scheduling, reports, outcomes and correction verification.
- Role-aware student, supervisor, coordinator, HOD, dean, leadership, examiner, support and administrator workspaces.
- Permission-scoped reports, formula-safe CSV export, audit history, file downloads and real scoped dashboard counts.
- Responsive narrow-screen navigation, keyboard focus, loading/empty/error states, text-labelled status and no colour-only meaning.

The same-origin interface uses the versioned API for persisted state transitions. Public registration is disabled; university-issued account provisioning, temporary-password resets and scoped roles are administered separately. Leave coordinators review requests; HOD/dean/leadership record approval decisions.

Before a public rollout, replace demonstration organization/templates with approved school policy, configure HTTPS and operational rate limits, arrange database/file backups and recovery, and perform institutional security and user-acceptance review. Passing the automated suite is not a production-readiness certification.

Excluded integrations from the specification remain excluded: Microsoft/Teams, registrar/finance, Turnitin API, calendars, email/SMS/WhatsApp, browser push, billing, and AI academic decisions.

## Deferred integrations

ERP/SSO, outbound email and external department delivery remain deferred by request. The local login represents a university-issued account but is not a live ERP login. No production email credentials or external messaging integrations have been added.

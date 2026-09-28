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

### New and continuing student registration

Choose **Create student account** on the sign-in page. Enter admission number, email, programme/cohort, study mode, admission date and a password of 12–72 characters. Select **New student** or **Continuing student**. Registration creates only a self-scoped student account with a submitted enrolment; it does not activate a plan or grant staff access. Sign in to track verification.

The programme office opens **Administration → Open enrolment** to verify the admission and plan start date, or return the record for correction. Continuing students use the same verification process, followed by **Existing-student baseline**: authorized staff record the evidence and historical completion date (or explicitly mark the date unknown). This preserves previous work and unlocks the next eligible milestone.

Email delivery and automatic admission-system verification are not connected. Staff must verify the student's identity/admission using the university's existing process before approval. Existing accounts should sign in or contact administration, not register again.

### Research and support

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
| Pending-verification student | `newstudent@demo.pac.test` |
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

Coverage includes registration privilege boundaries, verification and continuing-student baselines, distinct supervisor allocation, concurrent reservation capacity, immutable revisions and both-supervisor approval, meetings/actions, leave/extension schedule history, restricted-case access, scoped report variants, reminders, and browser flows for all nine role workspaces, signup, requests, concept feedback, document review and mobile navigation.

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

The same-origin interface uses the versioned API for persisted state transitions. Registration is student-only; staff account provisioning, temporary-password resets and scoped roles are administered separately. Leave coordinators review requests; HOD/dean/leadership record approval decisions.

Before a public rollout, replace demonstration organization/templates with approved school policy, configure HTTPS and operational rate limits, arrange database/file backups and recovery, and perform institutional security and user-acceptance review. Passing the automated suite is not a production-readiness certification.

Excluded integrations from the specification remain excluded: Microsoft/Teams, registrar/finance, Turnitin API, calendars, email/SMS/WhatsApp, browser push, billing, and AI academic decisions.

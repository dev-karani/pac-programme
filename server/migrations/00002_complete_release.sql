-- +goose Up
ALTER TABLE enrolments ADD COLUMN IF NOT EXISTS verification_status text NOT NULL DEFAULT 'verified' CHECK (verification_status IN ('draft','submitted','verified','returned'));
ALTER TABLE enrolments ADD COLUMN IF NOT EXISTS verification_note text;
ALTER TABLE enrolments ADD COLUMN IF NOT EXISTS verified_by uuid REFERENCES users(id);
ALTER TABLE enrolments ADD COLUMN IF NOT EXISTS verified_at timestamptz;
ALTER TABLE milestone_definitions ADD COLUMN IF NOT EXISTS similarity_required boolean NOT NULL DEFAULT false;

CREATE TABLE supervisor_profiles (
  user_id uuid PRIMARY KEY REFERENCES users(id),
  home_department_id uuid NOT NULL REFERENCES departments(id),
  description text NOT NULL DEFAULT '', expertise_tags text[] NOT NULL DEFAULT '{}',
  accepting_students boolean NOT NULL DEFAULT true,
  capacity integer NOT NULL DEFAULT 5 CHECK (capacity >= 0)
);
CREATE TABLE supervisor_programme_eligibility (
  supervisor_id uuid NOT NULL REFERENCES supervisor_profiles(user_id),
  programme_id uuid NOT NULL REFERENCES programmes(id),
  active boolean NOT NULL DEFAULT true,
  PRIMARY KEY(supervisor_id,programme_id)
);
CREATE TABLE supervision_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  supervisor_id uuid NOT NULL REFERENCES supervisor_profiles(user_id),
  position text NOT NULL CHECK(position IN ('primary','secondary')),
  state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','accepted','declined','withdrawn','expired','confirmed')),
  student_note text, response_reason text, requested_at timestamptz NOT NULL DEFAULT now(),
  responded_at timestamptz, reservation_expires_at timestamptz,
  UNIQUE(enrolment_id,supervisor_id,position,state)
);
CREATE TABLE supervision_assignments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  supervisor_id uuid NOT NULL REFERENCES supervisor_profiles(user_id),
  position text NOT NULL CHECK(position IN ('primary','secondary')),
  effective_from timestamptz NOT NULL, effective_to timestamptz,
  request_id uuid REFERENCES supervision_requests(id), replacement_reason text,
  created_by uuid NOT NULL REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT now(),
  CHECK(effective_to IS NULL OR effective_to > effective_from)
);
CREATE UNIQUE INDEX supervision_one_active_position ON supervision_assignments(enrolment_id,position) WHERE effective_to IS NULL;
CREATE UNIQUE INDEX supervision_one_person_once ON supervision_assignments(enrolment_id,supervisor_id) WHERE effective_to IS NULL;
CREATE TABLE allocation_exceptions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  supervisor_id uuid NOT NULL REFERENCES users(id), kind text NOT NULL CHECK(kind IN ('eligibility','capacity')),
  reason text NOT NULL, state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','approved','declined')),
  requested_by uuid NOT NULL REFERENCES users(id), decided_by uuid REFERENCES users(id), decided_at timestamptz
);

CREATE TABLE topics (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), supervisor_id uuid NOT NULL REFERENCES users(id),
  title text NOT NULL, description text NOT NULL, tags text[] NOT NULL DEFAULT '{}', capacity integer NOT NULL CHECK(capacity>0),
  closing_date date NOT NULL, state text NOT NULL DEFAULT 'draft' CHECK(state IN ('draft','published','returned','closed')),
  created_at timestamptz NOT NULL DEFAULT now(), version integer NOT NULL DEFAULT 1
);
CREATE TABLE topic_programmes (topic_id uuid NOT NULL REFERENCES topics(id), programme_id uuid NOT NULL REFERENCES programmes(id), PRIMARY KEY(topic_id,programme_id));
CREATE TABLE topic_applications (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), topic_id uuid REFERENCES topics(id), enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  proposed_title text, statement text NOT NULL,
  state text NOT NULL DEFAULT 'submitted' CHECK(state IN ('submitted','under_review','offered','accepted','declined','withdrawn')),
  submitted_at timestamptz NOT NULL DEFAULT now(), decided_at timestamptz,
  CHECK(topic_id IS NOT NULL OR proposed_title IS NOT NULL)
);
CREATE UNIQUE INDEX topic_one_accepted ON topic_applications(enrolment_id) WHERE state='accepted';

CREATE TABLE activity_definitions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), milestone_id uuid NOT NULL REFERENCES milestone_definitions(id),
  title text NOT NULL, activity_type text NOT NULL CHECK(activity_type IN ('checkpoint','submission','verified')),
  owner_role text NOT NULL, due_offset_days integer NOT NULL DEFAULT 0, instructions text NOT NULL DEFAULT '',
  required boolean NOT NULL DEFAULT true, approval_rule text NOT NULL DEFAULT 'none' CHECK(approval_rule IN ('none','primary','either','both')),
  evidence_required boolean NOT NULL DEFAULT false, position integer NOT NULL,
  UNIQUE(milestone_id,position)
);
CREATE TABLE milestone_dependencies (
  milestone_id uuid NOT NULL REFERENCES milestone_definitions(id), depends_on_id uuid NOT NULL REFERENCES milestone_definitions(id),
  PRIMARY KEY(milestone_id,depends_on_id), CHECK(milestone_id<>depends_on_id)
);
CREATE TABLE activity_instances (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), milestone_id uuid NOT NULL REFERENCES milestone_instances(id),
  definition_id uuid NOT NULL REFERENCES activity_definitions(id), owner_id uuid REFERENCES users(id),
  baseline_due timestamptz, current_due timestamptz, state workflow_state NOT NULL DEFAULT 'not_started',
  completed_at timestamptz, verified_by uuid REFERENCES users(id), version integer NOT NULL DEFAULT 1,
  UNIQUE(milestone_id,definition_id)
);
CREATE TABLE resources (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), milestone_definition_id uuid REFERENCES milestone_definitions(id),
  title text NOT NULL, kind text NOT NULL CHECK(kind IN ('guidance','template','link','file')),
  url text, storage_key text, created_by uuid NOT NULL REFERENCES users(id), active boolean NOT NULL DEFAULT true
);
CREATE TABLE plan_revisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), plan_id uuid NOT NULL REFERENCES student_plans(id),
  source_type text NOT NULL CHECK(source_type IN ('individual','extension','leave','template_migration')),
  source_id uuid, reason text NOT NULL, approved_by uuid NOT NULL REFERENCES users(id), approved_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE schedule_revision_items (
  revision_id uuid NOT NULL REFERENCES plan_revisions(id), milestone_id uuid NOT NULL REFERENCES milestone_instances(id),
  old_start date NOT NULL, old_due date NOT NULL, new_start date NOT NULL, new_due date NOT NULL,
  PRIMARY KEY(revision_id,milestone_id)
);
CREATE TABLE baseline_completions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  milestone_id uuid NOT NULL REFERENCES milestone_instances(id), completed_on date, date_known boolean NOT NULL DEFAULT true,
  explanation text NOT NULL, evidence_key text, verified_by uuid NOT NULL REFERENCES users(id), verified_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(enrolment_id,milestone_id)
);

ALTER TABLE submissions ADD COLUMN IF NOT EXISTS approval_mode text NOT NULL DEFAULT 'primary' CHECK(approval_mode IN ('primary','either','both'));
CREATE TABLE similarity_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), submission_version_id uuid NOT NULL REFERENCES submission_versions(id),
  storage_key text NOT NULL, original_name text NOT NULL, percentage numeric(5,2) CHECK(percentage BETWEEN 0 AND 100),
  uploaded_by uuid NOT NULL REFERENCES users(id), uploaded_at timestamptz NOT NULL DEFAULT now(),
  verified_by uuid REFERENCES users(id), verified_at timestamptz, decision text, exception_reason text,
  UNIQUE(submission_version_id)
);
CREATE TABLE review_decisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), review_assignment_id uuid NOT NULL REFERENCES review_assignments(id),
  decision text NOT NULL CHECK(decision IN ('changes_requested','approved')),
  feedback text, decided_at timestamptz NOT NULL DEFAULT now(), superseded_at timestamptz
);
CREATE UNIQUE INDEX review_one_current_decision ON review_decisions(review_assignment_id) WHERE superseded_at IS NULL;
CREATE TABLE comments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  submission_version_id uuid REFERENCES submission_versions(id), milestone_id uuid REFERENCES milestone_instances(id),
  parent_id uuid REFERENCES comments(id), author_id uuid NOT NULL REFERENCES users(id), body text NOT NULL,
  staff_only boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now(),
  CHECK(submission_version_id IS NOT NULL OR milestone_id IS NOT NULL)
);
CREATE TABLE review_reopenings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), submission_version_id uuid NOT NULL REFERENCES submission_versions(id),
  reason text NOT NULL, reopened_by uuid NOT NULL REFERENCES users(id), reopened_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE meetings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), enrolment_id uuid NOT NULL REFERENCES enrolments(id), milestone_id uuid REFERENCES milestone_instances(id),
  title text NOT NULL, purpose text NOT NULL, scheduled_at timestamptz NOT NULL, duration_minutes integer NOT NULL CHECK(duration_minutes>0),
  location text, meeting_link text, organizer_id uuid NOT NULL REFERENCES users(id),
  state text NOT NULL DEFAULT 'proposed' CHECK(state IN ('proposed','confirmed','change_requested','cancelled','held','not_held')),
  cancellation_reason text, actual_at timestamptz, recorded_at timestamptz, version integer NOT NULL DEFAULT 1
);
CREATE TABLE meeting_participants (
  meeting_id uuid NOT NULL REFERENCES meetings(id), user_id uuid NOT NULL REFERENCES users(id),
  response text NOT NULL DEFAULT 'pending' CHECK(response IN ('pending','acknowledged','change_requested','confirmed')),
  attended boolean, PRIMARY KEY(meeting_id,user_id)
);
CREATE TABLE meeting_notes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), meeting_id uuid NOT NULL REFERENCES meetings(id), author_id uuid NOT NULL REFERENCES users(id),
  body text NOT NULL, state text NOT NULL DEFAULT 'proposed' CHECK(state IN ('proposed','confirmed','flagged')),
  flag_reason text, created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE case_access_grants (
  case_id uuid NOT NULL REFERENCES support_cases(id), user_id uuid NOT NULL REFERENCES users(id),
  granted_by uuid NOT NULL REFERENCES users(id), granted_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(case_id,user_id)
);
CREATE TABLE case_follow_ups (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), case_id uuid NOT NULL REFERENCES support_cases(id), owner_id uuid NOT NULL REFERENCES users(id),
  due_date date NOT NULL, action text NOT NULL, state text NOT NULL DEFAULT 'open' CHECK(state IN ('open','completed','cancelled')),
  completed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE leave_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  start_date date NOT NULL, end_date date NOT NULL, reason_category text NOT NULL, restricted_explanation text,
  proposed_return_date date NOT NULL, state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','coordinator_reviewed','approved','declined')),
  requested_at timestamptz NOT NULL DEFAULT now(), coordinator_id uuid REFERENCES users(id), decided_by uuid REFERENCES users(id),
  decision_reason text, decided_at timestamptz, CHECK(end_date>=start_date), CHECK(proposed_return_date>=end_date)
);
CREATE TABLE extension_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  milestone_id uuid REFERENCES milestone_instances(id), activity_id uuid REFERENCES activity_instances(id),
  requested_by uuid NOT NULL REFERENCES users(id), old_date date NOT NULL, requested_date date NOT NULL, reason text NOT NULL,
  state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','approved','declined','escalated')),
  decided_by uuid REFERENCES users(id), decision_reason text, decided_at timestamptz,
  CHECK(milestone_id IS NOT NULL OR activity_id IS NOT NULL), CHECK(requested_date>old_date)
);

CREATE TABLE committees (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  state text NOT NULL DEFAULT 'draft' CHECK(state IN ('draft','awaiting_acceptance','conflict_review','confirmed','superseded')),
  created_by uuid NOT NULL REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT now(), version integer NOT NULL DEFAULT 1
);
CREATE TABLE committee_members (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), committee_id uuid NOT NULL REFERENCES committees(id), user_id uuid NOT NULL REFERENCES users(id),
  member_role text NOT NULL CHECK(member_role IN ('chair','internal_examiner','external_examiner','department_representative')),
  response text NOT NULL DEFAULT 'pending' CHECK(response IN ('pending','accepted','declined')),
  conflict_state text NOT NULL DEFAULT 'undeclared' CHECK(conflict_state IN ('undeclared','none','declared','resolved')),
  conflict_note text, resolution text, UNIQUE(committee_id,member_role), UNIQUE(committee_id,user_id)
);
CREATE TABLE availability_windows (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), committee_member_id uuid NOT NULL REFERENCES committee_members(id),
  starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL, CHECK(ends_at>starts_at), UNIQUE(committee_member_id,starts_at,ends_at)
);
CREATE TABLE defence_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), committee_id uuid NOT NULL REFERENCES committees(id),
  submission_version_id uuid NOT NULL REFERENCES submission_versions(id), starts_at timestamptz, duration_minutes integer NOT NULL DEFAULT 60,
  venue text, meeting_link text, state text NOT NULL DEFAULT 'proposed' CHECK(state IN ('proposed','awaiting_confirmation','confirmed','completed','cancelled')),
  created_by uuid NOT NULL REFERENCES users(id), version integer NOT NULL DEFAULT 1
);
CREATE TABLE defence_confirmations (
  defence_id uuid NOT NULL REFERENCES defence_events(id), committee_member_id uuid NOT NULL REFERENCES committee_members(id),
  confirmed boolean NOT NULL, responded_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(defence_id,committee_member_id)
);
CREATE TABLE examiner_reports (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), defence_id uuid NOT NULL REFERENCES defence_events(id), examiner_id uuid NOT NULL REFERENCES users(id),
  report_key text NOT NULL, summary text, submitted_at timestamptz NOT NULL DEFAULT now(), released_at timestamptz,
  UNIQUE(defence_id,examiner_id)
);
CREATE TABLE examination_outcomes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), defence_id uuid NOT NULL UNIQUE REFERENCES defence_events(id),
  outcome text NOT NULL CHECK(outcome IN ('passed','corrections_required','resubmission_required','not_passed')),
  summary text NOT NULL, recorded_by uuid NOT NULL REFERENCES users(id), recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE corrections (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), outcome_id uuid NOT NULL REFERENCES examination_outcomes(id),
  title text NOT NULL, due_date date NOT NULL, verifier_id uuid NOT NULL REFERENCES users(id),
  state text NOT NULL DEFAULT 'open' CHECK(state IN ('open','submitted','verified','returned')),
  evidence_key text, verified_at timestamptz
);

CREATE TABLE notification_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), job_type text NOT NULL, dedupe_key text NOT NULL UNIQUE,
  payload jsonb NOT NULL DEFAULT '{}', run_after timestamptz NOT NULL, attempts integer NOT NULL DEFAULT 0,
  locked_at timestamptz, completed_at timestamptz, last_error text
);
CREATE INDEX notification_jobs_ready ON notification_jobs(run_after) WHERE completed_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS notification_jobs,corrections,examination_outcomes,examiner_reports,defence_confirmations,defence_events,availability_windows,committee_members,committees,extension_requests,leave_requests,case_follow_ups,case_access_grants,meeting_notes,meeting_participants,meetings,review_reopenings,comments,review_decisions,similarity_evidence,baseline_completions,schedule_revision_items,plan_revisions,resources,activity_instances,milestone_dependencies,activity_definitions,topic_applications,topic_programmes,topics,allocation_exceptions,supervision_assignments,supervision_requests,supervisor_programme_eligibility,supervisor_profiles CASCADE;
ALTER TABLE submissions DROP COLUMN IF EXISTS approval_mode;
ALTER TABLE milestone_definitions DROP COLUMN IF EXISTS similarity_required;
ALTER TABLE enrolments DROP COLUMN IF EXISTS verification_status,DROP COLUMN IF EXISTS verification_note,DROP COLUMN IF EXISTS verified_by,DROP COLUMN IF EXISTS verified_at;

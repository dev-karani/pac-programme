-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE enrolment_state AS ENUM ('pending_verification','active','approved_leave','withdrawn','completed');
CREATE TYPE workflow_state AS ENUM ('not_started','in_progress','submitted','awaiting_review','changes_requested','approved');
CREATE TYPE case_status AS ENUM ('open','in_progress','awaiting_response','resolved');

CREATE TABLE institutions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  timezone text NOT NULL DEFAULT 'Africa/Nairobi'
);
CREATE TABLE schools (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  institution_id uuid NOT NULL REFERENCES institutions(id),
  name text NOT NULL,
  UNIQUE(institution_id, name)
);
CREATE TABLE departments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  school_id uuid NOT NULL REFERENCES schools(id),
  name text NOT NULL,
  UNIQUE(school_id, name)
);
CREATE TABLE programmes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  department_id uuid NOT NULL REFERENCES departments(id),
  name text NOT NULL,
  code text NOT NULL UNIQUE,
  review_days integer NOT NULL DEFAULT 7 CHECK (review_days > 0),
  warning_days integer NOT NULL DEFAULT 4 CHECK (warning_days >= 0)
);
CREATE TABLE cohorts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  programme_id uuid NOT NULL REFERENCES programmes(id),
  name text NOT NULL,
  intake_date date NOT NULL,
  study_mode text NOT NULL CHECK (study_mode IN ('full_time','part_time')),
  UNIQUE(programme_id, name)
);
CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL UNIQUE,
  full_name text NOT NULL,
  password_hash text NOT NULL,
  student_number text UNIQUE,
  must_change_password boolean NOT NULL DEFAULT true,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE role_assignments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  role text NOT NULL CHECK (role IN ('student','supervisor','coordinator','hod','dean','leadership','examiner','support','admin')),
  scope_type text NOT NULL CHECK (scope_type IN ('institution','school','department','programme','self')),
  scope_id uuid,
  UNIQUE(user_id, role, scope_type, scope_id)
);
CREATE TABLE sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE enrolments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  student_id uuid NOT NULL REFERENCES users(id),
  programme_id uuid NOT NULL REFERENCES programmes(id),
  cohort_id uuid NOT NULL REFERENCES cohorts(id),
  state enrolment_state NOT NULL DEFAULT 'pending_verification',
  admission_date date NOT NULL,
  study_mode text NOT NULL CHECK (study_mode IN ('full_time','part_time')),
  plan_start_date date,
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX one_active_enrolment_per_student ON enrolments(student_id)
  WHERE state IN ('pending_verification','active','approved_leave');
CREATE TABLE programme_templates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  programme_id uuid NOT NULL REFERENCES programmes(id),
  study_mode text NOT NULL,
  name text NOT NULL,
  version integer NOT NULL,
  state text NOT NULL CHECK (state IN ('draft','published','retired')),
  UNIQUE(programme_id, study_mode, version)
);
CREATE TABLE milestone_definitions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  template_id uuid NOT NULL REFERENCES programme_templates(id),
  title text NOT NULL,
  description text NOT NULL DEFAULT '',
  stage_label text NOT NULL,
  position integer NOT NULL,
  start_days integer NOT NULL,
  end_days integer NOT NULL,
  requires_final_signoff boolean NOT NULL DEFAULT false,
  UNIQUE(template_id, position)
);
CREATE TABLE student_plans (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  enrolment_id uuid NOT NULL UNIQUE REFERENCES enrolments(id),
  template_id uuid NOT NULL REFERENCES programme_templates(id),
  started_on date NOT NULL,
  planned_completion date NOT NULL,
  completed_at timestamptz
);
CREATE TABLE milestone_instances (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  plan_id uuid NOT NULL REFERENCES student_plans(id),
  definition_id uuid NOT NULL REFERENCES milestone_definitions(id),
  baseline_start date NOT NULL,
  baseline_due date NOT NULL,
  current_start date NOT NULL,
  current_due date NOT NULL,
  state workflow_state NOT NULL DEFAULT 'not_started',
  version integer NOT NULL DEFAULT 1,
  approved_at timestamptz,
  UNIQUE(plan_id, definition_id)
);
CREATE TABLE action_tasks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  milestone_id uuid REFERENCES milestone_instances(id),
  owner_id uuid NOT NULL REFERENCES users(id),
  title text NOT NULL,
  task_type text NOT NULL,
  due_at timestamptz,
  state text NOT NULL DEFAULT 'open' CHECK (state IN ('open','completed','cancelled')),
  completed_at timestamptz,
  UNIQUE(owner_id, task_type, milestone_id, state)
);
CREATE TABLE submissions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  milestone_id uuid NOT NULL REFERENCES milestone_instances(id),
  student_id uuid NOT NULL REFERENCES users(id),
  current_version integer NOT NULL DEFAULT 0,
  state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft','submitted','changes_requested','approved')),
  UNIQUE(milestone_id, student_id)
);
CREATE TABLE submission_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  submission_id uuid NOT NULL REFERENCES submissions(id),
  version integer NOT NULL,
  storage_key text NOT NULL UNIQUE,
  original_name text NOT NULL,
  content_type text NOT NULL,
  size_bytes bigint NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 26214400),
  submitted_at timestamptz NOT NULL,
  idempotency_key text NOT NULL,
  UNIQUE(submission_id, version),
  UNIQUE(submission_id, idempotency_key)
);
CREATE TABLE review_assignments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  submission_version_id uuid NOT NULL REFERENCES submission_versions(id),
  reviewer_id uuid NOT NULL REFERENCES users(id),
  due_at timestamptz NOT NULL,
  state text NOT NULL DEFAULT 'awaiting_review' CHECK (state IN ('awaiting_review','changes_requested','approved')),
  feedback text,
  decided_at timestamptz,
  UNIQUE(submission_version_id, reviewer_id)
);
CREATE TABLE support_cases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  enrolment_id uuid NOT NULL REFERENCES enrolments(id),
  category text NOT NULL,
  summary text NOT NULL,
  restricted boolean NOT NULL DEFAULT false,
  owner_id uuid REFERENCES users(id),
  status case_status NOT NULL DEFAULT 'open',
  next_follow_up date,
  resolution_summary text,
  created_at timestamptz NOT NULL DEFAULT now(),
  resolved_at timestamptz,
  CHECK ((status <> 'resolved') OR (resolution_summary IS NOT NULL AND length(resolution_summary) > 0))
);
CREATE TABLE notifications (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  recipient_id uuid NOT NULL REFERENCES users(id),
  dedupe_key text NOT NULL,
  title text NOT NULL,
  urgency text NOT NULL,
  read_at timestamptz,
  dismissed_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(recipient_id, dedupe_key)
);
CREATE TABLE audit_events (
  id bigserial PRIMARY KEY,
  actor_id uuid REFERENCES users(id),
  entity_type text NOT NULL,
  entity_id uuid NOT NULL,
  action text NOT NULL,
  safe_summary text NOT NULL,
  occurred_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS audit_events, notifications, support_cases, review_assignments, submission_versions, submissions, action_tasks, milestone_instances, student_plans, milestone_definitions, programme_templates, enrolments, sessions, role_assignments, users, cohorts, programmes, departments, schools, institutions CASCADE;
DROP TYPE IF EXISTS case_status, workflow_state, enrolment_state;

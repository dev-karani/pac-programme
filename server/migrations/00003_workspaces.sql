-- +goose Up
CREATE TABLE research_concepts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), enrolment_id uuid NOT NULL REFERENCES enrolments(id),
 title text NOT NULL, problem text NOT NULL, objectives text NOT NULL, methodology text NOT NULL,
 version integer NOT NULL, state text NOT NULL DEFAULT 'submitted' CHECK(state IN ('submitted','changes_requested','approved','declined')),
 feedback text NOT NULL DEFAULT '', reviewer_id uuid REFERENCES users(id), submitted_at timestamptz NOT NULL DEFAULT now(), decided_at timestamptz,
 UNIQUE(enrolment_id,version)
);
CREATE TABLE case_messages (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),case_id uuid NOT NULL REFERENCES support_cases(id),
 author_id uuid NOT NULL REFERENCES users(id),body text NOT NULL,created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE milestone_definitions ADD COLUMN approval_mode text NOT NULL DEFAULT 'primary' CHECK(approval_mode IN ('primary','either','both'));
CREATE TABLE meeting_history (id uuid PRIMARY KEY DEFAULT gen_random_uuid(),meeting_id uuid NOT NULL REFERENCES meetings(id),actor_id uuid NOT NULL REFERENCES users(id),action text NOT NULL,reason text NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE institution_settings (institution_id uuid PRIMARY KEY REFERENCES institutions(id),settings jsonb NOT NULL DEFAULT '{}');
ALTER TABLE review_assignments ADD COLUMN round_closed boolean NOT NULL DEFAULT false;
INSERT INTO milestone_dependencies(milestone_id,depends_on_id) SELECT md.id,prior.id FROM milestone_definitions md JOIN milestone_definitions prior ON prior.template_id=md.template_id AND prior.position=md.position-1 ON CONFLICT DO NOTHING;
-- +goose Down
DROP TABLE institution_settings,meeting_history,case_messages,research_concepts;
ALTER TABLE review_assignments DROP COLUMN round_closed;
ALTER TABLE milestone_definitions DROP COLUMN approval_mode;

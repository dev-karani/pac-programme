-- +goose Up
ALTER TABLE case_follow_ups ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE meetings ADD CONSTRAINT meeting_duration_limit CHECK(duration_minutes<=1440);
ALTER TABLE defence_events ADD CONSTRAINT defence_duration_limit CHECK(duration_minutes BETWEEN 15 AND 1440);
CREATE INDEX concepts_review_queue ON research_concepts(enrolment_id,state,submitted_at);
CREATE INDEX case_message_thread ON case_messages(case_id,created_at);
CREATE INDEX review_open_queue ON review_assignments(reviewer_id,due_at) WHERE state='awaiting_review';
-- +goose Down
DROP INDEX review_open_queue,case_message_thread,concepts_review_queue;
ALTER TABLE defence_events DROP CONSTRAINT defence_duration_limit;
ALTER TABLE meetings DROP CONSTRAINT meeting_duration_limit;

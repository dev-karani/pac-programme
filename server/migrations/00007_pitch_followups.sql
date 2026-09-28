-- +goose Up
ALTER TABLE notifications ADD COLUMN body text NOT NULL DEFAULT '';
ALTER TABLE notifications ADD COLUMN target_view text NOT NULL DEFAULT '';
ALTER TABLE notifications ADD COLUMN milestone_id uuid REFERENCES milestone_instances(id);
CREATE TABLE followup_messages (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 enrolment_id uuid NOT NULL REFERENCES enrolments(id),
 sender_id uuid NOT NULL REFERENCES users(id),
 recipient_id uuid NOT NULL REFERENCES users(id),
 body text NOT NULL CHECK(length(body) BETWEEN 2 AND 4000),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX followup_enrolment_idx ON followup_messages(enrolment_id,created_at);
-- Atomic notifications: a rolled-back comment or decision never sends an alert.
-- +goose StatementBegin
CREATE FUNCTION notify_academic_comment() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.staff_only THEN RETURN NEW; END IF;
 INSERT INTO notifications(recipient_id,dedupe_key,title,urgency,target_view,milestone_id)
 SELECT DISTINCT recipient,'comment:'||NEW.id,'New discussion reply on your research milestone','info','journey',NEW.milestone_id
 FROM (
 SELECT student_id AS recipient FROM enrolments WHERE id=NEW.enrolment_id
 UNION SELECT supervisor_id FROM supervision_assignments WHERE enrolment_id=NEW.enrolment_id AND effective_from<=now() AND (effective_to IS NULL OR effective_to>now())
 UNION SELECT c.author_id FROM comments c JOIN enrolments e ON e.id=c.enrolment_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id
 WHERE c.milestone_id=NEW.milestone_id AND NOT c.staff_only AND EXISTS(SELECT 1 FROM role_assignments ra WHERE ra.user_id=c.author_id AND ra.role IN('coordinator','hod','dean','leadership') AND ((ra.scope_type='programme' AND ra.scope_id=p.id) OR (ra.scope_type='department' AND ra.scope_id=d.id) OR (ra.scope_type='school' AND ra.scope_id=d.school_id) OR ra.scope_type='institution'))
 ) recipients WHERE recipient<>NEW.author_id ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER academic_comment_notification AFTER INSERT ON comments FOR EACH ROW EXECUTE FUNCTION notify_academic_comment();
-- +goose StatementBegin
CREATE FUNCTION notify_request_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE dest text; label text;
BEGIN
 dest := CASE WHEN TG_TABLE_NAME='supervision_requests' THEN 'supervision' ELSE 'requests' END;
 label := CASE TG_TABLE_NAME WHEN 'supervision_requests' THEN 'Supervision request' WHEN 'leave_requests' THEN 'Leave request' ELSE 'Extension request' END;
 IF TG_OP='UPDATE' AND NEW.state=OLD.state THEN RETURN NEW; END IF;
 INSERT INTO notifications(recipient_id,dedupe_key,title,urgency,target_view)
 SELECT student_id,TG_TABLE_NAME||':'||NEW.id||':'||NEW.state,label||': '||replace(NEW.state,'_',' '),'info',dest FROM enrolments WHERE id=NEW.enrolment_id
 ON CONFLICT DO NOTHING;
 IF TG_TABLE_NAME='supervision_requests' THEN
 INSERT INTO notifications(recipient_id,dedupe_key,title,urgency,target_view) VALUES(NEW.supervisor_id,TG_TABLE_NAME||':'||NEW.id||':'||NEW.state,label||': '||replace(NEW.state,'_',' '),'info',dest) ON CONFLICT DO NOTHING;
 END IF;
 IF NEW.state IN ('pending','accepted','coordinator_reviewed','escalated') THEN
 INSERT INTO notifications(recipient_id,dedupe_key,title,urgency,target_view)
 SELECT DISTINCT ra.user_id,TG_TABLE_NAME||':'||NEW.id||':'||NEW.state,label||' needs attention','info',dest
 FROM enrolments e JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id JOIN role_assignments ra ON ra.role IN('coordinator','hod','dean','leadership')
 WHERE e.id=NEW.enrolment_id AND ((ra.scope_type='programme' AND ra.scope_id=p.id) OR (ra.scope_type='department' AND ra.scope_id=d.id) OR (ra.scope_type='school' AND ra.scope_id=d.school_id) OR ra.scope_type='institution') ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER supervision_notification AFTER INSERT OR UPDATE ON supervision_requests FOR EACH ROW EXECUTE FUNCTION notify_request_change();
CREATE TRIGGER leave_notification AFTER INSERT OR UPDATE ON leave_requests FOR EACH ROW EXECUTE FUNCTION notify_request_change();
CREATE TRIGGER extension_notification AFTER INSERT OR UPDATE ON extension_requests FOR EACH ROW EXECUTE FUNCTION notify_request_change();
-- +goose Down
DROP TRIGGER extension_notification ON extension_requests;
DROP TRIGGER leave_notification ON leave_requests;
DROP TRIGGER supervision_notification ON supervision_requests;
DROP FUNCTION notify_request_change();
DROP TRIGGER academic_comment_notification ON comments;
DROP FUNCTION notify_academic_comment();
DROP TABLE followup_messages;
ALTER TABLE notifications DROP COLUMN body, DROP COLUMN target_view, DROP COLUMN milestone_id;

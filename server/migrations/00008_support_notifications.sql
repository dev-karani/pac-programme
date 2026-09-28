-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION notify_case_message() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO notifications(recipient_id,dedupe_key,title,urgency,target_view)
 SELECT DISTINCT recipient,'case-message:'||NEW.id,'A support request has a new reply','info','support'
 FROM (SELECT e.student_id AS recipient FROM support_cases sc JOIN enrolments e ON e.id=sc.enrolment_id WHERE sc.id=NEW.case_id
 UNION SELECT owner_id FROM support_cases WHERE id=NEW.case_id
 UNION SELECT user_id FROM case_access_grants WHERE case_id=NEW.case_id) recipients
 WHERE recipient IS NOT NULL AND recipient<>NEW.author_id ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER case_message_notification AFTER INSERT ON case_messages FOR EACH ROW EXECUTE FUNCTION notify_case_message();
-- +goose StatementBegin
CREATE FUNCTION notify_assigned_task() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.state='open' THEN
 INSERT INTO notifications(recipient_id,dedupe_key,title,urgency,target_view,milestone_id)
 VALUES(NEW.owner_id,'assigned-task:'||NEW.id,NEW.title,'info',CASE WHEN NEW.task_type LIKE 'meeting%' THEN 'meetings' WHEN NEW.task_type='leave_return' THEN 'requests' ELSE 'journey' END,CASE WHEN NEW.task_type LIKE 'meeting%' OR NEW.task_type='leave_return' THEN NULL ELSE NEW.milestone_id END) ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER assigned_task_notification AFTER INSERT ON action_tasks FOR EACH ROW EXECUTE FUNCTION notify_assigned_task();
-- +goose Down
DROP TRIGGER assigned_task_notification ON action_tasks;
DROP FUNCTION notify_assigned_task();
DROP TRIGGER case_message_notification ON case_messages;
DROP FUNCTION notify_case_message();

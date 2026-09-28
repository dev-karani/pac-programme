-- name: FindUserByEmail :one
SELECT id, email, full_name, password_hash, must_change_password, active
FROM users
WHERE lower(email) = lower(sqlc.arg(email));

-- name: ListUserRoles :many
SELECT role, scope_type, scope_id
FROM role_assignments
WHERE user_id = sqlc.arg(user_id)
ORDER BY role;

-- name: ListStudentMilestones :many
SELECT mi.id, md.title, md.description, md.stage_label, mi.current_start,
       mi.current_due, mi.state, (mi.baseline_due <> mi.current_due) AS revised
FROM enrolments e
JOIN student_plans sp ON sp.enrolment_id = e.id
JOIN milestone_instances mi ON mi.plan_id = sp.id
JOIN milestone_definitions md ON md.id = mi.definition_id
WHERE e.student_id = sqlc.arg(student_id)
ORDER BY md.position;

-- name: ListStudentOpenActions :many
SELECT t.id, t.title, t.task_type, t.due_at, t.milestone_id
FROM action_tasks t
WHERE t.owner_id = sqlc.arg(owner_id) AND t.state = 'open'
ORDER BY t.due_at NULLS LAST;


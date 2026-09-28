package main

import (
	"net/http"
	"strings"
)

func (a *app) followupRecipients(w http.ResponseWriter, r *http.Request) {
	eid := r.PathValue("id")
	if !a.academicAccess(r.Context(), current(r), eid) {
		problem(w, 403, "Access denied", "Outside your academic scope.")
		return
	}
	a.sendRows(w, r, `SELECT u.id,u.full_name,u.email,
 EXISTS(SELECT 1 FROM action_tasks t WHERE t.enrolment_id=e.id AND t.owner_id=u.id AND t.state='open') OR EXISTS(SELECT 1 FROM review_assignments ra JOIN submission_versions v ON v.id=ra.submission_version_id JOIN submissions s ON s.id=v.submission_id JOIN milestone_instances mi ON mi.id=s.milestone_id JOIN student_plans sp ON sp.id=mi.plan_id WHERE sp.enrolment_id=e.id AND ra.reviewer_id=u.id AND ra.state='awaiting_review' AND NOT ra.round_closed) AS needs_action
 FROM users u CROSS JOIN enrolments e JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id
 WHERE e.id=$1 AND u.active AND `+strings.ReplaceAll(academicScope, "$1", "u.id")+` ORDER BY needs_action DESC,u.full_name`, eid)
}
func (a *app) followups(w http.ResponseWriter, r *http.Request) {
	eid := r.PathValue("id")
	u := current(r)
	if !a.academicAccess(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "Outside your academic scope.")
		return
	}
	a.sendRows(w, r, `SELECT f.id,f.body,f.created_at,s.full_name AS sender,t.full_name AS recipient,f.sender_id,f.recipient_id FROM followup_messages f JOIN users s ON s.id=f.sender_id JOIN users t ON t.id=f.recipient_id WHERE f.enrolment_id=$1 AND (f.sender_id=$2 OR f.recipient_id=$2) ORDER BY f.created_at`, eid, u.ID)
}
func (a *app) sendFollowup(w http.ResponseWriter, r *http.Request) {
	eid := r.PathValue("id")
	u := current(r)
	var in struct{ RecipientID, Body string }
	if !decode(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if len(in.Body) < 2 || len(in.Body) > 4000 {
		problem(w, 400, "Message required", "Use 2–4000 characters.")
		return
	}
	if !a.academicAccess(r.Context(), u, eid) || !a.academicAccess(r.Context(), user{ID: in.RecipientID}, eid) || in.RecipientID == u.ID {
		problem(w, 403, "Recipient unavailable", "Choose another authorized participant.")
		return
	}
	var active bool
	if a.db.QueryRowContext(r.Context(), `SELECT active FROM users WHERE id=$1`, in.RecipientID).Scan(&active) != nil || !active {
		problem(w, 403, "Recipient unavailable", "This account is inactive.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 500, "Unavailable", "Try again.")
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO followup_messages(enrolment_id,sender_id,recipient_id,body) VALUES($1,$2,$3,$4) RETURNING id`, eid, u.ID, in.RecipientID, in.Body).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO notifications(recipient_id,dedupe_key,title,body,urgency,target_view) VALUES($1,'followup:'||$2::text,$3,$4,'info','followups')`, in.RecipientID, id, "Follow-up from "+u.Name, in.Body)
	}
	if err == nil {
		err = auditTx(r.Context(), tx, u.ID, "enrolment", eid, "follow_up", "In-app follow-up sent to an authorized participant")
	}
	commitResponse(w, tx, err, map[string]string{"status": "sent_in_app"})
}
func (a *app) readNotification(w http.ResponseWriter, r *http.Request) {
	res, err := a.db.ExecContext(r.Context(), `UPDATE notifications SET read_at=COALESCE(read_at,now()) WHERE id=$1 AND recipient_id=$2`, r.PathValue("id"), current(r).ID)
	if err != nil {
		problem(w, 500, "Unavailable", "Try again.")
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		problem(w, 404, "Not found", "Notification unavailable.")
		return
	}
	w.WriteHeader(204)
}
func (a *app) provisionedOnly(w http.ResponseWriter, r *http.Request) {
	problem(w, http.StatusForbidden, "University-issued accounts only", "Contact university IT for an account. Public registration is disabled; ERP integration is deferred.")
}

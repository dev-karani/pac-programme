package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Academic access is always resolved from the enrolment, never a submitted scope ID.
const academicScope = `(e.student_id=$1 OR EXISTS(SELECT 1 FROM supervision_assignments sa WHERE sa.enrolment_id=e.id AND sa.supervisor_id=$1 AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now())) OR EXISTS(SELECT 1 FROM role_assignments ra WHERE ra.user_id=$1 AND ra.role IN('coordinator','hod','dean','leadership') AND ((ra.scope_type='programme' AND ra.scope_id=e.programme_id) OR (ra.scope_type='department' AND ra.scope_id=p.department_id) OR (ra.scope_type='school' AND ra.scope_id=d.school_id) OR ra.scope_type='institution')))`

func (a *app) academicAccess(ctx context.Context, u user, eid string) bool {
	var ok bool
	_ = a.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM enrolments e JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id WHERE e.id=$2 AND `+academicScope+`)`, u.ID, eid).Scan(&ok)
	return ok
}
func (a *app) assignedSupervisor(ctx context.Context, u user, eid string) bool {
	var ok bool
	_ = a.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM supervision_assignments WHERE enrolment_id=$1 AND supervisor_id=$2 AND effective_from<=now() AND (effective_to IS NULL OR effective_to>now()))`, eid, u.ID).Scan(&ok)
	return ok
}
func (a *app) jsonRows(ctx context.Context, q string, args ...any) ([]map[string]any, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT row_to_json(record) FROM (`+q+`) record`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var v map[string]any
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (a *app) sendRows(w http.ResponseWriter, r *http.Request, q string, args ...any) {
	out, err := a.jsonRows(r.Context(), q, args...)
	if err != nil {
		problem(w, 500, "Records unavailable", "Please try again.")
		return
	}
	writeJSON(w, 200, out)
}
func commitResponse(w http.ResponseWriter, tx *sql.Tx, err error, v any) {
	if err != nil {
		problem(w, 409, "Change not saved", "Check the record, permissions and current state.")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, 409, "Concurrent change", "Refresh and try again.")
		return
	}
	writeJSON(w, 200, v)
}
func (a *app) registerWorkspaceRoutes(m *http.ServeMux) {
	routes := map[string]http.HandlerFunc{
		"GET /api/v1/enrolments/{id}/contacts": a.followupRecipients, "GET /api/v1/enrolments/{id}/followups": a.followups, "POST /api/v1/enrolments/{id}/followups": a.sendFollowup, "POST /api/v1/notifications/{id}/read": a.readNotification,
		"GET /api/v1/workspace/students": a.workspaceStudents, "GET /api/v1/workspace/reports": a.workspaceReports,
		"GET /api/v1/concepts": a.concepts, "POST /api/v1/concepts": a.submitConcept, "POST /api/v1/concepts/{id}/decision": a.decideConcept,
		"GET /api/v1/cases/{id}": a.caseDetail, "POST /api/v1/cases/{id}/message": a.caseMessage, "POST /api/v1/cases/{id}/manage": a.caseManage,
		"GET /api/v1/workspace/supervision": a.supervisionQueue, "POST /api/v1/supervision/requests/{id}/withdraw": a.withdrawSupervision,
		"GET /api/v1/workspace/milestones": a.workspaceMilestones, "GET /api/v1/milestones/{id}/detail": a.milestoneDetail, "POST /api/v1/milestones/{id}/comments": a.addComment,
		"POST /api/v1/activities/{id}/complete": a.completeActivity, "GET /api/v1/workspace/reviews": a.workspaceReviews,
		"GET /api/v1/workspace/requests": a.requestQueue, "GET /api/v1/workspace/people": a.workspacePeople,
	}
	for path, h := range routes {
		m.HandleFunc(path, a.withAuth(h))
	}
}

const studentReport = `SELECT e.id,e.student_id,st.full_name AS student,st.student_number,p.name AS programme,p.id AS programme_id,d.name AS department,d.id AS department_id,sch.name AS school,sch.id AS school_id,c.name AS cohort,e.study_mode,e.state,e.entry_path,e.verification_status,e.verification_note,e.admission_date,e.plan_start_date,
 (SELECT planned_completion FROM student_plans WHERE enrolment_id=e.id) AS planned_completion,
 (SELECT count(*) FROM student_plans sp JOIN milestone_instances mi ON mi.plan_id=sp.id WHERE sp.enrolment_id=e.id AND mi.state='approved') AS completed,
 (SELECT count(*) FROM student_plans sp JOIN milestone_instances mi ON mi.plan_id=sp.id WHERE sp.enrolment_id=e.id) AS total,
 (SELECT count(*) FROM supervision_assignments sa WHERE sa.enrolment_id=e.id AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now())) AS supervisors,
 (SELECT count(*) FROM review_assignments ra JOIN submission_versions sv ON sv.id=ra.submission_version_id JOIN submissions s ON s.id=sv.submission_id WHERE s.student_id=e.student_id AND ra.state='awaiting_review' AND ra.due_at<now()) AS overdue_reviews,
 (SELECT count(*) FROM support_cases sc WHERE sc.enrolment_id=e.id AND sc.status<>'resolved') AS open_cases,
 (SELECT max(actual_at) FROM meetings WHERE enrolment_id=e.id AND state='held') AS last_meeting,
 (SELECT string_agg(DISTINCT owners.name,', ') FROM (SELECT u.full_name AS name FROM action_tasks t JOIN users u ON u.id=t.owner_id WHERE t.enrolment_id=e.id AND t.state='open' UNION SELECT reviewer.full_name FROM review_assignments ra JOIN users reviewer ON reviewer.id=ra.reviewer_id JOIN submission_versions sv ON sv.id=ra.submission_version_id JOIN submissions sub ON sub.id=sv.submission_id JOIN milestone_instances mi ON mi.id=sub.milestone_id JOIN student_plans plan ON plan.id=mi.plan_id WHERE plan.enrolment_id=e.id AND ra.state='awaiting_review' AND NOT ra.round_closed) owners) AS next_action_owner,
 (SELECT string_agg(DISTINCT md.stage_label,', ') FROM student_plans sp JOIN milestone_instances mi ON mi.plan_id=sp.id JOIN milestone_definitions md ON md.id=mi.definition_id WHERE sp.enrolment_id=e.id AND mi.state IN('in_progress','awaiting_review','changes_requested')) AS stage,
 (SELECT count(*) FROM student_plans sp JOIN milestone_instances mi ON mi.plan_id=sp.id WHERE sp.enrolment_id=e.id AND mi.state IN('in_progress','changes_requested') AND mi.current_due<current_date AND e.state='active') AS overdue_milestones
 FROM enrolments e JOIN users st ON st.id=e.student_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id JOIN schools sch ON sch.id=d.school_id JOIN cohorts c ON c.id=e.cohort_id WHERE ` + academicScope

func (a *app) workspaceStudents(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, studentReport+` ORDER BY st.full_name`, current(r).ID)
}
func (a *app) workspaceReports(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "supervisor", "coordinator", "hod", "dean", "leadership") {
		return
	}
	query := studentReport + ` ORDER BY st.full_name`
	kind := r.URL.Query().Get("kind")
	if kind == "reviews" {
		query = `SELECT ra.id,u.full_name AS student,p.name AS programme,d.name AS department,sch.name AS school,c.name AS cohort,e.study_mode,e.state,e.admission_date,reviewer.full_name AS reviewer,md.title AS milestone,ra.state AS review_state,ra.due_at,ra.decided_at,round((extract(epoch FROM(ra.decided_at-sv.submitted_at))/86400)::numeric,1) AS turnaround_days FROM review_assignments ra JOIN users reviewer ON reviewer.id=ra.reviewer_id JOIN submission_versions sv ON sv.id=ra.submission_version_id JOIN submissions s ON s.id=sv.submission_id JOIN milestone_instances mi ON mi.id=s.milestone_id JOIN milestone_definitions md ON md.id=mi.definition_id JOIN student_plans sp ON sp.id=mi.plan_id JOIN enrolments e ON e.id=sp.enrolment_id JOIN users u ON u.id=e.student_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id JOIN schools sch ON sch.id=d.school_id JOIN cohorts c ON c.id=e.cohort_id WHERE ` + academicScope + ` ORDER BY ra.due_at`
	}
	if kind == "cases" {
		query = `SELECT sc.id,u.full_name AS student,p.name AS programme,d.name AS department,sch.name AS school,c.name AS cohort,e.study_mode,e.state,e.admission_date,CASE WHEN sc.restricted THEN 'restricted support' ELSE sc.category END AS category,sc.status AS case_status,owner.full_name AS owner,sc.next_follow_up FROM support_cases sc JOIN enrolments e ON e.id=sc.enrolment_id JOIN users u ON u.id=e.student_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id JOIN schools sch ON sch.id=d.school_id JOIN cohorts c ON c.id=e.cohort_id LEFT JOIN users owner ON owner.id=sc.owner_id WHERE ` + academicScope + ` ORDER BY sc.next_follow_up NULLS LAST`
	}
	rows, err := a.jsonRows(r.Context(), query, u.ID)
	if err != nil {
		problem(w, 500, "Report unavailable", "Try again.")
		return
	}
	out := []map[string]any{}
	for _, row := range rows {
		match := true
		switch r.URL.Query().Get("kind") {
		case "overdue":
			match = row["overdue_reviews"].(float64) > 0 || row["overdue_milestones"].(float64) > 0
		case "allocation":
			match = row["supervisors"].(float64) < 2 && row["state"] == "active"
		case "leave":
			match = row["state"] == "approved_leave"
		case "completed":
			match = row["state"] == "completed"
		}
		if from := r.URL.Query().Get("from"); from != "" && fmt.Sprint(row["admission_date"]) < from {
			match = false
		}
		if to := r.URL.Query().Get("to"); to != "" && fmt.Sprint(row["admission_date"]) > to {
			match = false
		}
		for _, k := range []string{"school", "department", "programme", "cohort", "study_mode", "state", "stage"} {
			if v := r.URL.Query().Get(k); v != "" && !strings.Contains(strings.ToLower(fmt.Sprint(row[k])), strings.ToLower(v)) {
				match = false
			}
		}
		if q := strings.ToLower(r.URL.Query().Get("q")); q != "" && !strings.Contains(strings.ToLower(fmt.Sprint(row["student"], row["student_number"])), q) {
			match = false
		}
		if match {
			out = append(out, row)
		}
	}
	if r.URL.Query().Get("format") == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="progress-report.csv"`)
		cw := csv.NewWriter(w)
		keys := []string{"student", "student_number", "school", "department", "programme", "cohort", "study_mode", "state", "stage", "completed", "total", "supervisors", "overdue_reviews", "overdue_milestones", "open_cases", "next_action_owner", "planned_completion"}
		if kind == "reviews" {
			keys = []string{"student", "school", "department", "programme", "reviewer", "milestone", "review_state", "due_at", "decided_at", "turnaround_days"}
		}
		if kind == "cases" {
			keys = []string{"student", "school", "department", "programme", "category", "case_status", "owner", "next_follow_up"}
		}
		_ = cw.Write(keys)
		for _, row := range out {
			v := []string{}
			for _, k := range keys {
				value := ""
				if row[k] != nil {
					value = fmt.Sprint(row[k])
				}
				v = append(v, csvSafe(value))
			}
			_ = cw.Write(v)
		}
		cw.Flush()
		return
	}
	writeJSON(w, 200, out)
}
func (a *app) concepts(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT rc.*,u.full_name AS student,reviewer.full_name AS reviewer FROM research_concepts rc JOIN enrolments e ON e.id=rc.enrolment_id JOIN users u ON u.id=e.student_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id LEFT JOIN users reviewer ON reviewer.id=rc.reviewer_id WHERE `+academicScope+` ORDER BY rc.submitted_at DESC`, current(r).ID)
}
func (a *app) submitConcept(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Title, Problem, Objectives, Methodology string }
	if !decode(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Title)) < 5 || len(in.Problem) < 20 || len(in.Objectives) < 10 || len(in.Methodology) < 10 {
		problem(w, 400, "Incomplete concept", "Provide a title, problem statement, objectives and methodology.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 500, "Unavailable", "Try again.")
		return
	}
	defer tx.Rollback()
	var eid, id, state string
	err = tx.QueryRowContext(r.Context(), `SELECT id FROM enrolments WHERE student_id=$1 AND state='active' FOR UPDATE`, u.ID).Scan(&eid)
	if err == nil {
		_ = tx.QueryRowContext(r.Context(), `SELECT state FROM research_concepts WHERE enrolment_id=$1 ORDER BY version DESC LIMIT 1`, eid).Scan(&state)
		if state == "submitted" || state == "approved" {
			problem(w, 409, "Concept already submitted", "Wait for a decision or requested revisions before submitting another version.")
			return
		}
		err = tx.QueryRowContext(r.Context(), `INSERT INTO research_concepts(enrolment_id,title,problem,objectives,methodology,version) SELECT $1,$2,$3,$4,$5,COALESCE(max(version),0)+1 FROM research_concepts WHERE enrolment_id=$1 RETURNING id`, eid, in.Title, in.Problem, in.Objectives, in.Methodology).Scan(&id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO notifications(recipient_id,dedupe_key,title,urgency) SELECT supervisor_id,'concept:'||$2::text,'Research concept awaiting your review','info' FROM supervision_assignments WHERE enrolment_id=$1 AND effective_from<=now() AND (effective_to IS NULL OR effective_to>now()) ON CONFLICT DO NOTHING`, eid, id)
	}
	commitResponse(w, tx, err, map[string]string{"id": id})
}
func (a *app) decideConcept(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Decision, Feedback string }
	if !decode(w, r, &in) {
		return
	}
	if (in.Decision != "approved" && in.Decision != "changes_requested" && in.Decision != "declined") || len(strings.TrimSpace(in.Feedback)) < 5 {
		problem(w, 400, "Decision required", "Select a decision and provide feedback.")
		return
	}
	var eid string
	_ = a.db.QueryRowContext(r.Context(), `SELECT enrolment_id FROM research_concepts WHERE id=$1`, r.PathValue("id")).Scan(&eid)
	if !a.assignedSupervisor(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "Only the current assigned supervisor can decide this concept.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `UPDATE research_concepts SET state=$2,feedback=$3,reviewer_id=$4,decided_at=now() WHERE id=$1 AND state='submitted' RETURNING id`, r.PathValue("id"), in.Decision, in.Feedback, u.ID).Scan(&id)
	if err == nil {
		err = auditTx(r.Context(), tx, u.ID, "research_concept", id, in.Decision, "Concept review decision recorded")
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO notifications(recipient_id,dedupe_key,title,urgency) SELECT student_id,'concept-decision:'||$2::text,'Your research concept has been reviewed','info' FROM enrolments WHERE id=$1 ON CONFLICT DO NOTHING`, eid, id)
	}
	commitResponse(w, tx, err, map[string]string{"status": in.Decision})
}

func (a *app) canReadCase(ctx context.Context, u user, id string) (bool, bool) {
	var eid, student string
	var restricted, assigned bool
	err := a.db.QueryRowContext(ctx, `SELECT sc.enrolment_id,e.student_id,sc.restricted,COALESCE(sc.owner_id=$2,false) OR EXISTS(SELECT 1 FROM case_access_grants WHERE case_id=sc.id AND user_id=$2) FROM support_cases sc JOIN enrolments e ON e.id=sc.enrolment_id WHERE sc.id=$1`, id, u.ID).Scan(&eid, &student, &restricted, &assigned)
	if err != nil {
		return false, false
	}
	manage := assigned || (!restricted && a.canManageEnrolment(ctx, u, eid))
	return student == u.ID || manage || (!restricted && a.academicAccess(ctx, u, eid)), manage
}
func (a *app) caseDetail(w http.ResponseWriter, r *http.Request) {
	ok, manage := a.canReadCase(r.Context(), current(r), r.PathValue("id"))
	if !ok {
		problem(w, 404, "Request unavailable", "This request is outside your access.")
		return
	}
	rows, err := a.jsonRows(r.Context(), `SELECT sc.*,u.full_name AS student,owner.full_name AS owner FROM support_cases sc JOIN enrolments e ON e.id=sc.enrolment_id JOIN users u ON u.id=e.student_id LEFT JOIN users owner ON owner.id=sc.owner_id WHERE sc.id=$1`, r.PathValue("id"))
	if err != nil || len(rows) == 0 {
		problem(w, 404, "Request unavailable", "Try again.")
		return
	}
	messages, _ := a.jsonRows(r.Context(), `SELECT cm.*,u.full_name AS author FROM case_messages cm JOIN users u ON u.id=cm.author_id WHERE case_id=$1 ORDER BY created_at`, r.PathValue("id"))
	followups, _ := a.jsonRows(r.Context(), `SELECT f.*,u.full_name AS owner FROM case_follow_ups f JOIN users u ON u.id=f.owner_id WHERE case_id=$1 ORDER BY created_at`, r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"case": rows[0], "messages": messages, "followups": followups, "canManage": manage})
}
func (a *app) caseMessage(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	ok, _ := a.canReadCase(r.Context(), u, r.PathValue("id"))
	if !ok {
		problem(w, 403, "Access denied", "Request unavailable.")
		return
	}
	var in struct{ Body string }
	if !decode(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Body)) < 2 || len(in.Body) > 4000 {
		problem(w, 400, "Message required", "Enter your reply.")
		return
	}
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO case_messages(case_id,author_id,body) VALUES($1,$2,$3)`, r.PathValue("id"), u.ID, in.Body)
	if err != nil {
		problem(w, 500, "Reply not saved", "Try again.")
		return
	}
	writeJSON(w, 201, map[string]string{"status": "sent"})
}
func (a *app) caseManage(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	_, ok := a.canReadCase(r.Context(), u, r.PathValue("id"))
	if !ok {
		problem(w, 403, "Access denied", "Only assigned staff may manage this request.")
		return
	}
	var in struct{ Status, Resolution, Action, DueDate, FollowupID string }
	if !decode(w, r, &in) {
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	if in.Status != "" {
		if in.Status == "resolved" && len(in.Resolution) < 8 {
			problem(w, 400, "Resolution required", "Explain how this request was resolved.")
			return
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE support_cases SET status=$2::case_status,resolution_summary=NULLIF($3,''),resolved_at=CASE WHEN $2='resolved' THEN now() ELSE NULL END,owner_id=COALESCE(owner_id,$4) WHERE id=$1`, r.PathValue("id"), in.Status, in.Resolution, u.ID)
	}
	if err == nil && in.Action != "" {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO case_follow_ups(case_id,owner_id,due_date,action) VALUES($1,$2,$3,$4)`, r.PathValue("id"), u.ID, in.DueDate, in.Action)
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE support_cases SET next_follow_up=$2 WHERE id=$1`, r.PathValue("id"), in.DueDate)
		}
	}
	if err == nil && in.FollowupID != "" {
		_, err = tx.ExecContext(r.Context(), `UPDATE case_follow_ups SET state='completed',completed_at=now() WHERE id=$1 AND case_id=$2 AND owner_id=$3`, in.FollowupID, r.PathValue("id"), u.ID)
	}
	if err == nil {
		err = auditTx(r.Context(), tx, u.ID, "support_case", r.PathValue("id"), "updated", "Support case status or follow-up updated")
	}
	commitResponse(w, tx, err, map[string]string{"status": "saved"})
}

func (a *app) supervisionQueue(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT sr.*,u.full_name AS supervisor,st.full_name AS student FROM supervision_requests sr JOIN enrolments e ON e.id=sr.enrolment_id JOIN users st ON st.id=e.student_id JOIN users u ON u.id=sr.supervisor_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id WHERE sr.supervisor_id=$1 OR `+academicScope+` ORDER BY requested_at DESC`, current(r).ID)
}
func (a *app) withdrawSupervision(w http.ResponseWriter, r *http.Request) {
	res, err := a.db.ExecContext(r.Context(), `UPDATE supervision_requests SET state='withdrawn' WHERE id=$1 AND state IN('pending','accepted','expired') AND enrolment_id IN(SELECT id FROM enrolments WHERE student_id=$2)`, r.PathValue("id"), current(r).ID)
	if err != nil {
		problem(w, 409, "Cannot withdraw", "Refresh the request.")
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		problem(w, 409, "Cannot withdraw", "The request is already allocated or unavailable.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "withdrawn"})
}
func (a *app) workspaceMilestones(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT mi.*,md.title,md.description,md.stage_label,md.similarity_required,e.id AS enrolment_id,u.full_name AS student FROM milestone_instances mi JOIN milestone_definitions md ON md.id=mi.definition_id JOIN student_plans sp ON sp.id=mi.plan_id JOIN enrolments e ON e.id=sp.enrolment_id JOIN users u ON u.id=e.student_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id WHERE `+academicScope+` ORDER BY u.full_name,md.position`, current(r).ID)
}
func (a *app) milestoneEnrolment(r *http.Request) (string, bool) {
	var eid string
	err := a.db.QueryRowContext(r.Context(), `SELECT sp.enrolment_id FROM milestone_instances mi JOIN student_plans sp ON sp.id=mi.plan_id WHERE mi.id=$1`, r.PathValue("id")).Scan(&eid)
	return eid, err == nil && a.academicAccess(r.Context(), current(r), eid)
}
func (a *app) milestoneDetail(w http.ResponseWriter, r *http.Request) {
	_, ok := a.milestoneEnrolment(r)
	if !ok {
		problem(w, 404, "Milestone unavailable", "Outside your access.")
		return
	}
	id := r.PathValue("id")
	activities, _ := a.jsonRows(r.Context(), `SELECT ai.*,ad.title,ad.instructions,ad.required,ad.activity_type FROM activity_instances ai JOIN activity_definitions ad ON ad.id=ai.definition_id WHERE ai.milestone_id=$1 ORDER BY ad.position`, id)
	resources, _ := a.jsonRows(r.Context(), `SELECT rs.id,rs.title,rs.kind,rs.url FROM resources rs JOIN milestone_instances mi ON mi.definition_id=rs.milestone_definition_id WHERE mi.id=$1 AND rs.active`, id)
	versions, _ := a.jsonRows(r.Context(), `SELECT sv.id,sv.version,sv.original_name,sv.size_bytes,sv.submitted_at,s.state,se.id AS similarity_id,se.verified_at,se.percentage FROM submission_versions sv JOIN submissions s ON s.id=sv.submission_id LEFT JOIN similarity_evidence se ON se.submission_version_id=sv.id WHERE s.milestone_id=$1 ORDER BY sv.version DESC`, id)
	comments, _ := a.jsonRows(r.Context(), `SELECT c.id,c.body,c.created_at,c.submission_version_id,u.full_name AS author FROM comments c JOIN users u ON u.id=c.author_id WHERE c.milestone_id=$1 AND NOT c.staff_only ORDER BY c.created_at`, id)
	decisions, _ := a.jsonRows(r.Context(), `SELECT ra.id,ra.state,ra.feedback,ra.decided_at,sv.version,u.full_name AS reviewer FROM review_assignments ra JOIN submission_versions sv ON sv.id=ra.submission_version_id JOIN submissions s ON s.id=sv.submission_id JOIN users u ON u.id=ra.reviewer_id WHERE s.milestone_id=$1 ORDER BY sv.version,ra.decided_at`, id)
	writeJSON(w, 200, map[string]any{"activities": activities, "resources": resources, "versions": versions, "comments": comments, "decisions": decisions})
}
func (a *app) addComment(w http.ResponseWriter, r *http.Request) {
	eid, ok := a.milestoneEnrolment(r)
	if !ok {
		problem(w, 403, "Access denied", "Milestone unavailable.")
		return
	}
	var in struct{ Body, VersionID string }
	if !decode(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Body)) < 2 {
		problem(w, 400, "Comment required", "Enter a comment.")
		return
	}
	if in.VersionID != "" {
		var exists bool
		_ = a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM submission_versions v JOIN submissions s ON s.id=v.submission_id WHERE v.id=$1 AND s.milestone_id=$2)`, in.VersionID, r.PathValue("id")).Scan(&exists)
		if !exists {
			problem(w, 400, "Invalid version", "Choose a version from this milestone.")
			return
		}
	}
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO comments(enrolment_id,milestone_id,submission_version_id,author_id,body) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5)`, eid, r.PathValue("id"), in.VersionID, current(r).ID, in.Body)
	if err != nil {
		problem(w, 500, "Comment not saved", "Try again.")
		return
	}
	writeJSON(w, 201, map[string]string{"status": "posted"})
}
func (a *app) completeActivity(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var eid, kind, student string
	err := a.db.QueryRowContext(r.Context(), `SELECT e.id,ad.activity_type,e.student_id FROM activity_instances ai JOIN activity_definitions ad ON ad.id=ai.definition_id JOIN milestone_instances mi ON mi.id=ai.milestone_id JOIN student_plans sp ON sp.id=mi.plan_id JOIN enrolments e ON e.id=sp.enrolment_id WHERE ai.id=$1`, r.PathValue("id")).Scan(&eid, &kind, &student)
	if err != nil || kind == "submission" || (kind == "checkpoint" && student != u.ID) || (kind == "verified" && !a.assignedSupervisor(r.Context(), u, eid) && !a.canManageEnrolment(r.Context(), u, eid)) {
		problem(w, 403, "Action unavailable", "This activity needs its designated owner or a formal submission.")
		return
	}
	var in struct{ Reopen bool }
	if !decode(w, r, &in) {
		return
	}
	state := "approved"
	if in.Reopen {
		state = "in_progress"
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `UPDATE activity_instances SET state=$2,completed_at=CASE WHEN $2='approved' THEN now() END,verified_by=$3,version=version+1 WHERE id=$1`, r.PathValue("id"), state, u.ID)
	if err == nil && !in.Reopen {
		_, err = tx.ExecContext(r.Context(), `UPDATE milestone_instances mi SET state='approved',approved_at=now() FROM milestone_definitions md WHERE md.id=mi.definition_id AND mi.id=(SELECT milestone_id FROM activity_instances WHERE id=$1) AND NOT md.requires_final_signoff AND NOT EXISTS(SELECT 1 FROM activity_instances ai JOIN activity_definitions ad ON ad.id=ai.definition_id WHERE ai.milestone_id=mi.id AND ad.required AND ai.state<>'approved')`, r.PathValue("id"))
	}
	if err == nil && in.Reopen {
		_, err = tx.ExecContext(r.Context(), `UPDATE milestone_instances SET state='in_progress',approved_at=NULL WHERE id=(SELECT milestone_id FROM activity_instances WHERE id=$1)`, r.PathValue("id"))
	}
	if err == nil {
		err = advancePlan(r.Context(), tx, eid)
	}
	if err == nil {
		err = auditTx(r.Context(), tx, u.ID, "activity", r.PathValue("id"), state, "Activity completion changed; dependent records preserved")
	}
	commitResponse(w, tx, err, map[string]string{"status": state})
}
func (a *app) workspaceReviews(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT ra.*,sv.original_name,sv.version,sv.id AS version_id,s.milestone_id,u.full_name AS student,md.title,md.similarity_required,se.id AS similarity_id,se.verified_at AS similarity_verified FROM review_assignments ra JOIN submission_versions sv ON sv.id=ra.submission_version_id JOIN submissions s ON s.id=sv.submission_id JOIN users u ON u.id=s.student_id JOIN milestone_instances mi ON mi.id=s.milestone_id JOIN milestone_definitions md ON md.id=mi.definition_id LEFT JOIN similarity_evidence se ON se.submission_version_id=sv.id WHERE ra.reviewer_id=$1 AND EXISTS(SELECT 1 FROM student_plans pl JOIN supervision_assignments sa ON sa.enrolment_id=pl.enrolment_id WHERE pl.id=mi.plan_id AND sa.supervisor_id=$1 AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now())) ORDER BY ra.due_at`, current(r).ID)
}
func (a *app) requestQueue(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	leaves, _ := a.jsonRows(r.Context(), `SELECT lr.id,lr.enrolment_id,lr.start_date,lr.end_date,lr.proposed_return_date,lr.reason_category,lr.state,lr.decision_reason,lr.decided_at,(SELECT full_name FROM users WHERE id=lr.decided_by) AS decider,u.full_name AS student FROM leave_requests lr JOIN enrolments e ON e.id=lr.enrolment_id JOIN users u ON u.id=e.student_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id WHERE `+academicScope+` ORDER BY lr.requested_at DESC`, u.ID)
	extensions, _ := a.jsonRows(r.Context(), `SELECT er.*,(SELECT md.title FROM milestone_instances mi JOIN milestone_definitions md ON md.id=mi.definition_id WHERE mi.id=er.milestone_id) AS milestone,(SELECT full_name FROM users WHERE id=er.decided_by) AS decider,u.full_name AS student FROM extension_requests er JOIN enrolments e ON e.id=er.enrolment_id JOIN users u ON u.id=e.student_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id WHERE `+academicScope+` ORDER BY er.requested_date DESC`, u.ID)
	writeJSON(w, 200, map[string]any{"leave": leaves, "extensions": extensions})
}
func (a *app) workspacePeople(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT DISTINCT u.id,u.full_name FROM users u WHERE u.id=$1 OR EXISTS(SELECT 1 FROM enrolments e JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id WHERE `+academicScope+` AND (e.student_id=u.id OR EXISTS(SELECT 1 FROM supervision_assignments sa WHERE sa.enrolment_id=e.id AND sa.supervisor_id=u.id AND sa.effective_to IS NULL))) ORDER BY u.full_name`, current(r).ID)
}

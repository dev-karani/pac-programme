package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *app) registerReleaseRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/change-password", a.withAuth(a.changePassword))
	m.HandleFunc("POST /api/v1/admin/users/{id}/reset-password", a.withAuth(a.resetPassword))
	m.HandleFunc("POST /api/v1/admin/users/{id}/deactivate", a.withAuth(a.deactivateUser))
	m.HandleFunc("GET /api/v1/organization", a.withAuth(a.organization))
	m.HandleFunc("GET /api/v1/programme-details", a.withAuth(a.programmeDetails))
	m.HandleFunc("POST /api/v1/onboarding/submit", a.withAuth(a.onboardingSubmit))
	m.HandleFunc("POST /api/v1/enrolments/{id}/verify", a.withAuth(a.verifyEnrolment))
	m.HandleFunc("POST /api/v1/enrolments/{id}/baseline", a.withAuth(a.addBaseline))
	m.HandleFunc("GET /api/v1/supervisors", a.withAuth(a.supervisors))
	m.HandleFunc("GET /api/v1/supervision", a.withAuth(a.supervision))
	m.HandleFunc("POST /api/v1/supervision/requests", a.withAuth(a.requestSupervisor))
	m.HandleFunc("POST /api/v1/supervision/requests/{id}/respond", a.withAuth(a.respondSupervision))
	m.HandleFunc("POST /api/v1/supervision/requests/{id}/allocate", a.withAuth(a.allocateSupervisor))
	m.HandleFunc("POST /api/v1/supervision/replace", a.withAuth(a.replaceSupervisor))
	m.HandleFunc("GET /api/v1/reviews", a.withAuth(a.workspaceReviews))
	m.HandleFunc("POST /api/v1/reviews/{id}/decision", a.withAuth(a.reviewDecision))
	m.HandleFunc("POST /api/v1/submission-versions/{id}/similarity", a.withAuth(a.uploadSimilarity))
	m.HandleFunc("POST /api/v1/similarity/{id}/verify", a.withAuth(a.verifySimilarity))
	m.HandleFunc("GET /api/v1/similarity/{id}/file", a.withAuth(a.downloadSimilarity))
	m.HandleFunc("GET /api/v1/meetings", a.withAuth(a.meetings))
	m.HandleFunc("POST /api/v1/meetings", a.withAuth(a.createMeeting))
	m.HandleFunc("POST /api/v1/meetings/{id}/record", a.withAuth(a.recordMeeting))
	m.HandleFunc("POST /api/v1/cases/{id}/transition", a.withAuth(a.transitionCase))
	m.HandleFunc("POST /api/v1/leave", a.withAuth(a.requestLeave))
	m.HandleFunc("POST /api/v1/leave/{id}/decide", a.withAuth(a.decideLeave))
	m.HandleFunc("POST /api/v1/extensions", a.withAuth(a.requestExtension))
	m.HandleFunc("POST /api/v1/extensions/{id}/decide", a.withAuth(a.decideExtension))
	m.HandleFunc("GET /api/v1/examination", a.withAuth(a.examination))
	m.HandleFunc("POST /api/v1/committees", a.withAuth(a.createCommittee))
	m.HandleFunc("POST /api/v1/committee-members/{id}/declare", a.withAuth(a.declareConflict))
	m.HandleFunc("POST /api/v1/committee-members/{id}/availability", a.withAuth(a.addAvailability))
	m.HandleFunc("POST /api/v1/committee-members/{id}/resolve-conflict", a.withAuth(a.resolveConflict))
	m.HandleFunc("POST /api/v1/committees/{id}/confirm", a.withAuth(a.confirmCommittee))
	m.HandleFunc("GET /api/v1/committees/{id}/overlaps", a.withAuth(a.availabilityOverlaps))
	m.HandleFunc("POST /api/v1/defences", a.withAuth(a.scheduleDefence))
	m.HandleFunc("POST /api/v1/defences/{id}/outcome", a.withAuth(a.recordOutcome))
	m.HandleFunc("POST /api/v1/corrections/{id}/verify", a.withAuth(a.verifyCorrection))
	m.HandleFunc("GET /api/v1/notifications", a.withAuth(a.notifications))
	m.HandleFunc("POST /api/v1/notifications/{id}/dismiss", a.withAuth(a.dismissNotification))
	m.HandleFunc("POST /api/v1/jobs/run", a.withAuth(a.runReminderJobs))
	m.HandleFunc("GET /api/v1/reports/progress.csv", a.withAuth(a.progressCSV))
	m.HandleFunc("GET /api/v1/audit", a.withAuth(a.audit))
}
func (a *app) changePassword(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Current, Next string }
	if !decode(w, r, &in) {
		return
	}
	if len(in.Next) < 12 {
		problem(w, 400, "Password is too short", "Use at least 12 characters.")
		return
	}
	var old string
	if a.db.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id=$1`, u.ID).Scan(&old) != nil || bcrypt.CompareHashAndPassword([]byte(old), []byte(in.Current)) != nil {
		problem(w, 400, "Current password is incorrect", "Check it and try again.")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Next), bcrypt.DefaultCost)
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	_, err := tx.ExecContext(r.Context(), `UPDATE users SET password_hash=$2,must_change_password=false WHERE id=$1`, u.ID, string(hash))
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=now() WHERE user_id=$1`, u.ID)
	}
	if err != nil {
		problem(w, 500, "Password not changed", "Try again.")
		return
	}
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "pac_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(204)
}
func (a *app) resetPassword(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "admin") {
		return
	}
	var in struct{ Temporary string }
	if !decode(w, r, &in) || len(in.Temporary) < 12 {
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Temporary), bcrypt.DefaultCost)
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `UPDATE users SET password_hash=$2,must_change_password=true WHERE id=$1`, r.PathValue("id"), string(hash))
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=now() WHERE user_id=$1`, r.PathValue("id"))
	}
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if err != nil || n != 1 {
		problem(w, 404, "Account not found", "No account was changed.")
		return
	}
	auditTx(r.Context(), tx, u.ID, "user", r.PathValue("id"), "password_reset", "Administrator reset temporary credentials")
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	w.WriteHeader(204)
}
func (a *app) deactivateUser(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "admin") {
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `UPDATE users SET active=false WHERE id=$1 AND id<>$2`, r.PathValue("id"), u.ID)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=now() WHERE user_id=$1`, r.PathValue("id"))
	}
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if err != nil || n != 1 {
		problem(w, 409, "Account not deactivated", "The account was missing or is your own.")
		return
	}
	auditTx(r.Context(), tx, u.ID, "user", r.PathValue("id"), "deactivated", "Account deactivated and sessions revoked")
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	w.WriteHeader(204)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		problem(w, 400, "Check the form", "Some required information is missing or invalid.")
		return false
	}
	return true
}
func requireAny(w http.ResponseWriter, u user, roles ...string) bool {
	for _, x := range roles {
		if hasRole(u, x) {
			return true
		}
	}
	problem(w, 403, "Access denied", "Your assigned role does not permit this action.")
	return false
}
func auditTx(ctx context.Context, tx *sql.Tx, actor, kind, id, action, summary string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(actor_id,entity_type,entity_id,action,safe_summary) VALUES($1,$2,$3,$4,$5)`, actor, kind, id, action, summary)
	return err
}
func (a *app) canManageEnrolment(ctx context.Context, u user, enrolmentID string) bool {
	var ok bool
	_ = a.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM enrolments e JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id JOIN role_assignments ra ON ra.user_id=$2 WHERE e.id=$1 AND ra.role IN('coordinator','hod','dean','leadership') AND ((ra.scope_type='programme' AND ra.scope_id=p.id) OR (ra.scope_type='department' AND ra.scope_id=d.id) OR (ra.scope_type='school' AND ra.scope_id=d.school_id) OR ra.scope_type='institution'))`, enrolmentID, u.ID).Scan(&ok)
	return ok
}
func (a *app) enrolmentFor(ctx context.Context, table, id string) (string, error) {
	allowed := map[string]string{"supervision_requests": "SELECT enrolment_id FROM supervision_requests WHERE id=$1", "leave_requests": "SELECT enrolment_id FROM leave_requests WHERE id=$1", "extension_requests": "SELECT enrolment_id FROM extension_requests WHERE id=$1", "support_cases": "SELECT enrolment_id FROM support_cases WHERE id=$1", "defence_events": "SELECT c.enrolment_id FROM defence_events de JOIN committees c ON c.id=de.committee_id WHERE de.id=$1"}
	q := allowed[table]
	if q == "" {
		return "", sql.ErrNoRows
	}
	var eid string
	err := a.db.QueryRowContext(ctx, q, id).Scan(&eid)
	return eid, err
}

func (a *app) organization(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT s.id,s.name,d.id,d.name,p.id,p.name,p.code FROM schools s JOIN departments d ON d.school_id=s.id JOIN programmes p ON p.department_id=d.id ORDER BY s.name,d.name,p.name`)
	if err != nil {
		problem(w, 500, "Organization unavailable", "Try again.")
		return
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var si, sn, di, dn, pi, pn, pc string
		rows.Scan(&si, &sn, &di, &dn, &pi, &pn, &pc)
		out = append(out, map[string]string{"schoolId": si, "school": sn, "departmentId": di, "department": dn, "programmeId": pi, "programme": pn, "code": pc})
	}
	writeJSON(w, 200, out)
}
func (a *app) programmeDetails(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var x map[string]any
	var id, number, programme, cohort, mode, state, verification string
	var admission time.Time
	err := a.db.QueryRowContext(r.Context(), `SELECT e.id,COALESCE(u.student_number,''),p.name,c.name,e.study_mode,e.state::text,e.verification_status,e.admission_date FROM enrolments e JOIN users u ON u.id=e.student_id JOIN programmes p ON p.id=e.programme_id JOIN cohorts c ON c.id=e.cohort_id WHERE e.student_id=$1 AND e.state IN('pending_verification','active','approved_leave')`, u.ID).Scan(&id, &number, &programme, &cohort, &mode, &state, &verification, &admission)
	if err != nil {
		problem(w, 404, "Programme not found", "No active enrolment is available.")
		return
	}
	x = map[string]any{"id": id, "studentNumber": number, "programme": programme, "cohort": cohort, "studyMode": mode, "state": state, "verification": verification, "admissionDate": admission}
	writeJSON(w, 200, x)
}
func (a *app) onboardingSubmit(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "student") {
		return
	}
	var in struct{ ProgrammeID, CohortID, StudyMode, AdmissionDate string }
	if !decode(w, r, &in) {
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE enrolments e SET programme_id=$2,cohort_id=$3,study_mode=$4,admission_date=$5,verification_status='submitted',version=version+1 FROM cohorts c WHERE e.student_id=$1 AND e.state='pending_verification' AND c.id=$3 AND c.programme_id=$2`, u.ID, in.ProgrammeID, in.CohortID, in.StudyMode, in.AdmissionDate)
	if err != nil {
		problem(w, 400, "Onboarding not submitted", "Programme and cohort must match.")
		return
	}
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if n != 1 {
		problem(w, 409, "Onboarding changed", "Refresh and try again.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "submitted"})
}
func (a *app) verifyEnrolment(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	if !a.canManageEnrolment(r.Context(), u, r.PathValue("id")) {
		problem(w, 403, "Access denied", "This enrolment is outside your assigned scope.")
		return
	}
	var in struct {
		Decision, Note string
		PlanStart      time.Time
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Decision != "verified" && in.Decision != "returned" {
		problem(w, 400, "Invalid decision", "Choose verified or returned.")
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	var template, plan string
	err := tx.QueryRowContext(r.Context(), `UPDATE enrolments SET verification_status=$2,verification_note=$3,verified_by=CASE WHEN $2='verified' THEN $4::uuid ELSE NULL END,verified_at=CASE WHEN $2='verified' THEN now() ELSE NULL END,state=CASE WHEN $2='verified' THEN 'active'::enrolment_state ELSE state END,plan_start_date=CASE WHEN $2='verified' THEN $5::date ELSE plan_start_date END,version=version+1 WHERE id=$1 AND verification_status='submitted' RETURNING (SELECT id FROM programme_templates WHERE programme_id=enrolments.programme_id AND study_mode=enrolments.study_mode AND state='published' ORDER BY version DESC LIMIT 1)`, r.PathValue("id"), in.Decision, in.Note, u.ID, in.PlanStart).Scan(&template)
	if err != nil {
		problem(w, 409, "Verification unavailable", "The record changed or has no published template.")
		return
	}
	if in.Decision == "verified" {
		err = tx.QueryRowContext(r.Context(), `INSERT INTO student_plans(enrolment_id,template_id,started_on,planned_completion) SELECT $1,$2,$3::date,$3::date+max(end_days) FROM milestone_definitions WHERE template_id=$2 GROUP BY template_id ON CONFLICT(enrolment_id) DO NOTHING RETURNING id`, r.PathValue("id"), template, in.PlanStart).Scan(&plan)
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO milestone_instances(plan_id,definition_id,baseline_start,baseline_due,current_start,current_due,state) SELECT $1,id,$2::date+start_days,$2::date+end_days,$2::date+start_days,$2::date+end_days,CASE WHEN position=1 THEN 'in_progress'::workflow_state ELSE 'not_started'::workflow_state END FROM milestone_definitions WHERE template_id=$3 ON CONFLICT DO NOTHING`, plan, in.PlanStart, template)
		}
	}
	if err != nil {
		problem(w, 500, "Verification failed", "The plan could not be generated.")
		return
	}
	auditTx(r.Context(), tx, u.ID, "enrolment", r.PathValue("id"), in.Decision, "Enrolment verification decision recorded")
	if in.Decision == "verified" {
		if err = generateActivities(r.Context(), tx, r.PathValue("id")); err != nil {
			problem(w, 500, "Plan activities failed", "Try again.")
			return
		}
		if err = advancePlan(r.Context(), tx, r.PathValue("id")); err != nil {
			problem(w, 500, "Plan dependencies failed", "Try again.")
			return
		}
	}
	if tx.Commit() != nil {
		problem(w, 409, "Verification changed", "Refresh and try again.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": in.Decision})
}
func (a *app) addBaseline(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	if !a.canManageEnrolment(r.Context(), u, r.PathValue("id")) {
		problem(w, 403, "Access denied", "This enrolment is outside your assigned scope.")
		return
	}
	var in struct {
		MilestoneID, Explanation, CompletedOn string
		DateKnown                             bool
	}
	if !decode(w, r, &in) || len(in.Explanation) < 8 {
		return
	}
	var belongs bool
	_ = a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM milestone_instances mi JOIN student_plans sp ON sp.id=mi.plan_id WHERE mi.id=$1 AND sp.enrolment_id=$2)`, in.MilestoneID, r.PathValue("id")).Scan(&belongs)
	if !belongs {
		problem(w, 403, "Invalid milestone", "The milestone does not belong to this enrolment.")
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	_, err := tx.ExecContext(r.Context(), `INSERT INTO baseline_completions(enrolment_id,milestone_id,completed_on,date_known,explanation,verified_by) VALUES($1,$2,NULLIF($3,'')::date,$4,$5,$6) ON CONFLICT(enrolment_id,milestone_id) DO NOTHING`, r.PathValue("id"), in.MilestoneID, in.CompletedOn, in.DateKnown, in.Explanation, u.ID)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE milestone_instances SET state='approved',approved_at=now(),version=version+1 WHERE id=$1`, in.MilestoneID)
	}
	if err != nil {
		problem(w, 400, "Baseline not saved", "Check the milestone and date.")
		return
	}
	auditTx(r.Context(), tx, u.ID, "enrolment", r.PathValue("id"), "baseline_added", "Staff-verified baseline completion recorded")
	if err = advancePlan(r.Context(), tx, r.PathValue("id")); err != nil {
		problem(w, 500, "Baseline not saved", "Could not update the next research stage.")
		return
	}
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 201, map[string]string{"status": "verified_baseline"})
}

func (a *app) supervisors(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT sp.user_id AS id,u.full_name AS name,d.name AS department,sp.description,sp.expertise_tags AS tags,sp.capacity,sp.accepting_students AS accepting,GREATEST(0,sp.capacity-(SELECT count(*) FROM supervision_assignments sa WHERE sa.supervisor_id=sp.user_id AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now()))-(SELECT count(*) FROM supervision_requests sr WHERE sr.supervisor_id=sp.user_id AND sr.state='accepted' AND sr.reservation_expires_at>now())) AS available,EXISTS(SELECT 1 FROM allocation_exceptions x JOIN enrolments e ON e.id=x.enrolment_id WHERE e.student_id=$1 AND x.supervisor_id=sp.user_id AND x.kind='capacity' AND x.state='approved') AS capacity_exception FROM supervisor_profiles sp JOIN users u ON u.id=sp.user_id JOIN departments d ON d.id=sp.home_department_id WHERE u.active AND EXISTS(SELECT 1 FROM enrolments e WHERE e.student_id=$1 AND (EXISTS(SELECT 1 FROM supervisor_programme_eligibility pe WHERE pe.programme_id=e.programme_id AND pe.supervisor_id=sp.user_id AND pe.active) OR EXISTS(SELECT 1 FROM allocation_exceptions x WHERE x.enrolment_id=e.id AND x.supervisor_id=sp.user_id AND x.kind='eligibility' AND x.state='approved'))) ORDER BY u.full_name`, current(r).ID)
}
func (a *app) supervision(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT sr.id,usr.full_name,sr.position,sr.state,sr.requested_at,sr.reservation_expires_at FROM supervision_requests sr JOIN enrolments e ON e.id=sr.enrolment_id JOIN users usr ON usr.id=sr.supervisor_id WHERE e.student_id=$1 ORDER BY sr.requested_at DESC`, u.ID)
	if err != nil {
		problem(w, 500, "Supervision unavailable", "Try again.")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, n, p, s string
		var req time.Time
		var exp sql.NullTime
		rows.Scan(&id, &n, &p, &s, &req, &exp)
		out = append(out, map[string]any{"id": id, "supervisor": n, "position": p, "state": s, "requestedAt": req, "reservationExpires": exp})
	}
	writeJSON(w, 200, out)
}
func (a *app) requestSupervisor(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "student") {
		return
	}
	var in struct{ SupervisorID, Position, Note string }
	if !decode(w, r, &in) {
		return
	}
	var id string
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO supervision_requests(enrolment_id,supervisor_id,position,student_note) SELECT e.id,$2,$3,$4 FROM enrolments e JOIN supervisor_profiles sp ON sp.user_id=$2 AND sp.accepting_students WHERE e.student_id=$1 AND e.state='active' AND (EXISTS(SELECT 1 FROM supervisor_programme_eligibility pe WHERE pe.programme_id=e.programme_id AND pe.supervisor_id=$2 AND pe.active) OR EXISTS(SELECT 1 FROM allocation_exceptions x WHERE x.enrolment_id=e.id AND x.supervisor_id=$2 AND x.kind='eligibility' AND x.state='approved')) AND NOT EXISTS(SELECT 1 FROM supervision_requests x WHERE x.enrolment_id=e.id AND x.state IN('pending','accepted') AND (x.position=$3 OR x.supervisor_id=$2)) AND NOT EXISTS(SELECT 1 FROM supervision_assignments sa WHERE sa.enrolment_id=e.id AND sa.supervisor_id=$2 AND (sa.effective_to IS NULL OR sa.effective_to>now())) RETURNING id`, u.ID, in.SupervisorID, in.Position, in.Note).Scan(&id)
	if err != nil {
		problem(w, 409, "Request not created", "Check eligibility and existing requests.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "pending"})
}
func (a *app) respondSupervision(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "supervisor") {
		return
	}
	var in struct{ Decision, Reason string }
	if !decode(w, r, &in) {
		return
	}
	if in.Decision != "accepted" && in.Decision != "declined" {
		problem(w, 400, "Invalid decision", "Choose accepted or declined.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		problem(w, 503, "Response unavailable", "Try again.")
		return
	}
	defer tx.Rollback()
	var capacity, used int
	err = tx.QueryRowContext(r.Context(), `SELECT capacity FROM supervisor_profiles WHERE user_id=$1 FOR UPDATE`, u.ID).Scan(&capacity)
	if err == nil && in.Decision == "accepted" {
		err = tx.QueryRowContext(r.Context(), `SELECT (SELECT count(*) FROM supervision_assignments WHERE supervisor_id=$1 AND (effective_to IS NULL OR effective_to>now()))+(SELECT count(*) FROM supervision_requests WHERE supervisor_id=$1 AND state='accepted' AND reservation_expires_at>now())`, u.ID).Scan(&used)
		var eid string
		if err == nil {
			err = tx.QueryRowContext(r.Context(), `SELECT enrolment_id FROM supervision_requests WHERE id=$1 AND supervisor_id=$2`, r.PathValue("id"), u.ID).Scan(&eid)
		}
		if err != nil || (used >= capacity && !capacityException(r, tx, eid, u.ID)) {
			problem(w, 409, "Capacity full", "An accepted reservation also uses capacity. Release a reservation or ask scoped academic administration to approve a capacity exception.")
			return
		}
	}
	if err != nil {
		problem(w, 409, "Profile unavailable", "Your supervisor profile must be active.")
		return
	}
	res, err := tx.ExecContext(r.Context(), `UPDATE supervision_requests SET state=$3,response_reason=$4,responded_at=now(),reservation_expires_at=CASE WHEN $3='accepted' THEN now()+interval '7 days' END WHERE id=$1 AND supervisor_id=$2 AND state IN('pending','expired')`, r.PathValue("id"), u.ID, in.Decision, in.Reason)
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if err != nil || n != 1 {
		problem(w, 409, "Response not recorded", "The request is no longer awaiting your response.")
		return
	}
	if tx.Commit() != nil {
		problem(w, 409, "Capacity changed", "Refresh and try again.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": in.Decision})
}
func (a *app) allocateSupervisor(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	eid, lookupErr := a.enrolmentFor(r.Context(), "supervision_requests", r.PathValue("id"))
	if lookupErr != nil || !a.canManageEnrolment(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "This allocation is outside your assigned scope.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return
	}
	defer tx.Rollback()
	var enrolment, supervisor, position string
	var capacity, active int
	err = tx.QueryRowContext(r.Context(), `SELECT sr.enrolment_id,sr.supervisor_id,sr.position,sp.capacity,(SELECT count(*) FROM supervision_assignments sa WHERE sa.supervisor_id=sr.supervisor_id AND (sa.effective_to IS NULL OR sa.effective_to>now())) FROM supervision_requests sr JOIN supervisor_profiles sp ON sp.user_id=sr.supervisor_id WHERE sr.id=$1 AND sr.state='accepted' AND sr.reservation_expires_at>now() FOR UPDATE`, r.PathValue("id")).Scan(&enrolment, &supervisor, &position, &capacity, &active)
	if err != nil || (active >= capacity && !capacityException(r, tx, enrolment, supervisor)) {
		problem(w, 409, "Allocation unavailable", "The reservation expired or supervisor capacity is full.")
		return
	}
	if allowed, e := allocationAllowed(r, tx, enrolment, supervisor); e != nil || !allowed {
		problem(w, 409, "Eligibility changed", "Supervisor eligibility must be current or covered by an approved exception.")
		return
	}
	var id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO supervision_assignments(enrolment_id,supervisor_id,position,effective_from,request_id,created_by) VALUES($1,$2,$3,now(),$4,$5) RETURNING id`, enrolment, supervisor, position, r.PathValue("id"), u.ID).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE supervision_requests SET state='confirmed' WHERE id=$1`, r.PathValue("id"))
	}
	if err != nil {
		problem(w, 409, "Allocation conflict", "Another allocation changed capacity. Refresh and try again.")
		return
	}
	auditTx(r.Context(), tx, u.ID, "supervision_assignment", id, "confirmed", "Supervisor allocation confirmed")
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "confirmed"})
}
func (a *app) replaceSupervisor(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	var in struct {
		EnrolmentID, OldAssignmentID, AcceptedRequestID, Reason string
		EffectiveAt                                             time.Time
	}
	if !decode(w, r, &in) {
		return
	}
	if !a.canManageEnrolment(r.Context(), u, in.EnrolmentID) {
		problem(w, 403, "Access denied", "This enrolment is outside your assigned scope.")
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	defer tx.Rollback()
	var supervisor, position string
	err := tx.QueryRowContext(r.Context(), `SELECT supervisor_id,position FROM supervision_requests WHERE id=$1 AND enrolment_id=$2 AND state='accepted' AND reservation_expires_at>now() FOR UPDATE`, in.AcceptedRequestID, in.EnrolmentID).Scan(&supervisor, &position)
	if len(strings.TrimSpace(in.Reason)) < 8 || in.EffectiveAt.IsZero() {
		problem(w, 400, "Replacement details required", "Provide an effective date and reason.")
		return
	}
	var capacity, used int
	if err == nil {
		err = tx.QueryRowContext(r.Context(), `SELECT capacity FROM supervisor_profiles WHERE user_id=$1 FOR UPDATE`, supervisor).Scan(&capacity)
	}
	if err == nil {
		err = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM supervision_assignments WHERE supervisor_id=$1 AND (effective_to IS NULL OR effective_to>$2)`, supervisor, in.EffectiveAt).Scan(&used)
	}
	if err != nil || (used >= capacity && !capacityException(r, tx, in.EnrolmentID, supervisor)) {
		problem(w, 409, "Replacement unavailable", "Check the accepted request and supervisor capacity.")
		return
	}
	if allowed, e := allocationAllowed(r, tx, in.EnrolmentID, supervisor); e != nil || !allowed {
		problem(w, 409, "Eligibility changed", "Supervisor eligibility must be current or covered by an approved exception.")
		return
	}
	if err == nil {
		var oldID string
		err = tx.QueryRowContext(r.Context(), `UPDATE supervision_assignments SET effective_to=$3,replacement_reason=$4 WHERE id=$1 AND enrolment_id=$2 AND effective_to IS NULL AND position=$5 AND supervisor_id<>$6 AND effective_from<$3 RETURNING id`, in.OldAssignmentID, in.EnrolmentID, in.EffectiveAt, in.Reason, position, supervisor).Scan(&oldID)
	}
	var id string
	if err == nil {
		err = tx.QueryRowContext(r.Context(), `INSERT INTO supervision_assignments(enrolment_id,supervisor_id,position,effective_from,request_id,replacement_reason,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, in.EnrolmentID, supervisor, position, in.EffectiveAt, in.AcceptedRequestID, in.Reason, u.ID).Scan(&id)
	}
	if err != nil {
		problem(w, 409, "Replacement not completed", "The accepted request or current assignment changed.")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE supervision_requests SET state='confirmed' WHERE id=$1`, in.AcceptedRequestID); err != nil {
		problem(w, 409, "Replacement changed", "Refresh and try again.")
		return
	}
	auditTx(r.Context(), tx, u.ID, "supervision_assignment", id, "replacement_scheduled", "Supervisor replacement scheduled with preserved history")
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "scheduled"})
}

func (a *app) topics(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !hasRole(u, "student") {
		rows, err := a.db.QueryContext(r.Context(), `SELECT t.id,t.title,t.description,t.tags,usr.full_name,t.capacity-(SELECT count(*) FROM topic_applications ta WHERE ta.topic_id=t.id AND ta.state='accepted'),t.closing_date,t.state FROM topics t JOIN users usr ON usr.id=t.supervisor_id WHERE t.supervisor_id=$1 OR EXISTS(SELECT 1 FROM role_assignments ra JOIN topic_programmes tp ON tp.topic_id=t.id JOIN programmes p ON p.id=tp.programme_id WHERE ra.user_id=$1 AND ((ra.scope_type='programme' AND ra.scope_id=p.id) OR (ra.scope_type='department' AND ra.scope_id=p.department_id) OR (ra.scope_type='school' AND EXISTS(SELECT 1 FROM departments d WHERE d.id=p.department_id AND d.school_id=ra.scope_id)) OR ra.scope_type='institution')) ORDER BY t.closing_date`, u.ID)
		if err != nil {
			problem(w, 500, "Topics unavailable", "Try again.")
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, title, desc, owner, state string
			var tags []string
			var avail int
			var close time.Time
			rows.Scan(&id, &title, &desc, &tags, &owner, &avail, &close, &state)
			out = append(out, map[string]any{"id": id, "title": title, "description": desc, "tags": tags, "supervisor": owner, "available": avail, "closingDate": close, "state": state})
		}
		writeJSON(w, 200, out)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT t.id,t.title,t.description,t.tags,usr.full_name,t.capacity-taken.n,t.closing_date FROM enrolments e JOIN topic_programmes tp ON tp.programme_id=e.programme_id JOIN topics t ON t.id=tp.topic_id AND t.state='published' JOIN users usr ON usr.id=t.supervisor_id CROSS JOIN LATERAL(SELECT count(*)::int n FROM topic_applications ta WHERE ta.topic_id=t.id AND ta.state='accepted') taken WHERE e.student_id=$1 AND t.closing_date>=current_date ORDER BY t.closing_date`, u.ID)
	if err != nil {
		problem(w, 500, "Topics unavailable", "Try again.")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, title, desc, owner string
		var tags []string
		var avail int
		var close time.Time
		rows.Scan(&id, &title, &desc, &tags, &owner, &avail, &close)
		out = append(out, map[string]any{"id": id, "title": title, "description": desc, "tags": tags, "supervisor": owner, "available": avail, "closingDate": close})
	}
	writeJSON(w, 200, out)
}
func (a *app) createTopic(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "supervisor") {
		return
	}
	var in struct {
		Title, Description, ClosingDate string
		Capacity                        int
		ProgrammeIDs, Tags              []string
	}
	if !decode(w, r, &in) {
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	var id string
	err := tx.QueryRowContext(r.Context(), `INSERT INTO topics(supervisor_id,title,description,tags,capacity,closing_date) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, u.ID, in.Title, in.Description, in.Tags, in.Capacity, in.ClosingDate).Scan(&id)
	for _, p := range in.ProgrammeIDs {
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO topic_programmes(topic_id,programme_id) VALUES($1,$2)`, id, p)
		}
	}
	if err != nil {
		problem(w, 400, "Topic not saved", "Check capacity, date and programmes.")
		return
	}
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "draft"})
}
func (a *app) applyTopic(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Statement string }
	if !decode(w, r, &in) {
		return
	}
	var id string
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO topic_applications(topic_id,enrolment_id,statement) SELECT $2,e.id,$3 FROM enrolments e JOIN topic_programmes tp ON tp.programme_id=e.programme_id AND tp.topic_id=$2 JOIN topics t ON t.id=$2 AND t.state='published' AND t.closing_date>=current_date WHERE e.student_id=$1 AND (SELECT count(*) FROM topic_applications x WHERE x.enrolment_id=e.id AND x.state IN('submitted','under_review','offered'))<3 RETURNING id`, u.ID, r.PathValue("id"), in.Statement).Scan(&id)
	if err != nil {
		problem(w, 409, "Application not created", "The topic is unavailable or you already have three active applications.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "submitted"})
}
func (a *app) confirmTopic(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	tx, _ := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	defer tx.Rollback()
	var enrolment, topic string
	var cap, taken int
	err := tx.QueryRowContext(r.Context(), `SELECT ta.enrolment_id,ta.topic_id,t.capacity,(SELECT count(*) FROM topic_applications WHERE topic_id=ta.topic_id AND state='accepted') FROM topic_applications ta JOIN enrolments e ON e.id=ta.enrolment_id JOIN topics t ON t.id=ta.topic_id WHERE ta.id=$1 AND e.student_id=$2 AND ta.state='offered' FOR UPDATE`, r.PathValue("id"), u.ID).Scan(&enrolment, &topic, &cap, &taken)
	if err != nil || taken >= cap {
		problem(w, 409, "Offer unavailable", "The offer changed or topic capacity is full.")
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE topic_applications SET state=CASE WHEN id=$1 THEN 'accepted' ELSE 'withdrawn' END,decided_at=now() WHERE enrolment_id=$2 AND state IN('submitted','under_review','offered')`, r.PathValue("id"), enrolment)
	if err != nil {
		problem(w, 409, "Topic selection conflict", "Refresh and try again.")
		return
	}
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "accepted"})
}

func (a *app) reviews(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT ra.id,sv.version,sv.original_name,ra.due_at,ra.state,st.full_name,md.title FROM review_assignments ra JOIN submission_versions sv ON sv.id=ra.submission_version_id JOIN submissions s ON s.id=sv.submission_id JOIN users st ON st.id=s.student_id JOIN milestone_instances mi ON mi.id=s.milestone_id JOIN milestone_definitions md ON md.id=mi.definition_id WHERE ra.reviewer_id=$1 ORDER BY ra.due_at`, u.ID)
	if err != nil {
		problem(w, 500, "Reviews unavailable", "Try again.")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, file, state, student, milestone string
		var ver int
		var due time.Time
		rows.Scan(&id, &ver, &file, &due, &state, &student, &milestone)
		out = append(out, map[string]any{"id": id, "version": ver, "file": file, "due": due, "state": state, "student": student, "milestone": milestone})
	}
	writeJSON(w, 200, out)
}
func (a *app) reviewDecision(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Decision, Feedback string }
	if !decode(w, r, &in) {
		return
	}
	if in.Decision != "approved" && in.Decision != "changes_requested" {
		problem(w, 400, "Invalid decision", "Choose approve or request changes.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 500, "Unavailable", "Try again.")
		return
	}
	defer tx.Rollback()
	var vid, sid, mid, eid, mode string
	var required, verified bool
	err = tx.QueryRowContext(r.Context(), `SELECT sv.id,s.id,mi.id,sp.enrolment_id,s.approval_mode,md.similarity_required,EXISTS(SELECT 1 FROM similarity_evidence se WHERE se.submission_version_id=sv.id AND se.verified_at IS NOT NULL) FROM review_assignments ra JOIN submission_versions sv ON sv.id=ra.submission_version_id JOIN submissions s ON s.id=sv.submission_id JOIN milestone_instances mi ON mi.id=s.milestone_id JOIN milestone_definitions md ON md.id=mi.definition_id JOIN student_plans sp ON sp.id=mi.plan_id WHERE ra.id=$1 AND ra.reviewer_id=$2 AND ra.state='awaiting_review' AND NOT ra.round_closed AND s.state='submitted' AND sv.version=s.current_version AND EXISTS(SELECT 1 FROM supervision_assignments sa WHERE sa.enrolment_id=sp.enrolment_id AND sa.supervisor_id=$2 AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now())) FOR UPDATE OF s,ra`, r.PathValue("id"), u.ID).Scan(&vid, &sid, &mid, &eid, &mode, &required, &verified)
	if err != nil {
		problem(w, 409, "Review unavailable", "This round is closed, the version changed, or you are no longer assigned.")
		return
	}
	if in.Decision == "approved" && required && !verified {
		problem(w, 409, "Similarity evidence required", "Verify the report for this exact version before approval.")
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE review_assignments SET state=$2,feedback=$3,decided_at=now() WHERE id=$1`, r.PathValue("id"), in.Decision, in.Feedback)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO review_decisions(review_assignment_id,decision,feedback) VALUES($1,$2,$3)`, r.PathValue("id"), in.Decision, in.Feedback)
	}
	if err == nil && in.Decision == "changes_requested" {
		_, err = tx.ExecContext(r.Context(), `UPDATE submissions SET state='changes_requested' WHERE id=$1`, sid)
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE milestone_instances SET state='changes_requested' WHERE id=$1`, mid)
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE review_assignments SET round_closed=true WHERE submission_version_id=$1`, vid)
		}
	}
	if err == nil && in.Decision == "approved" {
		var all bool
		err = tx.QueryRowContext(r.Context(), `SELECT NOT EXISTS(SELECT 1 FROM review_assignments WHERE submission_version_id=$1 AND state<>'approved')`, vid).Scan(&all)
		if err == nil && (mode != "both" || all) {
			_, err = tx.ExecContext(r.Context(), `UPDATE submissions SET state='approved' WHERE id=$1`, sid)
			if err == nil {
				_, err = tx.ExecContext(r.Context(), `UPDATE review_assignments SET round_closed=true WHERE submission_version_id=$1`, vid)
			}
			if err == nil {
				_, err = tx.ExecContext(r.Context(), `UPDATE activity_instances ai SET state='approved',completed_at=now() FROM activity_definitions ad WHERE ai.definition_id=ad.id AND ai.milestone_id=$1 AND ad.activity_type='submission'`, mid)
			}
			if err == nil {
				_, err = tx.ExecContext(r.Context(), `UPDATE milestone_instances mi SET state='approved',approved_at=now() WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM activity_instances ai JOIN activity_definitions ad ON ad.id=ai.definition_id WHERE ai.milestone_id=mi.id AND ad.required AND ai.state<>'approved')`, mid)
			}
			if err == nil {
				err = advancePlan(r.Context(), tx, eid)
			}
		}
	}
	if err == nil {
		err = auditTx(r.Context(), tx, u.ID, "submission_version", vid, in.Decision, "Academic review decision recorded")
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO notifications(recipient_id,dedupe_key,title,urgency) SELECT student_id,'review:'||$2::text,'Your submission has new academic feedback','info' FROM enrolments WHERE id=$1 ON CONFLICT DO NOTHING`, eid, r.PathValue("id"))
	}
	commitResponse(w, tx, err, map[string]string{"status": in.Decision})
}

func (a *app) meetings(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT m.id,m.title,m.purpose,m.scheduled_at,m.duration_minutes,m.state,COALESCE(m.location,m.meeting_link,'Not set') FROM meetings m JOIN enrolments e ON e.id=m.enrolment_id WHERE e.student_id=$1 OR m.organizer_id=$1 OR EXISTS(SELECT 1 FROM meeting_participants mp WHERE mp.meeting_id=m.id AND mp.user_id=$1) ORDER BY m.scheduled_at DESC`, u.ID)
	if err != nil {
		problem(w, 500, "Meetings unavailable", "Try again.")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, t, p, s, l string
		var at time.Time
		var dur int
		rows.Scan(&id, &t, &p, &at, &dur, &s, &l)
		out = append(out, map[string]any{"id": id, "title": t, "purpose": p, "scheduledAt": at, "duration": dur, "state": s, "location": l})
	}
	writeJSON(w, 200, out)
}
func (a *app) createMeeting(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct {
		EnrolmentID, MilestoneID, Title, Purpose, Location, Link string
		ScheduledAt                                              time.Time
		Duration                                                 int
		Participants                                             []string
	}
	if !decode(w, r, &in) {
		return
	}
	if !a.academicAccess(r.Context(), u, in.EnrolmentID) {
		problem(w, 403, "Access denied", "This student is outside your assignments.")
		return
	}
	for _, id := range in.Participants {
		var allowed bool
		_ = a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM enrolments e WHERE e.id=$1 AND (e.student_id=$2 OR EXISTS(SELECT 1 FROM supervision_assignments sa WHERE sa.enrolment_id=e.id AND sa.supervisor_id=$2 AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now()))))`, in.EnrolmentID, id).Scan(&allowed)
		if !allowed {
			problem(w, 400, "Invalid participant", "Choose the student or an active assigned supervisor.")
			return
		}
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	var id string
	err := tx.QueryRowContext(r.Context(), `INSERT INTO meetings(enrolment_id,milestone_id,title,purpose,scheduled_at,duration_minutes,location,meeting_link,organizer_id) VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, in.EnrolmentID, in.MilestoneID, in.Title, in.Purpose, in.ScheduledAt, in.Duration, in.Location, in.Link, u.ID).Scan(&id)
	for _, p := range append(in.Participants, u.ID) {
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO meeting_participants(meeting_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, p)
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO meeting_participants(meeting_id,user_id) SELECT $1::uuid,student_id FROM enrolments WHERE id=$2 UNION SELECT $1::uuid,supervisor_id FROM supervision_assignments WHERE enrolment_id=$2 AND effective_from<=now() AND (effective_to IS NULL OR effective_to>now()) ON CONFLICT DO NOTHING`, id, in.EnrolmentID)
	}
	if err != nil {
		problem(w, 400, "Meeting not proposed", "Check the date, duration and participants.")
		return
	}
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "proposed"})
}
func (a *app) recordMeeting(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "supervisor") {
		return
	}
	var in struct {
		Held     bool
		ActualAt time.Time
		Notes    string
		Attended []string
		Actions  []struct {
			OwnerID, Title string
			DueAt          time.Time
		}
	}
	if !decode(w, r, &in) {
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	state := "not_held"
	if in.Held {
		state = "held"
	}
	var enrolment string
	err := tx.QueryRowContext(r.Context(), `UPDATE meetings SET state=$3,actual_at=$4,recorded_at=now(),version=version+1 WHERE id=$1 AND state='confirmed' AND EXISTS(SELECT 1 FROM supervision_assignments sa WHERE sa.enrolment_id=meetings.enrolment_id AND sa.supervisor_id=$2 AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now())) RETURNING enrolment_id`, r.PathValue("id"), u.ID, state, in.ActualAt).Scan(&enrolment)
	if err == nil && in.Notes != "" {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO meeting_notes(meeting_id,author_id,body,state) VALUES($1,$2,$3,'confirmed')`, r.PathValue("id"), u.ID, in.Notes)
	}
	for _, x := range in.Actions {
		if err == nil {
			var task string
			err = tx.QueryRowContext(r.Context(), `INSERT INTO action_tasks(enrolment_id,owner_id,title,task_type,due_at) SELECT $1,$2,$3,'meeting_action',$4 WHERE EXISTS(SELECT 1 FROM meeting_participants WHERE meeting_id=$5 AND user_id=$2) RETURNING id`, enrolment, x.OwnerID, x.Title, x.DueAt, r.PathValue("id")).Scan(&task)
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE meeting_participants SET attended=(user_id::text=ANY($2::text[])) WHERE meeting_id=$1`, r.PathValue("id"), in.Attended)
	}
	if err != nil {
		problem(w, 409, "Meeting not recorded", "The record changed or you are not the organizer.")
		return
	}
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": state})
}

func (a *app) transitionCase(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Status, Resolution string }
	if !decode(w, r, &in) {
		return
	}
	if !requireAny(w, u, "coordinator", "support", "hod", "dean", "leadership") {
		return
	}
	eid, lookupErr := a.enrolmentFor(r.Context(), "support_cases", r.PathValue("id"))
	var assigned, restricted bool
	_ = a.db.QueryRowContext(r.Context(), `SELECT sc.restricted,(sc.owner_id=$2 OR EXISTS(SELECT 1 FROM case_access_grants g WHERE g.case_id=sc.id AND g.user_id=$2)) FROM support_cases sc WHERE sc.id=$1`, r.PathValue("id"), u.ID).Scan(&restricted, &assigned)
	if lookupErr != nil || (restricted && !assigned) || (!restricted && !assigned && !a.canManageEnrolment(r.Context(), u, eid)) {
		problem(w, 403, "Access denied", "This case is outside your assignment or scope.")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE support_cases SET status=$2::case_status,resolution_summary=CASE WHEN $2='resolved' THEN $3 ELSE resolution_summary END,resolved_at=CASE WHEN $2='resolved' THEN now() END WHERE id=$1 AND ($2<>'resolved' OR length($3)>=8)`, r.PathValue("id"), in.Status, in.Resolution)
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if err != nil || n != 1 {
		problem(w, 400, "Case not updated", "Resolution requires a meaningful summary.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": in.Status})
}
func (a *app) requestLeave(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ StartDate, EndDate, Category, Explanation, ReturnDate string }
	if !decode(w, r, &in) {
		return
	}
	var id string
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO leave_requests(enrolment_id,start_date,end_date,reason_category,restricted_explanation,proposed_return_date) SELECT id,$2,$3,$4,$5,$6 FROM enrolments WHERE student_id=$1 AND state='active' RETURNING id`, u.ID, in.StartDate, in.EndDate, in.Category, in.Explanation, in.ReturnDate).Scan(&id)
	if err != nil {
		problem(w, 400, "Leave request not saved", "Check the dates.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "pending"})
}
func (a *app) decideLeave(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	eid, lookupErr := a.enrolmentFor(r.Context(), "leave_requests", r.PathValue("id"))
	if lookupErr != nil || !a.canManageEnrolment(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "This leave request is outside your assigned scope.")
		return
	}
	var in struct{ Decision, Reason string }
	if !decode(w, r, &in) {
		return
	}
	canApprove := hasRole(u, "hod") || hasRole(u, "dean") || hasRole(u, "leadership")
	if len(strings.TrimSpace(in.Reason)) < 8 || (!canApprove && in.Decision != "coordinator_reviewed") || (in.Decision != "approved" && in.Decision != "declined" && in.Decision != "coordinator_reviewed") {
		problem(w, 400, "Invalid leave decision", "Coordinators record a review; a HOD, dean or graduate-school leader approves or declines with a reason.")
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	var enrolment string
	var start, end time.Time
	err := tx.QueryRowContext(r.Context(), `UPDATE leave_requests SET state=$2,decision_reason=$3,decided_by=$4,decided_at=now() WHERE id=$1 AND state IN('pending','coordinator_reviewed') RETURNING enrolment_id,start_date,end_date`, r.PathValue("id"), in.Decision, in.Reason, u.ID).Scan(&enrolment, &start, &end)
	if err == nil && in.Decision == "approved" {
		days := int(end.Sub(start).Hours()/24) + 1
		var revision string
		err = tx.QueryRowContext(r.Context(), `INSERT INTO plan_revisions(plan_id,source_type,source_id,reason,approved_by) SELECT id,'leave',$2,$3,$4 FROM student_plans WHERE enrolment_id=$1 RETURNING id`, enrolment, r.PathValue("id"), in.Reason, u.ID).Scan(&revision)
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO schedule_revision_items(revision_id,milestone_id,old_start,old_due,new_start,new_due) SELECT $1,mi.id,mi.current_start,mi.current_due,mi.current_start+$3::integer,mi.current_due+$3::integer FROM milestone_instances mi JOIN student_plans sp ON sp.id=mi.plan_id WHERE sp.enrolment_id=$2 AND mi.state<>'approved' AND mi.current_due>=$4`, revision, enrolment, days, start)
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE enrolments SET state='approved_leave' WHERE id=$1`, enrolment)
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE milestone_instances mi SET current_start=current_start+$2::integer,current_due=current_due+$2::integer,version=version+1 FROM student_plans sp WHERE mi.plan_id=sp.id AND sp.enrolment_id=$1 AND mi.state NOT IN('approved') AND mi.current_due>=$3`, enrolment, days, start)
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE activity_instances ai SET current_due=ai.current_due+(ri.new_due-ri.old_due)*interval '1 day',version=ai.version+1 FROM schedule_revision_items ri WHERE ri.revision_id=$1 AND ri.milestone_id=ai.milestone_id AND ai.state<>'approved'`, revision)
		}

		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE notifications SET dismissed_at=now() WHERE recipient_id=(SELECT student_id FROM enrolments WHERE id=$1) AND dedupe_key LIKE 'deadline:%'`, enrolment)
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE action_tasks SET due_at=due_at+$2*interval '1 day' WHERE enrolment_id=$1 AND state='open' AND owner_id=(SELECT student_id FROM enrolments WHERE id=$1)`, enrolment, days)
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO action_tasks(enrolment_id,owner_id,title,task_type,due_at) VALUES($1,$2,'Review return from approved leave','leave_return',$3)`, enrolment, u.ID, end.AddDate(0, 0, 1))
		}
	}
	if err != nil {
		problem(w, 409, "Leave decision failed", "The request changed or dates could not be revised.")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE student_plans sp SET planned_completion=(SELECT max(current_due) FROM milestone_instances WHERE plan_id=sp.id) WHERE enrolment_id=$1`, enrolment); err != nil {
		problem(w, 409, "Schedule unavailable", "Refresh and try again.")
		return
	}
	if err = auditTx(r.Context(), tx, u.ID, "enrolment", enrolment, "leave_"+in.Decision, "Leave decision recorded; original dates retained"); err != nil {
		problem(w, 409, "Audit unavailable", "Try again.")
		return
	}
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": in.Decision})
}
func (a *app) requestExtension(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ MilestoneID, RequestedDate, Reason string }
	if !decode(w, r, &in) {
		return
	}
	var id string
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO extension_requests(enrolment_id,milestone_id,requested_by,old_date,requested_date,reason) SELECT e.id,mi.id,$1,mi.current_due,$3,$4 FROM milestone_instances mi JOIN student_plans sp ON sp.id=mi.plan_id JOIN enrolments e ON e.id=sp.enrolment_id WHERE mi.id=$2 AND mi.state<>'approved' AND e.state='active' AND NOT EXISTS(SELECT 1 FROM extension_requests prior WHERE prior.milestone_id=mi.id AND prior.state IN('pending','escalated')) AND (e.student_id=$1 OR EXISTS(SELECT 1 FROM supervision_assignments sa WHERE sa.enrolment_id=e.id AND sa.supervisor_id=$1 AND sa.effective_to IS NULL)) RETURNING id`, u.ID, in.MilestoneID, in.RequestedDate, in.Reason).Scan(&id)
	if err != nil {
		problem(w, 400, "Extension not requested", "The new date must be later than the current date.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "pending"})
}
func (a *app) decideExtension(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	eid, lookupErr := a.enrolmentFor(r.Context(), "extension_requests", r.PathValue("id"))
	if lookupErr != nil || !a.canManageEnrolment(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "This extension is outside your assigned scope.")
		return
	}
	var in struct{ Decision, Reason string }
	if !decode(w, r, &in) {
		return
	}
	if (in.Decision != "approved" && in.Decision != "declined") || len(strings.TrimSpace(in.Reason)) < 8 {
		problem(w, 400, "Invalid decision", "Approve or decline with a reason of at least 8 characters.")
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	var mid, enrolment string
	var old, new time.Time
	err := tx.QueryRowContext(r.Context(), `UPDATE extension_requests SET state=$2,decision_reason=$3,decided_by=$4,decided_at=now() WHERE id=$1 AND state IN('pending','escalated') AND ($2='declined' OR EXISTS(SELECT 1 FROM milestone_instances mi WHERE mi.id=extension_requests.milestone_id AND mi.state<>'approved' AND mi.current_due=extension_requests.old_date)) RETURNING milestone_id,enrolment_id,old_date,requested_date`, r.PathValue("id"), in.Decision, in.Reason, u.ID).Scan(&mid, &enrolment, &old, &new)
	if err == nil && in.Decision == "approved" {
		var revision string
		err = tx.QueryRowContext(r.Context(), `INSERT INTO plan_revisions(plan_id,source_type,source_id,reason,approved_by) SELECT plan_id,'extension',$2,$3,$4 FROM milestone_instances WHERE id=$1 RETURNING id`, mid, r.PathValue("id"), in.Reason, u.ID).Scan(&revision)
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO schedule_revision_items(revision_id,milestone_id,old_start,old_due,new_start,new_due) SELECT $1,id,current_start,current_due,current_start,$3 FROM milestone_instances WHERE id=$2`, revision, mid, new)
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE milestone_instances SET current_due=$2,version=version+1 WHERE id=$1`, mid, new)
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE action_tasks SET due_at=due_at+($2::date-$3::date)*interval '1 day' WHERE milestone_id=$1 AND state='open' AND owner_id=(SELECT student_id FROM enrolments WHERE id=$4)`, mid, new, old, enrolment)
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE activity_instances SET current_due=current_due+($2::date-$3::date)*interval '1 day',version=version+1 WHERE milestone_id=$1 AND state<>'approved'`, mid, new, old)
		}

		if err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE notifications SET dismissed_at=now() WHERE dedupe_key IN(SELECT 'deadline:'||id FROM action_tasks WHERE milestone_id=$1)`, mid)
		}
	}
	if err != nil {
		problem(w, 409, "Extension decision failed", "The request changed.")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE student_plans sp SET planned_completion=(SELECT max(current_due) FROM milestone_instances WHERE plan_id=sp.id) WHERE enrolment_id=$1`, enrolment); err != nil {
		problem(w, 409, "Schedule unavailable", "Refresh and try again.")
		return
	}
	if err = auditTx(r.Context(), tx, u.ID, "enrolment", enrolment, "extension_"+in.Decision, "Extension decision recorded; original dates retained"); err != nil {
		problem(w, 409, "Audit unavailable", "Try again.")
		return
	}
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": in.Decision})
}

func (a *app) examination(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT de.id,st.full_name,de.starts_at,de.state,c.state FROM defence_events de JOIN committees c ON c.id=de.committee_id JOIN enrolments e ON e.id=c.enrolment_id JOIN users st ON st.id=e.student_id LEFT JOIN committee_members cm ON cm.committee_id=c.id WHERE e.student_id=$1 OR cm.user_id=$1 OR de.created_by=$1 GROUP BY de.id,st.full_name,c.state ORDER BY de.starts_at NULLS LAST`, u.ID)
	if err != nil {
		problem(w, 500, "Examination unavailable", "Try again.")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, student, state, committee string
		var start sql.NullTime
		rows.Scan(&id, &student, &start, &state, &committee)
		out = append(out, map[string]any{"id": id, "student": student, "startsAt": start, "state": state, "committeeState": committee})
	}
	writeJSON(w, 200, out)
}
func (a *app) createCommittee(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	var in struct {
		EnrolmentID string
		Members     []struct{ UserID, Role string }
	}
	if !decode(w, r, &in) {
		return
	}
	if !a.canManageEnrolment(r.Context(), u, in.EnrolmentID) {
		problem(w, 403, "Access denied", "This enrolment is outside your assigned scope.")
		return
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	var id string
	err := tx.QueryRowContext(r.Context(), `INSERT INTO committees(enrolment_id,created_by,state) VALUES($1,$2,'awaiting_acceptance') RETURNING id`, in.EnrolmentID, u.ID).Scan(&id)
	for _, m := range in.Members {
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO committee_members(committee_id,user_id,member_role) VALUES($1,$2,$3)`, id, m.UserID, m.Role)
		}
	}
	if err != nil {
		problem(w, 400, "Committee not created", "Required roles must be distinct.")
		return
	}
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "awaiting_acceptance"})
}
func (a *app) declareConflict(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Response, Conflict, Note string }
	if !decode(w, r, &in) {
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE committee_members SET response=$3,conflict_state=$4,conflict_note=$5 WHERE id=$1 AND user_id=$2`, r.PathValue("id"), u.ID, in.Response, in.Conflict, in.Note)
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if err != nil || n != 1 {
		problem(w, 403, "Declaration not recorded", "This committee assignment is not yours.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": in.Conflict})
}
func (a *app) addAvailability(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ StartsAt, EndsAt time.Time }
	if !decode(w, r, &in) {
		return
	}
	var id string
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO availability_windows(committee_member_id,starts_at,ends_at) SELECT id,$3,$4 FROM committee_members WHERE id=$1 AND user_id=$2 RETURNING id`, r.PathValue("id"), u.ID, in.StartsAt, in.EndsAt).Scan(&id)
	if err != nil {
		problem(w, 400, "Availability not saved", "End time must be after start time.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}
func (a *app) resolveConflict(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	var in struct{ Resolution string }
	if !decode(w, r, &in) || len(in.Resolution) < 8 {
		return
	}
	var eid string
	_ = a.db.QueryRowContext(r.Context(), `SELECT c.enrolment_id FROM committee_members cm JOIN committees c ON c.id=cm.committee_id WHERE cm.id=$1`, r.PathValue("id")).Scan(&eid)
	if !a.canManageEnrolment(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "This committee is outside your assigned scope.")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE committee_members SET conflict_state='resolved',resolution=$2 WHERE id=$1 AND conflict_state='declared'`, r.PathValue("id"), in.Resolution)
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if err != nil || n != 1 {
		problem(w, 409, "Conflict not resolved", "A declared conflict is required.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "resolved"})
}
func (a *app) confirmCommittee(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	var eid string
	_ = a.db.QueryRowContext(r.Context(), `SELECT enrolment_id FROM committees WHERE id=$1`, r.PathValue("id")).Scan(&eid)
	if !a.canManageEnrolment(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "This committee is outside your assigned scope.")
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE committees c SET state='confirmed',version=version+1 WHERE id=$1 AND (SELECT count(DISTINCT member_role) FROM committee_members WHERE committee_id=c.id AND member_role IN('chair','internal_examiner','external_examiner'))=3 AND NOT EXISTS(SELECT 1 FROM committee_members cm WHERE cm.committee_id=c.id AND (cm.response<>'accepted' OR cm.conflict_state NOT IN('none','resolved')))`, r.PathValue("id"))
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if err != nil || n != 1 {
		problem(w, 409, "Committee not ready", "Every member must accept and every conflict must be cleared.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "confirmed"})
}
func (a *app) availabilityOverlaps(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var eid string
	var member bool
	_ = a.db.QueryRowContext(r.Context(), `SELECT enrolment_id,EXISTS(SELECT 1 FROM committee_members WHERE committee_id=c.id AND user_id=$2) FROM committees c WHERE id=$1`, r.PathValue("id"), u.ID).Scan(&eid, &member)
	if !member && !a.canManageEnrolment(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "Committee unavailable.")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `WITH windows AS(SELECT aw.*,cm.committee_id FROM availability_windows aw JOIN committee_members cm ON cm.id=aw.committee_member_id WHERE cm.committee_id=$1), boundaries AS(SELECT starts_at AS t FROM windows UNION SELECT ends_at FROM windows), segments AS(SELECT t,lead(t) OVER(ORDER BY t) AS finish FROM boundaries) SELECT t,finish FROM segments WHERE finish>t AND NOT EXISTS(SELECT 1 FROM committee_members cm WHERE cm.committee_id=$1 AND NOT EXISTS(SELECT 1 FROM windows aw WHERE aw.committee_member_id=cm.id AND aw.starts_at<=t AND aw.ends_at>=finish)) ORDER BY t`, r.PathValue("id"))
	if err != nil {
		problem(w, 500, "Availability unavailable", "Try again.")
		return
	}
	defer rows.Close()
	out := []map[string]time.Time{}
	for rows.Next() {
		var s, e time.Time
		rows.Scan(&s, &e)
		out = append(out, map[string]time.Time{"startsAt": s, "endsAt": e})
	}
	writeJSON(w, 200, out)
}
func (a *app) scheduleDefence(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	var in struct {
		CommitteeID, SubmissionVersionID, Venue, Link string
		StartsAt                                      time.Time
		Duration                                      int
	}
	if !decode(w, r, &in) {
		return
	}
	var eid string
	_ = a.db.QueryRowContext(r.Context(), `SELECT enrolment_id FROM committees WHERE id=$1`, in.CommitteeID).Scan(&eid)
	if !a.canManageEnrolment(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "This committee is outside your assigned scope.")
		return
	}
	if in.Duration < 15 || in.Duration > 1440 || in.StartsAt.IsZero() || (strings.TrimSpace(in.Venue) == "" && strings.TrimSpace(in.Link) == "") {
		problem(w, 400, "Invalid defence slot", "Provide a date, venue or meeting link and a duration of 15–1440 minutes.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		problem(w, 503, "Scheduling unavailable", "Try again.")
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO defence_events(committee_id,submission_version_id,starts_at,duration_minutes,venue,meeting_link,state,created_by)
 SELECT $1,$2,$3,$4,$5,$6,'awaiting_confirmation',$7
 WHERE EXISTS(SELECT 1 FROM committees c JOIN enrolments e ON e.id=c.enrolment_id JOIN submissions s ON s.student_id=e.student_id JOIN submission_versions sv ON sv.submission_id=s.id WHERE c.id=$1 AND c.state='confirmed' AND sv.id=$2 AND sv.version=s.current_version AND s.state='approved')
 AND NOT EXISTS(SELECT 1 FROM committee_members cm WHERE cm.committee_id=$1 AND NOT EXISTS(SELECT 1 FROM availability_windows aw WHERE aw.committee_member_id=cm.id AND aw.starts_at<=$3::timestamptz AND aw.ends_at>=$3::timestamptz+($4::integer*interval '1 minute')))
 AND NOT EXISTS(SELECT 1 FROM defence_events de WHERE de.state IN('awaiting_confirmation','confirmed') AND tstzrange(de.starts_at,de.starts_at+de.duration_minutes*interval '1 minute','[)') && tstzrange($3::timestamptz,$3::timestamptz+$4::integer*interval '1 minute','[)') AND ((NULLIF($5,'') IS NOT NULL AND lower(de.venue)=lower($5)) OR EXISTS(SELECT 1 FROM committee_members a JOIN committee_members b ON b.user_id=a.user_id WHERE a.committee_id=de.committee_id AND b.committee_id=$1))) RETURNING id`, in.CommitteeID, in.SubmissionVersionID, in.StartsAt, in.Duration, in.Venue, in.Link, u.ID).Scan(&id)
	if err != nil {
		problem(w, 409, "Defence not scheduled", "Use this student's current approved package, confirm the committee, choose shared availability and resolve participant or venue conflicts.")
		return
	}
	if tx.Commit() != nil {
		problem(w, 409, "Scheduling changed", "A conflicting event was scheduled. Refresh and choose another slot.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "awaiting_confirmation"})
}
func (a *app) recordOutcome(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	eid, lookupErr := a.enrolmentFor(r.Context(), "defence_events", r.PathValue("id"))
	if lookupErr != nil || !a.canManageEnrolment(r.Context(), u, eid) {
		problem(w, 403, "Access denied", "This defence is outside your assigned scope.")
		return
	}
	var in struct {
		Outcome, Summary string
		Corrections      []struct{ Title, DueDate, VerifierID string }
	}
	if !decode(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Summary)) < 12 || ((in.Outcome == "corrections_required" || in.Outcome == "resubmission_required") && len(in.Corrections) == 0) {
		problem(w, 400, "Outcome details required", "Provide the consolidated summary and a dated correction task for outcomes requiring further work.")
		return
	}
	for _, correction := range in.Corrections {
		var valid bool
		_ = a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM committee_members cm JOIN defence_events de ON de.committee_id=cm.committee_id WHERE de.id=$1 AND cm.user_id=$2 AND cm.response='accepted' AND cm.conflict_state IN('none','resolved'))`, r.PathValue("id"), correction.VerifierID).Scan(&valid)
		if !valid {
			problem(w, 400, "Invalid verifier", "Choose an accepted, conflict-cleared committee member.")
			return
		}
	}
	tx, _ := a.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	var oid string
	err := tx.QueryRowContext(r.Context(), `INSERT INTO examination_outcomes(defence_id,outcome,summary,recorded_by) SELECT $1,$2,$3,$4 WHERE EXISTS(SELECT 1 FROM defence_events de JOIN committees c ON c.id=de.committee_id WHERE de.id=$1 AND c.state='confirmed' AND de.state='confirmed' AND NOT EXISTS(SELECT 1 FROM committee_members cm WHERE cm.committee_id=c.id AND cm.member_role IN('internal_examiner','external_examiner') AND NOT EXISTS(SELECT 1 FROM examiner_reports er WHERE er.defence_id=de.id AND er.examiner_id=cm.user_id))) RETURNING id`, r.PathValue("id"), in.Outcome, in.Summary, u.ID).Scan(&oid)
	for _, c := range in.Corrections {
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO corrections(outcome_id,title,due_date,verifier_id) VALUES($1,$2,$3,$4)`, oid, c.Title, c.DueDate, c.VerifierID)
		}
	}
	if err != nil {
		problem(w, 409, "Outcome not recorded", "Committee conflicts must be resolved and the committee confirmed.")
		return
	}
	auditTx(r.Context(), tx, u.ID, "examination_outcome", oid, "recorded", "Consolidated examination outcome recorded")
	if _, err = tx.ExecContext(r.Context(), `UPDATE defence_events SET state='completed' WHERE id=$1`, r.PathValue("id")); err != nil {
		problem(w, 409, "Outcome not recorded", "Refresh and try again.")
		return
	}
	if err := tx.Commit(); err != nil {
		problem(w, 409, "Change not saved", "The record changed or storage is unavailable. Refresh and try again.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": oid, "status": in.Outcome})
}
func (a *app) verifyCorrection(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Decision string }
	if !decode(w, r, &in) {
		return
	}
	state := "returned"
	if in.Decision == "verified" {
		state = "verified"
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE corrections SET state=$3,verified_at=CASE WHEN $3='verified' THEN now() END WHERE id=$1 AND verifier_id=$2 AND state='submitted'`, r.PathValue("id"), u.ID, state)
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if err != nil || n != 1 {
		problem(w, 409, "Correction not updated", "This task is not assigned to you or is not submitted.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": state})
}

func (a *app) notifications(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT id,title,body,urgency,read_at IS NOT NULL AS read,dismissed_at IS NOT NULL AS dismissed,updated_at AS "updatedAt",milestone_id,CASE WHEN target_view<>'' THEN target_view WHEN dedupe_key LIKE 'concept%' THEN 'concepts' WHEN dedupe_key LIKE 'submission:%' OR dedupe_key LIKE 'review-deadline:%' THEN 'reviews' WHEN dedupe_key LIKE 'review:%' OR dedupe_key LIKE 'deadline:%' THEN 'journey' ELSE '' END AS target_view FROM notifications n WHERE recipient_id=$1
 AND (dedupe_key NOT LIKE 'deadline:%' AND dedupe_key NOT LIKE 'assigned-task:%' OR EXISTS(SELECT 1 FROM action_tasks t JOIN enrolments e ON e.id=t.enrolment_id WHERE (n.dedupe_key='deadline:'||t.id OR n.dedupe_key='assigned-task:'||t.id) AND t.state='open' AND NOT(e.state='approved_leave' AND t.owner_id=e.student_id)))
 AND (dedupe_key NOT LIKE 'review-deadline:%' OR EXISTS(SELECT 1 FROM review_assignments ra WHERE n.dedupe_key='review-deadline:'||ra.id AND ra.state='awaiting_review' AND NOT ra.round_closed))
 ORDER BY dismissed_at NULLS FIRST,updated_at DESC LIMIT 100`, current(r).ID)
}
func (a *app) dismissNotification(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	res, _ := a.db.ExecContext(r.Context(), `UPDATE notifications SET dismissed_at=now() WHERE id=$1 AND recipient_id=$2`, r.PathValue("id"), u.ID)
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if n != 1 {
		problem(w, 404, "Notification not found", "It is unavailable.")
		return
	}
	w.WriteHeader(204)
}
func (a *app) runReminderJobs(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "admin") {
		return
	}
	n, err := a.processReminders(r.Context())
	if err != nil {
		problem(w, 500, "Reminder run failed", "Try again.")
		return
	}
	writeJSON(w, 200, map[string]int64{"updated": n})
}
func (a *app) processReminders(ctx context.Context) (int64, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE supervision_requests SET state='expired' WHERE state='accepted' AND reservation_expires_at<now()`)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO notifications(recipient_id,dedupe_key,title,urgency) SELECT ra.reviewer_id,'review-deadline:'||ra.id,'Academic review awaiting your decision',CASE WHEN ra.due_at<now() THEN 'overdue' ELSE 'warning' END FROM review_assignments ra WHERE ra.state='awaiting_review' AND NOT ra.round_closed AND ra.due_at<=now()+COALESCE((SELECT (settings->>'warningDays')::int FROM institution_settings LIMIT 1),4)*interval '1 day' ON CONFLICT(recipient_id,dedupe_key) DO UPDATE SET urgency=excluded.urgency`)
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO notifications(recipient_id,dedupe_key,title,urgency,updated_at) SELECT at.owner_id,'deadline:'||at.id,at.title,CASE WHEN at.due_at<now() THEN 'overdue' WHEN at.due_at::date=current_date THEN 'due_today' ELSE 'due_soon' END,now() FROM action_tasks at JOIN enrolments e ON e.id=at.enrolment_id WHERE at.state='open' AND at.due_at<=now()+COALESCE((SELECT (settings->>'warningDays')::int FROM institution_settings LIMIT 1),4)*interval '1 day' AND NOT(e.state='approved_leave' AND at.owner_id=e.student_id) ON CONFLICT(recipient_id,dedupe_key) DO UPDATE SET urgency=excluded.urgency,title=excluded.title,updated_at=now()`)
	if err != nil {
		return 0, err
	}
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO notifications(recipient_id,dedupe_key,title,urgency,updated_at) SELECT ra.user_id,'escalation:'||at.id||':'||ra.role,'Overdue follow-up: '||at.title,'overdue',now() FROM action_tasks at JOIN enrolments e ON e.id=at.enrolment_id JOIN programmes p ON p.id=e.programme_id JOIN role_assignments ra ON (at.due_at<=now()-COALESCE((SELECT (settings->>'hodEscalationDays')::int FROM institution_settings LIMIT 1),14)*interval '1 day' AND ra.role='hod' AND ra.scope_type='department' AND ra.scope_id=p.department_id) OR (at.due_at<=now()-COALESCE((SELECT (settings->>'coordinatorEscalationDays')::int FROM institution_settings LIMIT 1),7)*interval '1 day' AND ra.role='coordinator' AND ra.scope_type='programme' AND ra.scope_id=p.id) WHERE at.state='open' AND NOT(e.state='approved_leave' AND at.owner_id=e.student_id) AND at.due_at<now()-COALESCE((SELECT (settings->>'coordinatorEscalationDays')::int FROM institution_settings LIMIT 1),7)*interval '1 day' ON CONFLICT(recipient_id,dedupe_key) DO UPDATE SET title=excluded.title,urgency=excluded.urgency,updated_at=now()`)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}
func (a *app) reminderLoop() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		_, err := a.processReminders(ctx)
		cancel()
		if err != nil {
			fmt.Printf("reminder worker: %v\n", err)
		}
		<-ticker.C
	}
}

func (a *app) progressCSV(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership") {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `WITH scopes AS(SELECT scope_type,scope_id FROM role_assignments WHERE user_id=$1), allowed AS(SELECT DISTINCT p.id,p.name FROM scopes s JOIN programmes p ON (s.scope_type='programme' AND p.id=s.scope_id) OR (s.scope_type='department' AND p.department_id=s.scope_id) OR (s.scope_type='school' AND EXISTS(SELECT 1 FROM departments d WHERE d.id=p.department_id AND d.school_id=s.scope_id)) OR s.scope_type='institution') SELECT st.full_name,COALESCE(st.student_number,''),p.name,e.state::text,COALESCE((SELECT md.stage_label FROM student_plans sp JOIN milestone_instances mi ON mi.plan_id=sp.id JOIN milestone_definitions md ON md.id=mi.definition_id WHERE sp.enrolment_id=e.id AND mi.state IN('in_progress','submitted','awaiting_review','changes_requested') ORDER BY md.position LIMIT 1),'Not started'),(SELECT count(*) FROM student_plans sp JOIN milestone_instances mi ON mi.plan_id=sp.id WHERE sp.enrolment_id=e.id AND mi.state='approved'),(SELECT count(*) FROM student_plans sp JOIN milestone_instances mi ON mi.plan_id=sp.id WHERE sp.enrolment_id=e.id) FROM allowed p JOIN enrolments e ON e.programme_id=p.id JOIN users st ON st.id=e.student_id ORDER BY st.full_name`, u.ID)
	if err != nil {
		problem(w, 500, "Report unavailable", "Try again.")
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=student-progress.csv")
	cw := csv.NewWriter(w)
	cw.Write([]string{"Student", "Registration number", "Programme", "Enrolment state", "Current stage", "Milestones complete", "Milestones total"})
	for rows.Next() {
		var n, num, p, state, stage string
		var done, total int
		rows.Scan(&n, &num, &p, &state, &stage, &done, &total)
		cw.Write([]string{csvSafe(n), csvSafe(num), csvSafe(p), state, stage, strconv.Itoa(done), strconv.Itoa(total)})
	}
	cw.Flush()
}
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@", rune(s[0])) {
		return "'" + s
	}
	return s
}
func (a *app) audit(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "coordinator", "hod", "dean", "leadership", "admin") {
		return
	}
	broad := hasRole(u, "leadership") || hasRole(u, "admin")
	rows, err := a.db.QueryContext(r.Context(), `SELECT ae.id,COALESCE(actor.full_name,'System'),ae.entity_type,ae.entity_id,ae.action,ae.safe_summary,ae.occurred_at FROM audit_events ae LEFT JOIN users actor ON actor.id=ae.actor_id WHERE $2 OR ae.actor_id=$1 ORDER BY ae.occurred_at DESC LIMIT 200`, u.ID, broad)
	if err != nil {
		problem(w, 500, "Audit unavailable", "Try again.")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var actor, typ, eid, action, summary string
		var at time.Time
		rows.Scan(&id, &actor, &typ, &eid, &action, &summary, &at)
		out = append(out, map[string]any{"id": id, "actor": actor, "entityType": typ, "entityId": eid, "action": action, "summary": summary, "occurredAt": at})
	}
	writeJSON(w, 200, out)
}
func (a *app) moduleSummary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	allowed := map[string]bool{"onboarding": true, "templates": true, "supervision": true, "topics": true, "reviews": true, "meetings": true, "cases": true, "leave": true, "examination": true, "reports": true, "administration": true}
	if !allowed[name] {
		problem(w, 404, "Module not found", "Choose a platform module.")
		return
	}
	u := current(r)
	var count int
	queries := map[string]string{"supervision": `SELECT count(*) FROM supervision_requests sr JOIN enrolments e ON e.id=sr.enrolment_id WHERE e.student_id=$1 OR sr.supervisor_id=$1`, "topics": `SELECT count(*) FROM topics`, "reviews": `SELECT count(*) FROM review_assignments WHERE reviewer_id=$1`, "meetings": `SELECT count(*) FROM meetings m WHERE m.organizer_id=$1 OR EXISTS(SELECT 1 FROM meeting_participants WHERE meeting_id=m.id AND user_id=$1)`, "cases": `SELECT count(*) FROM support_cases sc JOIN enrolments e ON e.id=sc.enrolment_id WHERE e.student_id=$1 OR sc.owner_id=$1`, "leave": `SELECT count(*) FROM leave_requests lr JOIN enrolments e ON e.id=lr.enrolment_id WHERE e.student_id=$1`, "examination": `SELECT count(*) FROM defence_events de JOIN committees c ON c.id=de.committee_id JOIN enrolments e ON e.id=c.enrolment_id WHERE e.student_id=$1 OR de.created_by=$1`}
	if q := queries[name]; q != "" {
		_ = a.db.QueryRowContext(r.Context(), q, u.ID).Scan(&count)
	}
	writeJSON(w, 200, map[string]any{"module": name, "records": count, "status": "available", "serverBacked": true})
}

var _ = fmt.Sprintf

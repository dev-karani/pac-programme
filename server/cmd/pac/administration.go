package main

import (
	"database/sql"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strings"
)

func (a *app) registerAdminWorkflows(m *http.ServeMux) {
	for p, h := range map[string]http.HandlerFunc{
		"GET /api/v1/catalog": a.catalog, "GET /api/v1/admin/accounts": a.accounts, "POST /api/v1/admin/accounts": a.createAccount,
		"POST /api/v1/admin/roles": a.addRole, "GET /api/v1/templates": a.templates, "POST /api/v1/templates": a.saveTemplate,
		"GET /api/v1/templates/{id}": a.templateDetail, "POST /api/v1/templates/{id}/publish": a.publishTemplate,
		"POST /api/v1/enrolments/{id}/migrate": a.migratePlan, "POST /api/v1/milestones/{id}/signoff": a.signoff,
		"POST /api/v1/reviews/{id}/reopen": a.reopenReview,
	} {
		m.HandleFunc(p, a.withAuth(h))
	}
}
func (a *app) catalog(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT s.id AS school_id,s.name AS school,d.id AS department_id,d.name AS department,p.id AS programme_id,p.name AS programme,c.id AS cohort_id,c.name AS cohort,c.study_mode FROM schools s LEFT JOIN departments d ON d.school_id=s.id LEFT JOIN programmes p ON p.department_id=d.id LEFT JOIN cohorts c ON c.programme_id=p.id ORDER BY s.name,p.name,c.name`)
}
func (a *app) accounts(w http.ResponseWriter, r *http.Request) {
	if !requireAny(w, current(r), "admin") {
		return
	}
	a.sendRows(w, r, `SELECT u.id,u.email,u.full_name,u.active,u.must_change_password,u.student_number,COALESCE((SELECT json_agg(json_build_object('role',role,'scope_type',scope_type,'scope_id',scope_id)) FROM role_assignments WHERE user_id=u.id),'[]') AS roles FROM users u ORDER BY full_name`)
}
func (a *app) createAccount(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "admin") {
		return
	}
	var in struct{ Email, Name, Password, StudentNumber, ProgrammeID, CohortID, StudyMode, AdmissionDate, Role, ScopeType, ScopeID string }
	if !decode(w, r, &in) {
		return
	}
	if len(in.Password) < 12 || !strings.Contains(in.Email, "@") || len(in.Name) < 3 {
		problem(w, 400, "Invalid account", "Supply a name, email and temporary password of at least 12 characters.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO users(email,full_name,password_hash,student_number) VALUES(lower($1),$2,$3,NULLIF($4,'')) RETURNING id`, in.Email, in.Name, string(hash), in.StudentNumber).Scan(&id)
	if in.Role == "student" {
		in.ScopeType = "self"
		in.ScopeID = id
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO role_assignments(user_id,role,scope_type,scope_id) VALUES($1,$2,$3,$4)`, id, in.Role, in.ScopeType, in.ScopeID)
	}
	if err == nil && in.Role == "student" {
		var eid string
		err = tx.QueryRowContext(r.Context(), `INSERT INTO enrolments(student_id,programme_id,cohort_id,study_mode,admission_date,verification_status) SELECT $1,$2,$3,$4,$5,'draft' WHERE EXISTS(SELECT 1 FROM cohorts WHERE id=$3 AND programme_id=$2) RETURNING id`, id, in.ProgrammeID, in.CohortID, in.StudyMode, in.AdmissionDate).Scan(&eid)
	}
	if err == nil {
		err = auditTx(r.Context(), tx, u.ID, "user", id, "created", "Account and assigned role created")
	}
	commitResponse(w, tx, err, map[string]string{"id": id})
}
func (a *app) addRole(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "admin") {
		return
	}
	var in struct{ UserID, Role, ScopeType, ScopeID, Reason string }
	if !decode(w, r, &in) {
		return
	}
	if len(in.Reason) < 8 {
		problem(w, 400, "Reason required", "Explain this role assignment.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO role_assignments(user_id,role,scope_type,scope_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, in.UserID, in.Role, in.ScopeType, in.ScopeID)
	if err == nil {
		err = auditTx(r.Context(), tx, u.ID, "user", in.UserID, "role_assigned", "Role assignment recorded: "+in.Reason)
	}
	commitResponse(w, tx, err, map[string]string{"status": "assigned"})
}
func (a *app) canManageProgramme(r *http.Request, pid string) bool {
	var ok bool
	_ = a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM programmes p JOIN departments d ON d.id=p.department_id JOIN role_assignments ra ON ra.user_id=$1 WHERE p.id=$2 AND ra.role IN('coordinator','hod','dean','leadership') AND ((ra.scope_type='programme' AND ra.scope_id=p.id) OR (ra.scope_type='department' AND ra.scope_id=d.id) OR (ra.scope_type='school' AND ra.scope_id=d.school_id) OR ra.scope_type='institution'))`, current(r).ID, pid).Scan(&ok)
	return ok
}
func (a *app) templates(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	a.sendRows(w, r, `SELECT t.*,p.name AS programme FROM programme_templates t JOIN programmes p ON p.id=t.programme_id JOIN departments d ON d.id=p.department_id WHERE EXISTS(SELECT 1 FROM role_assignments ra WHERE ra.user_id=$1 AND ra.role IN('coordinator','hod','dean','leadership') AND ((ra.scope_type='programme' AND ra.scope_id=p.id) OR (ra.scope_type='department' AND ra.scope_id=d.id) OR (ra.scope_type='school' AND ra.scope_id=d.school_id) OR ra.scope_type='institution')) ORDER BY p.name,t.version DESC`, u.ID)
}

type templateMilestone struct {
	Title, Description, Stage, ApprovalMode string
	StartDays, EndDays                      int
	FinalSignoff, SimilarityRequired        bool
	Dependencies                            []int
	Activities                              []struct {
		Title, Type, OwnerRole, Instructions string
		Required                             bool
		DueDays                              int
	}
}

func (a *app) saveTemplate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ProgrammeID, Name, StudyMode string
		Milestones                   []templateMilestone
	}
	if !decode(w, r, &in) {
		return
	}
	if !a.canManageProgramme(r, in.ProgrammeID) {
		problem(w, 403, "Access denied", "Programme outside your scope.")
		return
	}
	if len(in.Milestones) == 0 {
		problem(w, 400, "Milestones required", "Add at least one milestone.")
		return
	}
	for i, m := range in.Milestones {
		if m.EndDays < m.StartDays || len(m.Title) < 3 {
			problem(w, 400, "Invalid milestone", "Check milestone titles and date offsets.")
			return
		}
		for _, dep := range m.Dependencies {
			if dep < 1 || dep > i {
				problem(w, 400, "Invalid dependency", "Dependencies must refer to an earlier milestone number; cycles are not permitted.")
				return
			}
		}
	}
	tx, err := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO programme_templates(programme_id,name,study_mode,version,state) SELECT $1,$2,$3,COALESCE(max(version),0)+1,'draft' FROM programme_templates WHERE programme_id=$1 AND study_mode=$3 RETURNING id`, in.ProgrammeID, in.Name, in.StudyMode).Scan(&id)
	ids := []string{}
	for i, m := range in.Milestones {
		if err != nil {
			break
		}
		if m.ApprovalMode == "" {
			m.ApprovalMode = "primary"
		}
		var mid string
		err = tx.QueryRowContext(r.Context(), `INSERT INTO milestone_definitions(template_id,title,description,stage_label,position,start_days,end_days,requires_final_signoff,similarity_required,approval_mode) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, id, m.Title, m.Description, m.Stage, i+1, m.StartDays, m.EndDays, m.FinalSignoff, m.SimilarityRequired, m.ApprovalMode).Scan(&mid)
		ids = append(ids, mid)
		for _, dep := range m.Dependencies {
			if err == nil {
				_, err = tx.ExecContext(r.Context(), `INSERT INTO milestone_dependencies(milestone_id,depends_on_id) VALUES($1,$2)`, mid, ids[dep-1])
			}
		}
		for j, activity := range m.Activities {
			if err == nil {
				_, err = tx.ExecContext(r.Context(), `INSERT INTO activity_definitions(milestone_id,title,activity_type,owner_role,instructions,required,due_offset_days,position) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, mid, activity.Title, activity.Type, activity.OwnerRole, activity.Instructions, activity.Required, activity.DueDays, j+1)
			}
		}
	}
	if err == nil {
		err = auditTx(r.Context(), tx, current(r).ID, "programme_template", id, "created", "New immutable template version drafted")
	}
	commitResponse(w, tx, err, map[string]string{"id": id})
}
func (a *app) templateDetail(w http.ResponseWriter, r *http.Request) {
	var pid string
	_ = a.db.QueryRowContext(r.Context(), `SELECT programme_id FROM programme_templates WHERE id=$1`, r.PathValue("id")).Scan(&pid)
	if !a.canManageProgramme(r, pid) {
		problem(w, 403, "Access denied", "Template outside your scope.")
		return
	}
	a.sendRows(w, r, `SELECT md.*,COALESCE((SELECT json_agg(ad) FROM activity_definitions ad WHERE ad.milestone_id=md.id),'[]') AS activities,COALESCE((SELECT json_agg(other.position) FROM milestone_dependencies dep JOIN milestone_definitions other ON other.id=dep.depends_on_id WHERE dep.milestone_id=md.id),'[]') AS dependencies FROM milestone_definitions md WHERE template_id=$1 ORDER BY position`, r.PathValue("id"))
}
func (a *app) publishTemplate(w http.ResponseWriter, r *http.Request) {
	var pid string
	_ = a.db.QueryRowContext(r.Context(), `SELECT programme_id FROM programme_templates WHERE id=$1`, r.PathValue("id")).Scan(&pid)
	if !a.canManageProgramme(r, pid) {
		problem(w, 403, "Access denied", "Template outside your scope.")
		return
	}
	_, err := a.db.ExecContext(r.Context(), `UPDATE programme_templates SET state='published' WHERE id=$1 AND state='draft'`, r.PathValue("id"))
	if err != nil {
		problem(w, 409, "Not published", "Refresh the template.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "published"})
}
func (a *app) migratePlan(w http.ResponseWriter, r *http.Request) {
	if !a.canManageEnrolment(r.Context(), current(r), r.PathValue("id")) {
		problem(w, 403, "Access denied", "Enrolment outside your scope.")
		return
	}
	var in struct {
		TemplateID, Reason string
		Apply              bool
	}
	if !decode(w, r, &in) {
		return
	}
	q := `SELECT mi.id,old.title AS old_title,new.title AS new_title,mi.current_due,sp.started_on+new.end_days AS proposed_due,new.id AS definition_id FROM student_plans sp JOIN enrolments e ON e.id=sp.enrolment_id JOIN programme_templates t ON t.id=$2 AND t.programme_id=e.programme_id AND t.study_mode=e.study_mode AND t.state='published' JOIN milestone_instances mi ON mi.plan_id=sp.id JOIN milestone_definitions old ON old.id=mi.definition_id JOIN milestone_definitions new ON new.template_id=t.id AND new.position=old.position WHERE sp.enrolment_id=$1 AND mi.state='not_started' AND NOT EXISTS(SELECT 1 FROM submissions WHERE milestone_id=mi.id) AND NOT EXISTS(SELECT 1 FROM baseline_completions WHERE milestone_id=mi.id)`
	if !in.Apply {
		a.sendRows(w, r, q, r.PathValue("id"), in.TemplateID)
		return
	}
	if len(in.Reason) < 8 {
		problem(w, 400, "Reason required", "Explain the template migration.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return
	}
	defer tx.Rollback()
	var revision string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO plan_revisions(plan_id,source_type,reason,approved_by) SELECT id,'template_migration',$2,$3 FROM student_plans WHERE enrolment_id=$1 RETURNING id`, r.PathValue("id"), in.Reason, current(r).ID).Scan(&revision)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO schedule_revision_items(revision_id,milestone_id,old_start,old_due,new_start,new_due) SELECT $3,mi.id,mi.current_start,mi.current_due,sp.started_on+md.start_days,sp.started_on+md.end_days FROM (`+q+`) eligible JOIN milestone_instances mi ON mi.id=eligible.id JOIN milestone_definitions md ON md.id=eligible.definition_id JOIN student_plans sp ON sp.id=mi.plan_id`, r.PathValue("id"), in.TemplateID, revision)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE milestone_instances mi SET definition_id=eligible.definition_id,current_start=sp.started_on+md.start_days,current_due=sp.started_on+md.end_days,version=mi.version+1 FROM (`+q+`) eligible JOIN milestone_definitions md ON md.id=eligible.definition_id JOIN milestone_instances old ON old.id=eligible.id JOIN student_plans sp ON sp.id=old.plan_id WHERE mi.id=eligible.id`, r.PathValue("id"), in.TemplateID)
	}
	commitResponse(w, tx, err, map[string]string{"status": "migrated"})
}
func (a *app) signoff(w http.ResponseWriter, r *http.Request) {
	eid, ok := a.milestoneEnrolment(r)
	if !ok || !a.assignedSupervisor(r.Context(), current(r), eid) {
		problem(w, 403, "Access denied", "An assigned supervisor must provide academic sign-off.")
		return
	}
	var in struct{ Reason string }
	if !decode(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Reason)) < 8 {
		problem(w, 400, "Sign-off note required", "Explain the verified milestone completion.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `UPDATE milestone_instances mi SET state='approved',approved_at=now() WHERE mi.id=$1 AND mi.state IN('in_progress','awaiting_review') AND NOT EXISTS(SELECT 1 FROM activity_instances ai JOIN activity_definitions ad ON ad.id=ai.definition_id WHERE ai.milestone_id=mi.id AND ad.required AND ai.state<>'approved') AND NOT EXISTS(SELECT 1 FROM submissions s WHERE s.milestone_id=mi.id AND s.state<>'approved') RETURNING id`, r.PathValue("id")).Scan(&id)
	if err == nil {
		err = advancePlan(r.Context(), tx, eid)
	}
	if err == nil {
		err = auditTx(r.Context(), tx, current(r).ID, "milestone", id, "signed_off", in.Reason)
	}
	commitResponse(w, tx, err, map[string]string{"status": "approved"})
}
func (a *app) reopenReview(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	if !decode(w, r, &in) {
		return
	}
	var eid, vid, mid string
	_ = a.db.QueryRowContext(r.Context(), `SELECT sp.enrolment_id,ra.submission_version_id,s.milestone_id FROM review_assignments ra JOIN submission_versions sv ON sv.id=ra.submission_version_id JOIN submissions s ON s.id=sv.submission_id JOIN milestone_instances mi ON mi.id=s.milestone_id JOIN student_plans sp ON sp.id=mi.plan_id WHERE ra.id=$1`, r.PathValue("id")).Scan(&eid, &vid, &mid)
	if !a.canManageEnrolment(r.Context(), current(r), eid) || len(in.Reason) < 8 {
		problem(w, 403, "Reopening not authorized", "Scoped academic administration and a reason are required.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO review_reopenings(submission_version_id,reason,reopened_by) VALUES($1,$2,$3)`, vid, in.Reason, current(r).ID)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE review_decisions SET superseded_at=now() WHERE review_assignment_id IN(SELECT id FROM review_assignments WHERE submission_version_id=$1) AND superseded_at IS NULL`, vid)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE review_assignments SET state='awaiting_review',decided_at=NULL,round_closed=false WHERE submission_version_id=$1`, vid)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE submissions s SET state='submitted' FROM submission_versions sv WHERE sv.id=$1 AND s.id=sv.submission_id AND sv.version=s.current_version`, vid)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE milestone_instances SET state='awaiting_review',approved_at=NULL WHERE id=$1`, mid)
	}
	commitResponse(w, tx, err, map[string]string{"status": "reopened; dependent work preserved and requires review"})
}

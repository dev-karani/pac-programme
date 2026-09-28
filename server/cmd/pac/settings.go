package main

import (
	"encoding/csv"
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/url"
	"strings"
)

func (a *app) registerSettings(m *http.ServeMux) {
	for p, h := range map[string]http.HandlerFunc{
		"POST /api/v1/admin/import-students": a.importStudents, "POST /api/v1/admin/organization": a.createOrganization,
		"GET /api/v1/settings": a.settings, "POST /api/v1/settings": a.saveSettings, "POST /api/v1/supervisor-profile": a.saveSupervisorProfile,
		"POST /api/v1/resources": a.saveResource, "GET /api/v1/workspace/assignments": a.assignments,
		"GET /api/v1/workspace/tasks": a.tasks, "POST /api/v1/tasks/{id}/complete": a.completeTask,
		"POST /api/v1/enrolments/{id}/return": a.returnFromLeave,
		"GET /api/v1/allocation-exceptions":   a.allocationExceptions, "POST /api/v1/allocation-exceptions": a.recordAllocationException,
	} {
		m.HandleFunc(p, a.withAuth(h))
	}
}
func (a *app) importStudents(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !requireAny(w, u, "admin") {
		return
	}
	var in struct{ CSV, CohortID, AdmissionDate string }
	if !decode(w, r, &in) {
		return
	}
	records, err := csv.NewReader(strings.NewReader(in.CSV)).ReadAll()
	if err != nil || len(records) < 2 || len(records) > 501 {
		problem(w, 400, "Invalid CSV", "Use a header and 1–500 rows: name,email,student_number,password.")
		return
	}
	if strings.Join(records[0], ",") != "name,email,student_number,password" {
		problem(w, 400, "Invalid columns", "Expected name,email,student_number,password.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	for _, row := range records[1:] {
		if len(row) != 4 || len(row[3]) < 12 || !strings.Contains(row[1], "@") {
			problem(w, 400, "Invalid row", "Each row needs four fields and a temporary password of at least 12 characters.")
			return
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(row[3]), bcrypt.DefaultCost)
		if e != nil {
			return
		}
		var id, eid string
		err = tx.QueryRowContext(r.Context(), `INSERT INTO users(full_name,email,student_number,password_hash) VALUES($1,lower($2),$3,$4) RETURNING id`, row[0], row[1], row[2], string(hash)).Scan(&id)
		if err != nil {
			break
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO role_assignments(user_id,role,scope_type,scope_id) VALUES($1,'student','self',$1)`, id)
		if err != nil {
			break
		}
		err = tx.QueryRowContext(r.Context(), `INSERT INTO enrolments(student_id,programme_id,cohort_id,admission_date,study_mode,verification_status) SELECT $1,programme_id,id,$3,study_mode,'draft' FROM cohorts WHERE id=$2 RETURNING id`, id, in.CohortID, in.AdmissionDate).Scan(&eid)
		if err != nil {
			break
		}
		err = auditTx(r.Context(), tx, u.ID, "user", id, "imported", "Student account imported with temporary credentials")
		if err != nil {
			break
		}
	}
	commitResponse(w, tx, err, map[string]int{"imported": len(records) - 1})
}
func (a *app) createOrganization(w http.ResponseWriter, r *http.Request) {
	if !requireAny(w, current(r), "admin") {
		return
	}
	var in struct{ Kind, Name, ParentID, Code, IntakeDate, StudyMode string }
	if !decode(w, r, &in) {
		return
	}
	if len(in.Name) < 2 {
		problem(w, 400, "Name required", "Enter an organization name.")
		return
	}
	var id string
	var err error
	switch in.Kind {
	case "school":
		err = a.db.QueryRowContext(r.Context(), `INSERT INTO schools(institution_id,name) VALUES($1,$2) RETURNING id`, in.ParentID, in.Name).Scan(&id)
	case "department":
		err = a.db.QueryRowContext(r.Context(), `INSERT INTO departments(school_id,name) VALUES($1,$2) RETURNING id`, in.ParentID, in.Name).Scan(&id)
	case "programme":
		err = a.db.QueryRowContext(r.Context(), `INSERT INTO programmes(department_id,name,code) VALUES($1,$2,$3) RETURNING id`, in.ParentID, in.Name, in.Code).Scan(&id)
	case "cohort":
		err = a.db.QueryRowContext(r.Context(), `INSERT INTO cohorts(programme_id,name,intake_date,study_mode) VALUES($1,$2,$3,$4) RETURNING id`, in.ParentID, in.Name, in.IntakeDate, in.StudyMode).Scan(&id)
	default:
		problem(w, 400, "Invalid type", "Choose school, department, programme or cohort.")
		return
	}
	if err != nil {
		problem(w, 409, "Organization not saved", "Check parent, name, code and dates.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}
func (a *app) settings(w http.ResponseWriter, r *http.Request) {
	var b []byte
	err := a.db.QueryRowContext(r.Context(), `SELECT settings FROM institution_settings LIMIT 1`).Scan(&b)
	if err != nil {
		writeJSON(w, 200, map[string]any{"reviewDays": 7, "warningDays": 4, "coordinatorEscalationDays": 7, "hodEscalationDays": 14})
		return
	}
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	writeJSON(w, 200, out)
}
func (a *app) saveSettings(w http.ResponseWriter, r *http.Request) {
	if !requireAny(w, current(r), "admin") {
		return
	}
	var in struct{ ReviewDays, WarningDays, CoordinatorEscalationDays, HodEscalationDays int }
	if !decode(w, r, &in) {
		return
	}
	if in.ReviewDays < 1 || in.WarningDays < 0 || in.CoordinatorEscalationDays < 1 || in.HodEscalationDays < in.CoordinatorEscalationDays {
		problem(w, 400, "Invalid policy", "Use positive durations and a later HOD escalation.")
		return
	}
	b, _ := json.Marshal(map[string]int{"reviewDays": in.ReviewDays, "warningDays": in.WarningDays, "coordinatorEscalationDays": in.CoordinatorEscalationDays, "hodEscalationDays": in.HodEscalationDays})
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO institution_settings(institution_id,settings) SELECT id,$1 FROM institutions ON CONFLICT(institution_id) DO UPDATE SET settings=excluded.settings`, b)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE programmes SET review_days=$1,warning_days=$2`, in.ReviewDays, in.WarningDays)
	}
	commitResponse(w, tx, err, map[string]string{"status": "saved"})
}
func (a *app) saveSupervisorProfile(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct {
		UserID, DepartmentID, Description, Tags string
		Capacity                                int
		Accepting                               bool
		ProgrammeIDs                            []string
	}
	if !decode(w, r, &in) {
		return
	}
	if !hasRole(u, "admin") {
		if !hasRole(u, "supervisor") || in.UserID != u.ID {
			problem(w, 403, "Access denied", "You may edit only your own profile.")
			return
		}
		if len(in.ProgrammeIDs) > 0 {
			problem(w, 403, "Eligibility approval required", "Programme eligibility is administered separately.")
			return
		}
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO supervisor_profiles(user_id,home_department_id,description,expertise_tags,accepting_students,capacity) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(user_id) DO UPDATE SET description=excluded.description,expertise_tags=excluded.expertise_tags,accepting_students=excluded.accepting_students,capacity=excluded.capacity`, in.UserID, in.DepartmentID, in.Description, strings.Split(in.Tags, ","), in.Accepting, in.Capacity)
	for _, pid := range in.ProgrammeIDs {
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO supervisor_programme_eligibility(supervisor_id,programme_id) VALUES($1,$2) ON CONFLICT(supervisor_id,programme_id) DO UPDATE SET active=true`, in.UserID, pid)
		}
	}
	commitResponse(w, tx, err, map[string]string{"status": "saved"})
}
func (a *app) saveResource(w http.ResponseWriter, r *http.Request) {
	var in struct{ MilestoneDefinitionID, Title, URL, Kind string }
	if !decode(w, r, &in) {
		return
	}
	var pid string
	_ = a.db.QueryRowContext(r.Context(), `SELECT t.programme_id FROM milestone_definitions md JOIN programme_templates t ON t.id=md.template_id WHERE md.id=$1`, in.MilestoneDefinitionID).Scan(&pid)
	if !a.canManageProgramme(r, pid) {
		problem(w, 403, "Access denied", "Milestone outside your programme scope.")
		return
	}
	link, err := url.Parse(in.URL)
	if err != nil || (link.Scheme != "https" && link.Scheme != "http") || link.Host == "" {
		problem(w, 400, "Invalid resource link", "Use an http or https resource address.")
		return
	}
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO resources(milestone_definition_id,title,kind,url,created_by) VALUES($1,$2,'link',$3,$4)`, in.MilestoneDefinitionID, in.Title, in.URL, current(r).ID)
	if err != nil {
		problem(w, 400, "Resource not saved", "Check the title and milestone.")
		return
	}
	writeJSON(w, 201, map[string]string{"status": "saved"})
}
func (a *app) assignments(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT sa.*,u.full_name AS supervisor,student.full_name AS student FROM supervision_assignments sa JOIN users u ON u.id=sa.supervisor_id JOIN enrolments e ON e.id=sa.enrolment_id JOIN users student ON student.id=e.student_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id WHERE `+academicScope+` ORDER BY student.full_name,sa.effective_from DESC`, current(r).ID)
}
func (a *app) tasks(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT t.*,u.full_name AS student FROM action_tasks t JOIN enrolments e ON e.id=t.enrolment_id JOIN users u ON u.id=e.student_id WHERE t.owner_id=$1 AND t.state='open' ORDER BY due_at NULLS LAST`, current(r).ID)
}
func (a *app) completeTask(w http.ResponseWriter, r *http.Request) {
	var id string
	err := a.db.QueryRowContext(r.Context(), `UPDATE action_tasks SET state='completed',completed_at=now() WHERE id=$1 AND owner_id=$2 AND state='open' AND task_type IN('meeting_action','meeting_follow_up') RETURNING id`, r.PathValue("id"), current(r).ID).Scan(&id)
	if err != nil {
		problem(w, 409, "Task not completed", "Academic approvals and leave-return decisions must use their own workflow.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "completed"})
}
func (a *app) returnFromLeave(w http.ResponseWriter, r *http.Request) {
	if !a.canManageEnrolment(r.Context(), current(r), r.PathValue("id")) {
		problem(w, 403, "Access denied", "Enrolment outside your scope.")
		return
	}
	var in struct{ Reason string }
	if !decode(w, r, &in) {
		return
	}
	if len(in.Reason) < 8 {
		problem(w, 400, "Reason required", "Record the return confirmation.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `UPDATE enrolments SET state='active' WHERE id=$1 AND state='approved_leave' RETURNING id`, r.PathValue("id")).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE action_tasks SET state='completed',completed_at=now() WHERE enrolment_id=$1 AND task_type='leave_return' AND state='open'`, id)
	}
	if err == nil {
		err = auditTx(r.Context(), tx, current(r).ID, "enrolment", id, "leave_return", in.Reason)
	}
	commitResponse(w, tx, err, map[string]string{"status": "active"})
}

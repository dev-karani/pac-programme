package main

import (
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// Public registration only creates a self-scoped student awaiting academic verification.
// Continuing students cannot self-certify previously completed work.
func (a *app) signup(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Email, Password, StudentNumber, ProgrammeID, CohortID, StudyMode, AdmissionDate, EntryPath string }
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.StudentNumber = strings.TrimSpace(in.StudentNumber)
	address, emailErr := mail.ParseAddress(in.Email)
	date, dateErr := time.Parse("2006-01-02", in.AdmissionDate)
	if emailErr != nil || address.Address != in.Email || len(in.Name) < 3 || len(in.Name) > 200 || len(in.Password) < 12 || len(in.Password) > 72 || len(in.StudentNumber) < 3 || len(in.StudentNumber) > 80 || dateErr != nil || date.After(time.Now()) || (in.EntryPath != "new" && in.EntryPath != "continuing") || (in.StudyMode != "full_time" && in.StudyMode != "part_time") {
		problem(w, 400, "Check registration details", "Use your admission details, a valid email and a password of 12–72 characters. Choose new or continuing student.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		problem(w, 500, "Registration unavailable", "Please try again.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Registration unavailable", "Please try again.")
		return
	}
	defer tx.Rollback()
	var id, eid string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO users(email,full_name,password_hash,student_number,must_change_password) VALUES($1,$2,$3,$4,false) RETURNING id`, in.Email, in.Name, string(hash), in.StudentNumber).Scan(&id)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO role_assignments(user_id,role,scope_type,scope_id) VALUES($1,'student','self',$1)`, id)
	}
	if err == nil {
		err = tx.QueryRowContext(r.Context(), `INSERT INTO enrolments(student_id,programme_id,cohort_id,study_mode,admission_date,verification_status,entry_path) SELECT $1,c.programme_id,c.id,$4,$5,'submitted',$6 FROM cohorts c WHERE c.id=$3 AND c.programme_id=$2 RETURNING id`, id, in.ProgrammeID, in.CohortID, in.StudyMode, in.AdmissionDate, in.EntryPath).Scan(&eid)
	}
	if err == nil {
		err = auditTx(r.Context(), tx, id, "enrolment", eid, "registration_submitted", "Student self-registration; academic verification required")
	}
	if err != nil {
		problem(w, 409, "Registration not completed", "Check your programme and cohort. If you already have an account, sign in or contact your programme office; do not register again.")
		return
	}
	if tx.Commit() != nil {
		problem(w, 503, "Registration unavailable", "Please try again.")
		return
	}
	writeJSON(w, 201, map[string]string{"status": "pending_verification", "message": "Account created. Sign in to track verification. Your programme office must verify your admission and any prior research progress."})
}

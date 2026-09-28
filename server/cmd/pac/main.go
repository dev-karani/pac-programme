package main

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
	"pac-progress/server/internal/seed"
)

type app struct {
	db        *sql.DB
	uploadDir string
	webDir    string
}
type user struct {
	ID, Email, Name string
	MustChange      bool
	Roles           []role
}
type role struct{ Name, ScopeType, ScopeID string }
type ctxKey string

const userKey ctxKey = "user"

func main() {
	db, err := sql.Open("pgx", mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatal("database: ", err)
	}
	if len(os.Args) > 1 && (os.Args[1] == "seed" || os.Args[1] == "seed-pitch") {
		query := seed.SQL
		if os.Args[1] == "seed-pitch" {
			query = seed.PitchSQL
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			log.Fatal("seed: ", err)
		}
		log.Println("demo data seeded")
		dir := env("UPLOAD_DIR", "./data/uploads")
		if err := os.MkdirAll(dir, 0700); err != nil {
			log.Fatal(err)
		}
		for _, key := range []string{"demo-examination-package", "demo-concept-document"} {
			file, err := os.OpenFile(filepath.Join(dir, key), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err == nil {
				_, err = file.Write(seed.DemoDocument)
				file.Close()
				if err != nil {
					log.Fatal(err)
				}
			} else if !os.IsExist(err) {
				log.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `UPDATE submission_versions SET size_bytes=$1 WHERE storage_key IN('demo-examination-package','demo-concept-document')`, len(seed.DemoDocument)); err != nil {
				log.Fatal(err)
			}
		}
		return
	}
	a := &app{db: db, uploadDir: env("UPLOAD_DIR", "./data/uploads"), webDir: env("WEB_DIR", "/app/web")}
	if err := os.MkdirAll(a.uploadDir, 0700); err != nil {
		log.Fatal(err)
	}
	go a.reminderLoop()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/signup", a.provisionedOnly)
	mux.HandleFunc("GET /api/v1/auth/programmes", a.catalog)
	mux.HandleFunc("POST /api/v1/auth/logout", a.withAuth(a.logout))
	mux.HandleFunc("GET /api/v1/me", a.withAuth(a.me))
	mux.HandleFunc("GET /api/v1/dashboard", a.withAuth(a.dashboard))
	mux.HandleFunc("GET /api/v1/milestones", a.withAuth(a.milestones))
	mux.HandleFunc("GET /api/v1/support-cases", a.withAuth(a.supportCases))
	mux.HandleFunc("POST /api/v1/support-cases", a.withAuth(a.createSupportCase))
	mux.HandleFunc("POST /api/v1/milestones/{id}/submit", a.withAuth(a.submitDocument))
	mux.HandleFunc("GET /api/v1/files/{id}", a.withAuth(a.downloadFile))
	a.registerReleaseRoutes(mux)
	a.registerWorkspaceRoutes(mux)
	a.registerAdminWorkflows(mux)
	a.registerCoordination(mux)
	a.registerSettings(mux)
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.Handle("/", spa(a.webDir))
	srv := &http.Server{Addr: env("APP_ADDR", ":8080"), Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	log.Printf("PAC Progress listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		problem(w, 400, "Invalid request", "Enter an email and password.")
		return
	}
	var u user
	var hash string
	var active bool
	err := a.db.QueryRowContext(r.Context(), `SELECT id,email,full_name,password_hash,must_change_password,active FROM users WHERE lower(email)=lower($1)`, strings.TrimSpace(in.Email)).Scan(&u.ID, &u.Email, &u.Name, &hash, &u.MustChange, &active)
	if err != nil || !active || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		problem(w, 401, "Sign in failed", "The email or password is incorrect.")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		problem(w, 500, "Sign in failed", "Please try again.")
		return
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	if _, err := a.db.ExecContext(r.Context(), `INSERT INTO sessions(user_id,token_hash,expires_at) VALUES($1,$2,$3)`, u.ID, sum[:], time.Now().Add(12*time.Hour)); err != nil {
		problem(w, 500, "Sign in failed", "Please try again.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "pac_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, MaxAge: 43200})
	a.loadRoles(r.Context(), &u)
	writeJSON(w, 200, u)
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("pac_session")
	if c != nil {
		s := sha256.Sum256([]byte(c.Value))
		_, _ = a.db.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=now() WHERE token_hash=$1`, s[:])
	}
	http.SetCookie(w, &http.Cookie{Name: "pac_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(204)
}

func (a *app) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("pac_session")
		if err != nil {
			problem(w, 401, "Sign in required", "Please sign in to continue.")
			return
		}
		s := sha256.Sum256([]byte(c.Value))
		var u user
		err = a.db.QueryRowContext(r.Context(), `SELECT u.id,u.email,u.full_name,u.must_change_password FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND u.active`, s[:]).Scan(&u.ID, &u.Email, &u.Name, &u.MustChange)
		if err != nil {
			problem(w, 401, "Session expired", "Please sign in again.")
			return
		}
		a.loadRoles(r.Context(), &u)
		if u.MustChange && r.URL.Path != "/api/v1/auth/change-password" && r.URL.Path != "/api/v1/auth/logout" && r.URL.Path != "/api/v1/me" {
			problem(w, 403, "Password change required", "Change your temporary password before continuing.")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}

func (a *app) loadRoles(ctx context.Context, u *user) {
	rows, err := a.db.QueryContext(ctx, `SELECT role,scope_type,COALESCE(scope_id::text,'') FROM role_assignments WHERE user_id=$1 ORDER BY role`, u.ID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var x role
		if rows.Scan(&x.Name, &x.ScopeType, &x.ScopeID) == nil {
			u.Roles = append(u.Roles, x)
		}
	}
}
func current(r *http.Request) user { return r.Context().Value(userKey).(user) }
func hasRole(u user, n string) bool {
	for _, r := range u.Roles {
		if r.Name == n {
			return true
		}
	}
	return false
}

func (a *app) me(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, current(r)) }
func (a *app) dashboard(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if !hasRole(u, "student") {
		a.staffDashboard(w, r, u)
		return
	}
	var d struct {
		EnrolmentID, Programme, Cohort, State, PlannedCompletion string
		Completed, Total, Unread                                 int
	}
	err := a.db.QueryRowContext(r.Context(), `SELECT e.id,p.name,c.name,e.state::text,COALESCE(to_char(sp.planned_completion,'DD Mon YYYY'),'Pending verification'),count(mi.id) FILTER (WHERE mi.state='approved'),count(mi.id),(SELECT count(*) FROM notifications n WHERE n.recipient_id=e.student_id AND n.read_at IS NULL AND n.dismissed_at IS NULL) FROM enrolments e JOIN programmes p ON p.id=e.programme_id JOIN cohorts c ON c.id=e.cohort_id LEFT JOIN student_plans sp ON sp.enrolment_id=e.id LEFT JOIN milestone_instances mi ON mi.plan_id=sp.id WHERE e.student_id=$1 AND e.state IN ('active','approved_leave','pending_verification') GROUP BY e.id,p.name,c.name,e.state,sp.planned_completion`, u.ID).Scan(&d.EnrolmentID, &d.Programme, &d.Cohort, &d.State, &d.PlannedCompletion, &d.Completed, &d.Total, &d.Unread)
	if err != nil {
		problem(w, 404, "No active programme", "Contact your programme coordinator.")
		return
	}
	rows, _ := a.db.QueryContext(r.Context(), `SELECT t.id,t.title,t.task_type,COALESCE(to_char(t.due_at AT TIME ZONE 'Africa/Nairobi','DD Mon YYYY, HH24:MI'),'No date'),CASE WHEN t.due_at<now() THEN 'overdue' WHEN t.due_at<now()+interval '4 days' THEN 'due_soon' ELSE 'upcoming' END,m.id,md.title FROM action_tasks t LEFT JOIN milestone_instances m ON m.id=t.milestone_id LEFT JOIN milestone_definitions md ON md.id=m.definition_id WHERE t.owner_id=$1 AND t.state='open' ORDER BY t.due_at NULLS LAST`, u.ID)
	defer closeRows(rows)
	actions := []map[string]any{}
	if rows != nil {
		for rows.Next() {
			var id, title, typ, due, health, mid, milestone string
			rows.Scan(&id, &title, &typ, &due, &health, &mid, &milestone)
			actions = append(actions, map[string]any{"id": id, "title": title, "type": typ, "due": due, "health": health, "milestoneId": mid, "milestone": milestone})
		}
	}
	writeJSON(w, 200, map[string]any{"enrolment": d, "actions": actions})
}

func (a *app) staffDashboard(w http.ResponseWriter, r *http.Request, u user) {
	rows, err := a.jsonRows(r.Context(), studentReport, u.ID)
	if err != nil {
		problem(w, 500, "Dashboard unavailable", "Please try again.")
		return
	}
	reviews, cases := 0, 0
	for _, row := range rows {
		reviews += int(row["overdue_reviews"].(float64))
		cases += int(row["open_cases"].(float64))
	}
	writeJSON(w, 200, map[string]any{"staff": true, "counts": map[string]int{"students": len(rows), "overdueReviews": reviews, "openCases": cases}, "actions": []any{}})
}

func (a *app) milestones(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT mi.id,md.title,md.description,md.stage_label,to_char(mi.current_start,'DD Mon YYYY'),to_char(mi.current_due,'DD Mon YYYY'),mi.state::text,mi.baseline_due<>mi.current_due FROM enrolments e JOIN student_plans sp ON sp.enrolment_id=e.id JOIN milestone_instances mi ON mi.plan_id=sp.id JOIN milestone_definitions md ON md.id=mi.definition_id WHERE e.student_id=$1 ORDER BY md.position`, u.ID)
	if err != nil {
		problem(w, 403, "Access denied", "This academic workspace is not assigned to you.")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, title, desc, stage, start, due, state string
		var revised bool
		rows.Scan(&id, &title, &desc, &stage, &start, &due, &state, &revised)
		out = append(out, map[string]any{"id": id, "title": title, "description": desc, "stage": stage, "start": start, "due": due, "state": state, "revised": revised})
	}
	writeJSON(w, 200, out)
}

func (a *app) supportCases(w http.ResponseWriter, r *http.Request) {
	a.sendRows(w, r, `SELECT sc.id,sc.category,sc.summary,sc.restricted,sc.status,COALESCE(to_char(sc.next_follow_up,'DD Mon YYYY'),'Not set') AS "followUp",COALESCE(owner.full_name,'Unassigned') AS owner FROM support_cases sc JOIN enrolments e ON e.id=sc.enrolment_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id LEFT JOIN users owner ON owner.id=sc.owner_id WHERE e.student_id=$1 OR sc.owner_id=$1 OR EXISTS(SELECT 1 FROM case_access_grants g WHERE g.case_id=sc.id AND g.user_id=$1) OR (NOT sc.restricted AND `+academicScope+`) ORDER BY sc.created_at DESC`, current(r).ID)
}
func (a *app) createSupportCase(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct {
		Category, Summary string
		Restricted        bool
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in) != nil || len(strings.TrimSpace(in.Summary)) < 10 {
		problem(w, 400, "Check the request", "Please provide a summary of at least 10 characters.")
		return
	}
	var id string
	in.Restricted = in.Restricted || in.Category == "personal/welfare"
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO support_cases(enrolment_id,category,summary,restricted,status,owner_id) SELECT e.id,$2,$3,$4,'open',(SELECT ra.user_id FROM role_assignments ra JOIN users u ON u.id=ra.user_id AND u.active WHERE ($4 AND ra.role='support') OR (NOT $4 AND ra.role='coordinator' AND ra.scope_type='programme' AND ra.scope_id=e.programme_id) ORDER BY ra.user_id LIMIT 1) FROM enrolments e WHERE e.student_id=$1 AND e.state IN ('active','approved_leave','pending_verification') RETURNING id`, u.ID, in.Category, strings.TrimSpace(in.Summary), in.Restricted).Scan(&id)
	if err != nil {
		problem(w, 500, "Request not saved", "Please try again.")
		return
	}
	a.db.ExecContext(r.Context(), `INSERT INTO audit_events(actor_id,entity_type,entity_id,action,safe_summary) VALUES($1,'support_case',$2,'created','Support request created')`, u.ID, id)
	writeJSON(w, 201, map[string]string{"id": id, "status": "open"})
}

func (a *app) submitDocument(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	r.Body = http.MaxBytesReader(w, r.Body, 26<<20)
	if err := r.ParseMultipartForm(26 << 20); err != nil {
		problem(w, 400, "Upload failed", "The file must be 25 MB or smaller.")
		return
	}
	f, h, err := r.FormFile("document")
	if err != nil {
		problem(w, 400, "Choose a file", "Select a PDF, DOCX or XLSX document.")
		return
	}
	defer f.Close()
	if !allowedExtension(h) {
		problem(w, 400, "Unsupported file", "Use PDF, DOCX or XLSX.")
		return
	}
	key := uuid.NewString()
	path := filepath.Join(a.uploadDir, key)
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		problem(w, 500, "Upload failed", "Please try again.")
		return
	}
	n, copyErr := io.Copy(dst, f)
	dst.Close()
	if copyErr != nil {
		os.Remove(path)
		problem(w, 500, "Upload failed", "Please try again.")
		return
	}
	if !validFileType(path, filepath.Ext(h.Filename)) {
		os.Remove(path)
		problem(w, 400, "File does not match its extension", "Use an authentic PDF, DOCX or XLSX document.")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		os.Remove(path)
		problem(w, 500, "Submission failed", "Please try again.")
		return
	}
	defer tx.Rollback()
	mid := r.PathValue("id")
	idem := r.Header.Get("Idempotency-Key")
	if idem == "" {
		idem = uuid.NewString()
	}
	var subID string
	var ver int
	err = tx.QueryRowContext(r.Context(), `INSERT INTO submissions(milestone_id,student_id,current_version,state,approval_mode) SELECT $1,$2,1,'submitted',md.approval_mode FROM milestone_instances mi JOIN milestone_definitions md ON md.id=mi.definition_id JOIN student_plans sp ON sp.id=mi.plan_id JOIN enrolments e ON e.id=sp.enrolment_id WHERE mi.id=$1 AND e.student_id=$2 AND e.state='active' AND mi.state IN('in_progress','changes_requested') AND NOT EXISTS(SELECT 1 FROM milestone_dependencies dep JOIN milestone_definitions required ON required.id=dep.depends_on_id WHERE dep.milestone_id=mi.definition_id AND NOT EXISTS(SELECT 1 FROM milestone_instances pre JOIN milestone_definitions pd ON pd.id=pre.definition_id WHERE pre.plan_id=mi.plan_id AND pd.position=required.position AND pre.state='approved')) ON CONFLICT(milestone_id,student_id) DO UPDATE SET current_version=submissions.current_version+1,state='submitted' WHERE submissions.state='changes_requested' RETURNING id,current_version`, mid, u.ID).Scan(&subID, &ver)
	if err != nil {
		os.Remove(path)
		problem(w, 403, "Submission not allowed", "This milestone is not available to you.")
		return
	}
	var vid string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO submission_versions(submission_id,version,storage_key,original_name,content_type,size_bytes,submitted_at,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,now(),$7) RETURNING id`, subID, ver, key, filepath.Base(h.Filename), h.Header.Get("Content-Type"), n, idem).Scan(&vid)
	if err != nil {
		os.Remove(path)
		if strings.Contains(err.Error(), "duplicate") {
			problem(w, 409, "Already submitted", "This submission request was already received.")
			return
		}
		problem(w, 500, "Submission failed", "Please try again.")
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE milestone_instances SET state='awaiting_review',version=version+1 WHERE id=$1`, mid)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE action_tasks SET state='completed',completed_at=now() WHERE milestone_id=$1 AND owner_id=$2 AND task_type='submission' AND state='open'`, mid, u.ID)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO audit_events(actor_id,entity_type,entity_id,action,safe_summary) VALUES($1,'submission_version',$2,'submitted','Academic document submitted')`, u.ID, vid)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO review_assignments(submission_version_id,reviewer_id,due_at)
			SELECT $1,sa.supervisor_id,now()+(p.review_days||' days')::interval
			FROM milestone_instances mi JOIN student_plans sp ON sp.id=mi.plan_id JOIN enrolments e ON e.id=sp.enrolment_id JOIN programmes p ON p.id=e.programme_id
			JOIN supervision_assignments sa ON sa.enrolment_id=e.id AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now())
			JOIN submissions s ON s.id=$3
			WHERE mi.id=$2 AND (s.approval_mode IN ('either','both') OR sa.position='primary')
			ORDER BY CASE sa.position WHEN 'primary' THEN 0 ELSE 1 END,sa.supervisor_id`, vid, mid, subID)
	}
	if err == nil {
		var reviewers int
		err = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM review_assignments WHERE submission_version_id=$1`, vid).Scan(&reviewers)
		var mode string
		if err == nil {
			err = tx.QueryRowContext(r.Context(), `SELECT approval_mode FROM submissions WHERE id=$1`, subID).Scan(&mode)
		}
		if err == nil && (reviewers == 0 || mode == "both" && reviewers < 2) {
			os.Remove(path)
			problem(w, 409, "Supervisor allocation required", "A primary supervisor must be confirmed before submitting academic work.")
			return
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO notifications(recipient_id,dedupe_key,title,urgency) SELECT reviewer_id,'submission:'||$1::text,'New academic submission awaiting review','info' FROM review_assignments WHERE submission_version_id=$1::uuid ON CONFLICT DO NOTHING`, vid)
	}
	if err != nil {
		os.Remove(path)
		problem(w, 500, "Submission failed", "Please try again.")
		return
	}
	if err = tx.Commit(); err != nil {
		os.Remove(path)
		problem(w, 409, "Submission changed", "Refresh and try again.")
		return
	}
	writeJSON(w, 201, map[string]any{"receipt": fmt.Sprintf("PAC-%s-V%d", strings.ToUpper(subID[:8]), ver), "version": ver, "submittedAt": time.Now().UTC()})
}
func (a *app) downloadFile(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var key, name string
	err := a.db.QueryRowContext(r.Context(), `SELECT sv.storage_key,sv.original_name FROM submission_versions sv JOIN submissions s ON s.id=sv.submission_id JOIN milestone_instances mi ON mi.id=s.milestone_id JOIN student_plans sp ON sp.id=mi.plan_id JOIN enrolments e ON e.id=sp.enrolment_id JOIN programmes p ON p.id=e.programme_id JOIN departments d ON d.id=p.department_id WHERE sv.id=$1 AND (e.student_id=$2 OR EXISTS(SELECT 1 FROM supervision_assignments sa WHERE sa.enrolment_id=e.id AND sa.supervisor_id=$2 AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now())) OR EXISTS(SELECT 1 FROM defence_events de JOIN committee_members cm ON cm.committee_id=de.committee_id WHERE de.submission_version_id=sv.id AND cm.user_id=$2) OR EXISTS(SELECT 1 FROM role_assignments r WHERE r.user_id=$2 AND r.role IN('coordinator','hod','dean','leadership') AND ((r.scope_type='programme' AND r.scope_id=p.id) OR (r.scope_type='department' AND r.scope_id=d.id) OR (r.scope_type='school' AND r.scope_id=d.school_id) OR r.scope_type='institution')))`, r.PathValue("id"), u.ID).Scan(&key, &name)
	if err != nil {
		problem(w, 404, "File not found", "The file is unavailable or outside your access.")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	http.ServeFile(w, r, filepath.Join(a.uploadDir, key))
}
func (a *app) uploadSimilarity(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	r.Body = http.MaxBytesReader(w, r.Body, 26<<20)
	if r.ParseMultipartForm(26<<20) != nil {
		problem(w, 400, "Upload failed", "The report must be 25 MB or smaller.")
		return
	}
	f, h, err := r.FormFile("report")
	if err != nil || strings.ToLower(filepath.Ext(h.Filename)) != ".pdf" {
		problem(w, 400, "Choose a PDF report", "Attach the external similarity report as a PDF.")
		return
	}
	defer f.Close()
	var allowed bool
	_ = a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM submission_versions sv JOIN submissions s ON s.id=sv.submission_id LEFT JOIN review_assignments ra ON ra.submission_version_id=sv.id AND ra.reviewer_id=$2 WHERE sv.id=$1 AND (s.student_id=$2 OR (ra.id IS NOT NULL AND EXISTS(SELECT 1 FROM milestone_instances mi JOIN student_plans sp ON sp.id=mi.plan_id JOIN supervision_assignments sa ON sa.enrolment_id=sp.enrolment_id WHERE mi.id=s.milestone_id AND sa.supervisor_id=$2 AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now())))))`, r.PathValue("id"), u.ID).Scan(&allowed)
	if !allowed {
		problem(w, 403, "Access denied", "This document version is outside your academic workspace.")
		return
	}
	key := uuid.NewString()
	path := filepath.Join(a.uploadDir, key)
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return
	}
	n, copyErr := io.Copy(dst, f)
	dst.Close()
	if copyErr != nil || !validFileType(path, ".pdf") {
		os.Remove(path)
		problem(w, 400, "Invalid report", "The file is not a valid PDF.")
		return
	}
	var percentage float64
	if r.FormValue("percentage") != "" {
		_, _ = fmt.Sscanf(r.FormValue("percentage"), "%f", &percentage)
	}
	var id string
	err = a.db.QueryRowContext(r.Context(), `INSERT INTO similarity_evidence(submission_version_id,storage_key,original_name,percentage,uploaded_by) VALUES($1,$2,$3,$4,$5) ON CONFLICT(submission_version_id) DO UPDATE SET storage_key=excluded.storage_key,original_name=excluded.original_name,percentage=excluded.percentage,uploaded_by=excluded.uploaded_by,uploaded_at=now(),verified_by=NULL,verified_at=NULL,decision=NULL RETURNING id`, r.PathValue("id"), key, filepath.Base(h.Filename), percentage, u.ID).Scan(&id)
	if err != nil {
		os.Remove(path)
		problem(w, 500, "Report not saved", "Try again.")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "size": n, "status": "awaiting_verification"})
}
func (a *app) verifySimilarity(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	var in struct{ Decision, Reason string }
	if !decode(w, r, &in) {
		return
	}
	res, err := a.db.ExecContext(r.Context(), `UPDATE similarity_evidence se SET verified_by=$2,verified_at=now(),decision=$3,exception_reason=NULLIF($4,'') WHERE se.id=$1 AND EXISTS(SELECT 1 FROM review_assignments ra JOIN submission_versions sv ON sv.id=ra.submission_version_id JOIN submissions s ON s.id=sv.submission_id JOIN milestone_instances mi ON mi.id=s.milestone_id JOIN student_plans sp ON sp.id=mi.plan_id JOIN supervision_assignments sa ON sa.enrolment_id=sp.enrolment_id AND sa.supervisor_id=$2 AND sa.effective_from<=now() AND (sa.effective_to IS NULL OR sa.effective_to>now()) WHERE ra.submission_version_id=se.submission_version_id AND ra.reviewer_id=$2)`, r.PathValue("id"), u.ID, in.Decision, in.Reason)
	var n int64
	if res != nil {
		n, _ = res.RowsAffected()
	}
	if err != nil || n != 1 {
		problem(w, 403, "Verification not recorded", "You are not an assigned reviewer for this version.")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "verified"})
}

func (a *app) downloadSimilarity(w http.ResponseWriter, r *http.Request) {
	var eid, key, name string
	err := a.db.QueryRowContext(r.Context(), `SELECT sp.enrolment_id,se.storage_key,se.original_name FROM similarity_evidence se JOIN submission_versions sv ON sv.id=se.submission_version_id JOIN submissions s ON s.id=sv.submission_id JOIN milestone_instances mi ON mi.id=s.milestone_id JOIN student_plans sp ON sp.id=mi.plan_id WHERE se.id=$1`, r.PathValue("id")).Scan(&eid, &key, &name)
	if err != nil || !a.academicAccess(r.Context(), current(r), eid) {
		problem(w, 404, "Report unavailable", "Outside your academic access.")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	http.ServeFile(w, r, filepath.Join(a.uploadDir, key))
}

func allowedExtension(h *multipart.FileHeader) bool {
	ext := strings.ToLower(filepath.Ext(h.Filename))
	return ext == ".pdf" || ext == ".docx" || ext == ".xlsx"
}
func validFileType(path, ext string) bool {
	ext = strings.ToLower(ext)
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 8)
	n, _ := io.ReadFull(f, head)
	if ext == ".pdf" {
		return n >= 5 && string(head[:5]) == "%PDF-"
	}
	if n < 4 || string(head[:2]) != "PK" {
		return false
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer z.Close()
	prefix := "word/"
	if ext == ".xlsx" {
		prefix = "xl/"
	}
	for _, entry := range z.File {
		if strings.HasPrefix(entry.Name, prefix) {
			return true
		}
	}
	return false
}
func closeRows(r *sql.Rows) {
	if r != nil {
		r.Close()
	}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, title, detail string) {
	writeJSON(w, status, map[string]any{"type": "about:blank", "title": title, "detail": detail, "status": status})
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
func spa(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatal(errors.New(k + " is required"))
	}
	return v
}

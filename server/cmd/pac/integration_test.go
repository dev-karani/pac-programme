package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func integrationApp(t *testing.T) *app {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}
	return &app{db: db, uploadDir: t.TempDir()}
}

func loadTestUser(t *testing.T, a *app, email string) user {
	t.Helper()
	var u user
	if err := a.db.QueryRow(`SELECT id,email,full_name,must_change_password FROM users WHERE email=$1`, email).Scan(&u.ID, &u.Email, &u.Name, &u.MustChange); err != nil {
		t.Fatal(err)
	}
	a.loadRoles(context.Background(), &u)
	return u
}

func TestDepartmentAndSchoolBoundaries(t *testing.T) {
	a := integrationApp(t)
	dean := loadTestUser(t, a, "dean@demo.pac.test")
	hod := loadTestUser(t, a, "hod@demo.pac.test")
	leader := loadTestUser(t, a, "leadership@demo.pac.test")
	schoolB := "60000000-0000-0000-0000-000000000014"
	schoolA := "60000000-0000-0000-0000-000000000001"
	if a.canManageEnrolment(context.Background(), dean, schoolB) {
		t.Fatal("School A dean gained School B access")
	}
	if a.canManageEnrolment(context.Background(), hod, schoolB) {
		t.Fatal("Department A HOD gained School B access")
	}
	if !a.canManageEnrolment(context.Background(), dean, schoolA) {
		t.Fatal("School A dean lost in-scope access")
	}
	if !a.canManageEnrolment(context.Background(), leader, schoolB) {
		t.Fatal("institution leadership should see both schools")
	}
}

func TestReminderProcessingIsIdempotent(t *testing.T) {
	a := integrationApp(t)
	ctx := context.Background()
	if _, err := a.processReminders(ctx); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := a.db.QueryRow(`SELECT count(*) FROM notifications WHERE dedupe_key LIKE 'deadline:%'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := a.processReminders(ctx); err != nil {
		t.Fatal(err)
	}
	var after int
	_ = a.db.QueryRow(`SELECT count(*) FROM notifications WHERE dedupe_key LIKE 'deadline:%'`).Scan(&after)
	if before != after {
		t.Fatalf("duplicate reminders: before=%d after=%d", before, after)
	}
}

func TestRestrictedCaseNarrativeIsRedacted(t *testing.T) {
	a := integrationApp(t)
	dean := loadTestUser(t, a, "dean@demo.pac.test")
	r := httptest.NewRequest("GET", "/api/v1/support-cases", nil).WithContext(context.WithValue(context.Background(), userKey, dean))
	w := httptest.NewRecorder()
	a.supportCases(w, r)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row["summary"].(string)), "welfare narrative") {
			t.Fatal("restricted welfare narrative leaked to dean")
		}
	}
}

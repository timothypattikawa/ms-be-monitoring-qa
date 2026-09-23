package repository

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMonitoringModelInventory(t *testing.T) {
	models := Models()
	if len(models) != 12 {
		t.Fatalf("got %d monitoring models, want 12", len(models))
	}
	seen := map[reflect.Type]bool{}
	for _, model := range models {
		typ := reflect.TypeOf(model)
		if typ.Kind() != reflect.Pointer || seen[typ] {
			t.Fatalf("model inventory contains invalid or duplicate type %v", typ)
		}
		id, ok := typ.Elem().FieldByName("ID")
		if !ok || strings.Contains(id.Tag.Get("gorm"), "type:uuid") {
			t.Fatalf("%v must keep string IDs compatible with relation columns", typ)
		}
		seen[typ] = true
	}
}

func TestProjectPersistenceContract(t *testing.T) {
	typ := reflect.TypeOf(Project{})
	want := map[string]reflect.Type{
		"JiraInitID": reflect.TypeOf((*string)(nil)), "JiraInitKey": reflect.TypeOf(""),
		"QaseTestRunID": reflect.TypeOf(int64(0)), "QAOwner": reflect.TypeOf(""),
		"StagingStartAt": reflect.TypeOf((*time.Time)(nil)), "StagingEndAt": reflect.TypeOf((*time.Time)(nil)),
		"BetaStartAt": reflect.TypeOf((*time.Time)(nil)), "BetaEndAt": reflect.TypeOf((*time.Time)(nil)),
	}
	for name, fieldType := range want {
		field, ok := typ.FieldByName(name)
		if !ok || field.Type != fieldType {
			t.Fatalf("Project.%s type = %v, want %v", name, field.Type, fieldType)
		}
	}
	key, _ := typ.FieldByName("JiraInitKey")
	if !strings.Contains(key.Tag.Get("gorm"), "uniqueIndex") {
		t.Fatal("JiraInitKey must have a database unique index")
	}
}

func TestWorkflowQueryIsCumulativeRunState(t *testing.T) {
	for _, required := range []string{
		"count(*) FROM qase_run_cases", "p.qase_test_run_id", "DISTINCT ON (r.case_id)",
		"SELECT DISTINCT date_trunc('day', r.ended_at)", "r.ended_at < d.day + interval '1 day'", "p.total AS total",
	} {
		if !strings.Contains(workflowQuery, required) {
			t.Fatalf("workflow query lost cumulative membership semantics: missing %q", required)
		}
	}
	if strings.Contains(workflowQuery, "generate_series") {
		t.Fatal("workflow must not invent calendar days without Qase activity")
	}
}

func TestNormalizeQaseEnvironment(t *testing.T) {
	tests := map[string]string{
		"staging":   "STAGING",
		"STG":       "STAGING",
		"stg-ios":   "STAGING",
		"[STG] AOS": "STAGING",
		"beta ios":  "BETA",
		"uat":       "UAT",
		"":          "",
	}
	for input, want := range tests {
		if got := normalizeQaseEnvironment(input); got != want {
			t.Errorf("normalizeQaseEnvironment(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMemberActiveFilterRoundTrips(t *testing.T) {
	repo := NewSQLiteForTest(t)
	if err := repo.SaveMember(&Member{ID: "m1", Name: "Nadia Putri", WeeklyCapacityHours: 40, Active: true}); err != nil {
		t.Fatalf("save active member: %v", err)
	}
	if err := repo.SaveMember(&Member{ID: "m2", Name: "Old Member", WeeklyCapacityHours: 40, Active: false}); err != nil {
		t.Fatalf("save inactive member: %v", err)
	}
	active, err := repo.Members(false)
	if err != nil {
		t.Fatalf("list active members: %v", err)
	}
	if len(active) != 1 || active[0].ID != "m1" {
		t.Fatalf("expected only m1 in active list, got %+v", active)
	}
	all, err := repo.Members(true)
	if err != nil {
		t.Fatalf("list all members: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 members total, got %d", len(all))
	}
	got, err := repo.Member("m2")
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if got.Name != "Old Member" || got.Active {
		t.Fatalf("unexpected member: %+v", got)
	}

	// Guards the exact bug the Active field's gorm tag was chosen to avoid:
	// there is no DB-level default, so an unset Active must persist as false,
	// not silently flip to true.
	if err := repo.SaveMember(&Member{ID: "m3", Name: "No Explicit Active"}); err != nil {
		t.Fatalf("save member with unset active: %v", err)
	}
	m3, err := repo.Member("m3")
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if m3.Active {
		t.Fatalf("expected Active to persist as false when left unset, got %+v", m3)
	}
}

func TestMemberDisplayNameFallsBackToStableID(t *testing.T) {
	if got := memberDisplayName(Member{ID: "qase-member-42"}); got != "qase-member-42" {
		t.Fatalf("empty display name returned %q", got)
	}
	if got := memberDisplayName(Member{ID: "42", Name: "  Kiki  "}); got != "Kiki" {
		t.Fatalf("named member returned %q", got)
	}
}

package repository

import (
	"testing"
	"time"
)

func timeAt(year int, month time.Month, day, hour int) *time.Time {
	t := time.Date(year, month, day, hour, 0, 0, 0, wib)
	return &t
}

// STG window 2026-08-24 (Mon) .. 2026-09-01 (Tue): 9 calendar days,
// 2 weekend days (Aug 29-30), 1 national holiday (Aug 25 Maulid — a weekday,
// and deliberately NOT the Aug 26-28 cuti bersama range) → 6 working days.
// A BETA run overlapping the window must not extend it; an open STG run with
// no run timestamps still extends End QA via its latest result.
func TestQaTimelinesWorkingDayBreakdown(t *testing.T) {
	repo := NewSQLiteForTest(t)
	if err := repo.SeedNationalHolidays(); err != nil {
		t.Fatal(err)
	}
	project := Project{ID: "p1", JiraInitKey: "INIT-1", Name: "Demo", QaseProjectCode: "P1", Status: "active", QaseTotalCases: 42}
	if err := repo.SaveProject(&project); err != nil {
		t.Fatal(err)
	}
	runs := []QaseRun{
		{ID: "r1", ProjectCode: "P1", RunID: 1, Title: "[STG] AOS", Active: true, StartedAt: timeAt(2026, 8, 24, 9), FinishedAt: timeAt(2026, 8, 28, 17)},
		{ID: "r2", ProjectCode: "P1", RunID: 2, Title: "[BETA] iOS", Active: true, StartedAt: timeAt(2026, 8, 20, 9), FinishedAt: timeAt(2026, 8, 21, 17)},
		{ID: "r3", ProjectCode: "P1", RunID: 3, Title: "[STG] Web", Active: true},
		{ID: "r4", ProjectCode: "P1", RunID: 4, Title: "Regression", Active: true, StartedAt: timeAt(2026, 8, 10, 9), FinishedAt: timeAt(2026, 9, 20, 17)},
	}
	for i := range runs {
		if err := repo.db.Save(&runs[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	result := QaseResult{ID: "res1", ProjectCode: "P1", ResultID: "h1", RunID: 3, CaseID: 7, Status: "passed", StartedAt: timeAt(2026, 8, 31, 14), EndedAt: timeAt(2026, 9, 1, 10)}
	if err := repo.db.Save(&result).Error; err != nil {
		t.Fatal(err)
	}

	rows, err := repo.QaTimelines()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 timeline row, got %d", len(rows))
	}
	row := rows[0]
	if row.QAStartAt == nil || row.QAStartAt.Format("2006-01-02") != "2026-08-24" {
		t.Fatalf("qaStartAt = %v, want 2026-08-24", row.QAStartAt)
	}
	if row.QAEndAt == nil || row.QAEndAt.Format("2006-01-02") != "2026-09-01" {
		t.Fatalf("qaEndAt = %v, want 2026-09-01", row.QAEndAt)
	}
	if row.CalendarDays != 9 || row.WeekendDays != 2 || row.HolidayDays != 1 || row.WorkingDays != 6 {
		t.Fatalf("breakdown = %d/%d/%d/%d, want 9/2/1/6", row.CalendarDays, row.WeekendDays, row.HolidayDays, row.WorkingDays)
	}
	if len(row.Holidays) != 1 || row.Holidays[0].Date != "2026-08-25" {
		t.Fatalf("holidays = %+v, want 2026-08-25 Maulid", row.Holidays)
	}
	if row.TotalScenarios != 42 {
		t.Fatalf("totalScenarios = %d, want 42", row.TotalScenarios)
	}
}

func TestWorkingDayBreakdownIncludesBothEnds(t *testing.T) {
	_, _, _, working, _ := workingDayBreakdown(*timeAt(2026, 8, 24, 0), *timeAt(2026, 8, 24, 0), nil)
	if working != 1 {
		t.Fatalf("single Monday must count as 1 working day, got %d", working)
	}
	sat, sun := timeAt(2026, 8, 29, 0), timeAt(2026, 8, 30, 0)
	cal, weekend, holiday, working, _ := workingDayBreakdown(*sat, *sun, map[string]NationalHoliday{})
	if working != 0 || weekend != 2 || cal != 2 {
		t.Fatalf("weekend span counted wrong: %d/%d/%d/%d", cal, weekend, holiday, working)
	}

	// Aug 24 (Mon) .. Sep 1 (Tue) 2026 = 9 days, Aug 29-30 weekend, Aug 25
	// Maulid on a weekday → 6 working days. A holiday on a weekend must not
	// be subtracted twice.
	holidays := map[string]NationalHoliday{
		"2026-08-25": {Date: *timeAt(2026, 8, 25, 0), Name: "Maulid Nabi Muhammad"},
		"2026-08-29": {Date: *timeAt(2026, 8, 29, 0), Name: "Saturday fake holiday"},
	}
	cal, weekend, holiday, working, list := workingDayBreakdown(*timeAt(2026, 8, 24, 0), *timeAt(2026, 9, 1, 0), holidays)
	if cal != 9 || weekend != 2 || holiday != 1 || working != 6 {
		t.Fatalf("breakdown with holiday = %d/%d/%d/%d, want 9/2/1/6", cal, weekend, holiday, working)
	}
	if len(list) != 1 || list[0].Name != "Maulid Nabi Muhammad" {
		t.Fatalf("weekend holiday must not be listed twice: %+v", list)
	}
}

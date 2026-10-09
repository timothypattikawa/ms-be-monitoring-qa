package repository

import (
	"strconv"
	"time"

	"gorm.io/gorm/clause"
)

// NationalHoliday is one official Indonesian public holiday (tanggal merah),
// seeded from the SKB 3 Menteri decrees (1017/2/2-2024 for 2025,
// 1497/2/5-2025 for 2026). Joint leave days (cuti bersama) are deliberately
// NOT in this table — product rule: cuti bersama never reduces QA working
// days. Extend the seed list (or insert rows directly) for later years.
type NationalHoliday struct {
	Date time.Time `gorm:"primaryKey;type:date" json:"date"`
	Name string    `json:"name"`
}

// wib is Asia/Jakarta without depending on the OS tz database (Windows has
// none); WIB has no DST, so a fixed +07:00 zone is exact.
var wib = time.FixedZone("Asia/Jakarta", 7*60*60)

var nationalHolidaySeed = []NationalHoliday{
	// 2025 — SKB 3 Menteri 1017/2/2-2024
	{Date: holidayDate(2025, 1, 1), Name: "Tahun Baru 2025"},
	{Date: holidayDate(2025, 1, 27), Name: "Isra Mikraj Nabi Muhammad"},
	{Date: holidayDate(2025, 1, 29), Name: "Tahun Baru Imlek 2576"},
	{Date: holidayDate(2025, 3, 29), Name: "Hari Suci Nyepi (Saka 1947)"},
	{Date: holidayDate(2025, 3, 31), Name: "Idulfitri 1446 H"},
	{Date: holidayDate(2025, 4, 1), Name: "Idulfitri 1446 H"},
	{Date: holidayDate(2025, 4, 18), Name: "Wafat Yesus Kristus"},
	{Date: holidayDate(2025, 4, 20), Name: "Kebangkitan Yesus Kristus (Paskah)"},
	{Date: holidayDate(2025, 5, 1), Name: "Hari Buruh Internasional"},
	{Date: holidayDate(2025, 5, 12), Name: "Hari Raya Waisak 2569 BE"},
	{Date: holidayDate(2025, 5, 29), Name: "Kenaikan Yesus Kristus"},
	{Date: holidayDate(2025, 6, 1), Name: "Hari Lahir Pancasila"},
	{Date: holidayDate(2025, 6, 6), Name: "Iduladha 1446 H"},
	{Date: holidayDate(2025, 6, 27), Name: "Tahun Baru Islam 1447 H"},
	{Date: holidayDate(2025, 8, 17), Name: "Proklamasi Kemerdekaan RI"},
	{Date: holidayDate(2025, 9, 5), Name: "Maulid Nabi Muhammad"},
	{Date: holidayDate(2025, 12, 25), Name: "Kelahiran Yesus Kristus"},
	// 2026 — SKB 3 Menteri 1497/2/5-2025
	{Date: holidayDate(2026, 1, 1), Name: "Tahun Baru 2026"},
	{Date: holidayDate(2026, 1, 16), Name: "Isra Mikraj Nabi Muhammad"},
	{Date: holidayDate(2026, 2, 17), Name: "Tahun Baru Imlek 2577"},
	{Date: holidayDate(2026, 3, 19), Name: "Hari Suci Nyepi (Saka 1948)"},
	{Date: holidayDate(2026, 3, 21), Name: "Idulfitri 1447 H"},
	{Date: holidayDate(2026, 3, 22), Name: "Idulfitri 1447 H"},
	{Date: holidayDate(2026, 4, 3), Name: "Wafat Yesus Kristus"},
	{Date: holidayDate(2026, 4, 5), Name: "Kebangkitan Yesus Kristus (Paskah)"},
	{Date: holidayDate(2026, 5, 1), Name: "Hari Buruh Internasional"},
	{Date: holidayDate(2026, 5, 14), Name: "Kenaikan Yesus Kristus"},
	{Date: holidayDate(2026, 5, 27), Name: "Iduladha 1447 H"},
	{Date: holidayDate(2026, 5, 31), Name: "Hari Raya Waisak 2570 BE"},
	{Date: holidayDate(2026, 6, 1), Name: "Hari Lahir Pancasila"},
	{Date: holidayDate(2026, 6, 16), Name: "Tahun Baru Islam 1448 H"},
	{Date: holidayDate(2026, 8, 17), Name: "Proklamasi Kemerdekaan RI"},
	{Date: holidayDate(2026, 8, 25), Name: "Maulid Nabi Muhammad"},
	{Date: holidayDate(2026, 12, 25), Name: "Kelahiran Yesus Kristus"},
}

func holidayDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// SeedNationalHolidays idempotently inserts the seed list — safe to run from
// both the API and worker startup (OnConflict DoNothing). Rows added
// manually for new years are never touched.
func (r *Monitoring) SeedNationalHolidays() error {
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&nationalHolidaySeed).Error
}

type HolidayDay struct {
	Date string `json:"date"`
	Name string `json:"name"`
}

// ProjectTimeline is the Bugs-page widget row: the auto-derived QA window
// from STG-prefixed runs, the working-day breakdown, the manual plan/size,
// and the project's scenario total.
type ProjectTimeline struct {
	ProjectID        string       `json:"projectId"`
	JiraInitKey      string       `json:"jiraInitKey"`
	Name             string       `json:"name"`
	ProjectSize      *string      `json:"projectSize"`
	TimelinePlanDays *float64     `json:"timelinePlanDays"`
	QAStartAt        *time.Time   `json:"qaStartAt"`
	QAEndAt          *time.Time   `json:"qaEndAt"`
	CalendarDays     int          `json:"calendarDays"`
	WeekendDays      int          `json:"weekendDays"`
	HolidayDays      int          `json:"holidayDays"`
	WorkingDays      int          `json:"workingDays"`
	Holidays         []HolidayDay `json:"holidays"`
	TotalScenarios   int64        `json:"totalScenarios"`
}

type runDateBounds struct {
	ProjectCode string
	RunID       int64
	MinStart    *time.Time
	MaxEnd      *time.Time
}

// QaTimelines computes the QA timeline for every active project. Start QA is
// the earliest execution date across the project's STG-prefixed Qase runs and
// End QA the latest — run start/end times win, with per-run result
// timestamps as the fallback when Qase leaves them null (e.g. a run that is
// still open has no end_time yet).
func (r *Monitoring) QaTimelines() ([]ProjectTimeline, error) {
	projects, err := r.Projects(-1, false, "")
	if err != nil {
		return nil, err
	}
	var runs []QaseRun
	if err := r.db.Find(&runs).Error; err != nil {
		return nil, err
	}
	var bounds []runDateBounds
	if err := r.db.Model(&QaseResult{}).
		Select("project_code, run_id, MIN(started_at) AS min_start, MAX(ended_at) AS max_end").
		Group("project_code, run_id").
		Scan(&bounds).Error; err != nil {
		return nil, err
	}
	boundsByRun := map[string]runDateBounds{}
	for _, b := range bounds {
		boundsByRun[b.ProjectCode+"|"+strconv.FormatInt(b.RunID, 10)] = b
	}
	runsByProject := map[string][]QaseRun{}
	for _, run := range runs {
		if DashboardEnvironment(run.Title) != "STAGING" {
			continue
		}
		runsByProject[run.ProjectCode] = append(runsByProject[run.ProjectCode], run)
	}
	holidays, err := r.holidayMap()
	if err != nil {
		return nil, err
	}
	out := make([]ProjectTimeline, 0, len(projects))
	for _, p := range projects {
		row := ProjectTimeline{
			ProjectID:        p.ID,
			JiraInitKey:      p.JiraInitKey,
			Name:             p.Name,
			ProjectSize:      p.ProjectSize,
			TimelinePlanDays: p.TimelinePlanDays,
			Holidays:         []HolidayDay{},
			TotalScenarios:   p.QaseTotalCases,
		}
		for _, run := range runsByProject[p.QaseProjectCode] {
			b := boundsByRun[p.QaseProjectCode+"|"+strconv.FormatInt(run.RunID, 10)]
			start := earlierTime(run.StartedAt, b.MinStart)
			end := laterTime(run.FinishedAt, b.MaxEnd)
			if row.QAStartAt == nil || (start != nil && start.Before(*row.QAStartAt)) {
				row.QAStartAt = start
			}
			if row.QAEndAt == nil || (end != nil && end.After(*row.QAEndAt)) {
				row.QAEndAt = end
			}
		}
		if row.QAStartAt != nil && row.QAEndAt != nil {
			// Normalize to WIB day boundaries so the serialized date is the
			// calendar day the dashboard means, whatever the source offset was.
			s := row.QAStartAt.In(wib)
			e := row.QAEndAt.In(wib)
			startDay := time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, wib)
			endDay := time.Date(e.Year(), e.Month(), e.Day(), 0, 0, 0, 0, wib)
			row.QAStartAt = &startDay
			row.QAEndAt = &endDay
			row.CalendarDays, row.WeekendDays, row.HolidayDays, row.WorkingDays, row.Holidays =
				workingDayBreakdown(startDay, endDay, holidays)
		}
		out = append(out, row)
	}
	return out, nil
}

// holidayMap keys official holidays by their WIB calendar date.
func (r *Monitoring) holidayMap() (map[string]NationalHoliday, error) {
	var rows []NationalHoliday
	if err := r.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]NationalHoliday{}
	for _, h := range rows {
		out[h.Date.In(wib).Format("2006-01-02")] = h
	}
	return out, nil
}

// workingDayBreakdown counts the inclusive date span start..end in WIB:
// weekend = Sat/Sun, holidays = seeded tanggal-merah that fall on a weekday
// (a holiday on a weekend is already counted once as a weekend day), and
// working = whatever remains.
func workingDayBreakdown(start, end time.Time, holidays map[string]NationalHoliday) (calendar, weekend, holiday, working int, list []HolidayDay) {
	list = []HolidayDay{}
	day := time.Date(start.In(wib).Year(), start.In(wib).Month(), start.In(wib).Day(), 0, 0, 0, 0, wib)
	last := time.Date(end.In(wib).Year(), end.In(wib).Month(), end.In(wib).Day(), 0, 0, 0, 0, wib)
	for ; !day.After(last); day = day.AddDate(0, 0, 1) {
		calendar++
		if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
			weekend++
			continue
		}
		if h, ok := holidays[day.Format("2006-01-02")]; ok {
			holiday++
			list = append(list, HolidayDay{Date: day.Format("2006-01-02"), Name: h.Name})
			continue
		}
		working++
	}
	return calendar, weekend, holiday, working, list
}

func earlierTime(a, b *time.Time) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.Before(*a):
		return b
	default:
		return a
	}
}

func laterTime(a, b *time.Time) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.After(*a):
		return b
	default:
		return a
	}
}

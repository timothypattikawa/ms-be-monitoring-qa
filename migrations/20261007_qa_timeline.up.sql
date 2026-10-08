BEGIN;

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS timeline_plan_days numeric(8,1);

DO $$ BEGIN
    ALTER TABLE projects ADD CONSTRAINT chk_projects_timeline_plan
        CHECK (timeline_plan_days IS NULL OR timeline_plan_days >= 0) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Official Indonesian public holidays (tanggal merah) only — cuti bersama is
-- intentionally absent and never reduces QA working days. Sources: SKB 3
-- Menteri 1017/2/2-2024 (2025) and 1497/2/5-2025 (2026). The same list is
-- seeded in code by repository.SeedNationalHolidays; this INSERT keeps a
-- migration-managed database identical.
CREATE TABLE IF NOT EXISTS national_holidays (
    date date PRIMARY KEY,
    name text NOT NULL
);

INSERT INTO national_holidays (date, name) VALUES
    ('2025-01-01', 'Tahun Baru 2025'),
    ('2025-01-27', 'Isra Mikraj Nabi Muhammad'),
    ('2025-01-29', 'Tahun Baru Imlek 2576'),
    ('2025-03-29', 'Hari Suci Nyepi (Saka 1947)'),
    ('2025-03-31', 'Idulfitri 1446 H'),
    ('2025-04-01', 'Idulfitri 1446 H'),
    ('2025-04-18', 'Wafat Yesus Kristus'),
    ('2025-04-20', 'Kebangkitan Yesus Kristus (Paskah)'),
    ('2025-05-01', 'Hari Buruh Internasional'),
    ('2025-05-12', 'Hari Raya Waisak 2569 BE'),
    ('2025-05-29', 'Kenaikan Yesus Kristus'),
    ('2025-06-01', 'Hari Lahir Pancasila'),
    ('2025-06-06', 'Iduladha 1446 H'),
    ('2025-06-27', 'Tahun Baru Islam 1447 H'),
    ('2025-08-17', 'Proklamasi Kemerdekaan RI'),
    ('2025-09-05', 'Maulid Nabi Muhammad'),
    ('2025-12-25', 'Kelahiran Yesus Kristus'),
    ('2026-01-01', 'Tahun Baru 2026'),
    ('2026-01-16', 'Isra Mikraj Nabi Muhammad'),
    ('2026-02-17', 'Tahun Baru Imlek 2577'),
    ('2026-03-19', 'Hari Suci Nyepi (Saka 1948)'),
    ('2026-03-21', 'Idulfitri 1447 H'),
    ('2026-03-22', 'Idulfitri 1447 H'),
    ('2026-04-03', 'Wafat Yesus Kristus'),
    ('2026-04-05', 'Kebangkitan Yesus Kristus (Paskah)'),
    ('2026-05-01', 'Hari Buruh Internasional'),
    ('2026-05-14', 'Kenaikan Yesus Kristus'),
    ('2026-05-27', 'Iduladha 1447 H'),
    ('2026-05-31', 'Hari Raya Waisak 2570 BE'),
    ('2026-06-01', 'Hari Lahir Pancasila'),
    ('2026-06-16', 'Tahun Baru Islam 1448 H'),
    ('2026-08-17', 'Proklamasi Kemerdekaan RI'),
    ('2026-08-25', 'Maulid Nabi Muhammad'),
    ('2026-12-25', 'Kelahiran Yesus Kristus')
ON CONFLICT (date) DO NOTHING;

COMMIT;

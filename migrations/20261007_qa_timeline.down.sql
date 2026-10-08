BEGIN;

ALTER TABLE projects DROP CONSTRAINT IF EXISTS chk_projects_timeline_plan;
ALTER TABLE projects DROP COLUMN IF EXISTS timeline_plan_days;
DROP TABLE IF EXISTS national_holidays;

COMMIT;

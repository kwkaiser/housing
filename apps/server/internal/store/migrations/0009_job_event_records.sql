ALTER TABLE job_events ADD COLUMN record TEXT NOT NULL DEFAULT '{}';
UPDATE job_events SET record = json_object('time', at, 'level', level, 'msg', message, 'stage', stage);
ALTER TABLE job_events DROP COLUMN stage;

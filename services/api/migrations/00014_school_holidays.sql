-- +goose Up
CREATE TABLE school_holidays (
 school_id uuid NOT NULL REFERENCES schools(id), date date NOT NULL,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 type text NOT NULL CHECK(type IN('national_holiday','joint_leave')),
 PRIMARY KEY(school_id,date,type,name)
);
CREATE TABLE school_holiday_sync (
 school_id uuid NOT NULL REFERENCES schools(id), year integer NOT NULL CHECK(year BETWEEN 1900 AND 2200),
 attempted_at timestamptz NOT NULL DEFAULT clock_timestamp(), synced_at timestamptz,
 status text NOT NULL CHECK(status IN('ready','unavailable')),
 PRIMARY KEY(school_id,year)
);
-- +goose Down
DROP TABLE school_holiday_sync;
DROP TABLE school_holidays;

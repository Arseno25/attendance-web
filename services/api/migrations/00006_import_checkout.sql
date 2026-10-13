-- +goose Up
CREATE TABLE import_batches (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), school_id uuid NOT NULL, actor_id uuid NOT NULL, class_id uuid NOT NULL,
 payload jsonb, fingerprint text NOT NULL, valid boolean NOT NULL, row_count integer NOT NULL CHECK(row_count BETWEEN 1 AND 500),
 expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '15 minutes', committed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(school_id,actor_id) REFERENCES users(school_id,id), FOREIGN KEY(school_id,class_id) REFERENCES classes(school_id,id)
);
ALTER TABLE attendance_events DROP CONSTRAINT attendance_events_event_type_check;
ALTER TABLE attendance_events ADD CHECK(event_type IN('check_in','check_out','manual_adjustment','reconciliation','correction'));
ALTER TABLE attendance_records ADD CONSTRAINT checkout_order CHECK(check_out_at IS NULL OR (status IN('present','late') AND check_in_at IS NOT NULL AND check_out_at>=check_in_at));
-- +goose Down
ALTER TABLE attendance_records DROP CONSTRAINT checkout_order;
ALTER TABLE attendance_events DROP CONSTRAINT attendance_events_event_type_check;
ALTER TABLE attendance_events ADD CHECK(event_type IN('check_in','manual_adjustment','reconciliation','correction'));
DROP TABLE import_batches;

-- +goose Up
ALTER TABLE enrolments ADD COLUMN entry_path text NOT NULL DEFAULT 'new' CHECK(entry_path IN ('new','continuing'));
-- +goose Down
ALTER TABLE enrolments DROP COLUMN entry_path;

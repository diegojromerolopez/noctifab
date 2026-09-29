ALTER TABLE actions ADD COLUMN action_id TEXT DEFAULT '';
UPDATE actions SET action_id = 'legacy-' || id WHERE action_id = '' OR action_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_actions_action_id ON actions(action_id);
CREATE INDEX IF NOT EXISTS idx_actions_state_id_id ON actions(state_id, id);

package store

const currentSchemaVersion = 2

const schemaV1 = `
CREATE TABLE IF NOT EXISTS runs (
    run_id TEXT PRIMARY KEY,
    target_input TEXT NOT NULL,
    target_user_id TEXT NOT NULL DEFAULT '',
    target_username TEXT NOT NULL,
    target_username_alias_note TEXT NOT NULL DEFAULT '',
    target_display_name TEXT NOT NULL DEFAULT '',
    target_avatar_url TEXT NOT NULL DEFAULT '',
    target_resolution_source TEXT NOT NULL DEFAULT '',
    target_verified_at TEXT NOT NULL,
    target_confirmed INTEGER NOT NULL DEFAULT 1,
    tag TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'running'
);

CREATE TABLE IF NOT EXISTS servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag TEXT NOT NULL,
    invite_code TEXT NOT NULL,
    guild_id TEXT NOT NULL DEFAULT '',
    guild_name TEXT NOT NULL DEFAULT '',
    approximate_member_count INTEGER NOT NULL DEFAULT 0,
    discovered_at TEXT NOT NULL,
    UNIQUE(tag, invite_code)
);

CREATE TABLE IF NOT EXISTS guild_scans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    guild_id TEXT NOT NULL,
    guild_name TEXT NOT NULL DEFAULT '',
    invite_code TEXT NOT NULL DEFAULT '',
    scan_status TEXT NOT NULL DEFAULT 'pending',
    best_match_status TEXT NOT NULL DEFAULT 'unknown',
    best_match_reason TEXT NOT NULL DEFAULT '',
    best_observed_user_id TEXT NOT NULL DEFAULT '',
    best_observed_username TEXT NOT NULL DEFAULT '',
    member_coverage TEXT NOT NULL DEFAULT 'partial',
    members_examined INTEGER NOT NULL DEFAULT 0,
    member_stop_reason TEXT NOT NULL DEFAULT '',
    message_coverage TEXT NOT NULL DEFAULT 'partial',
    messages_examined INTEGER NOT NULL DEFAULT 0,
    oldest_checked_at TEXT NOT NULL DEFAULT '',
    newest_checked_at TEXT NOT NULL DEFAULT '',
    channels_discovered INTEGER NOT NULL DEFAULT 0,
    channels_scanned INTEGER NOT NULL DEFAULT 0,
    channels_unreadable INTEGER NOT NULL DEFAULT 0,
    message_stop_reason TEXT NOT NULL DEFAULT '',
    onboarding_status TEXT NOT NULL DEFAULT '',
    gate_type TEXT NOT NULL DEFAULT '',
    gate_status TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT '',
    UNIQUE(run_id, guild_id),
    CHECK (best_match_status != 'not_found' OR member_coverage = 'complete')
);

CREATE TABLE IF NOT EXISTS match_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    guild_id TEXT NOT NULL,
    observed_user_id TEXT NOT NULL DEFAULT '',
    observed_username_norm TEXT NOT NULL DEFAULT '',
    observed_username TEXT NOT NULL DEFAULT '',
    observed_nick TEXT NOT NULL DEFAULT '',
    observed_joined_at TEXT NOT NULL DEFAULT '',
    observed_roles TEXT NOT NULL DEFAULT '[]',
    observed_premium_since TEXT NOT NULL DEFAULT '',
    observed_guild_avatar_url TEXT NOT NULL DEFAULT '',
    match_status TEXT NOT NULL,
    match_reason TEXT NOT NULL,
    acquisition_method TEXT NOT NULL DEFAULT '',
    observed_at TEXT NOT NULL,
    UNIQUE (run_id, guild_id, observed_user_id, observed_username_norm, observed_nick)
);

CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    guild_id TEXT NOT NULL,
    guild_name TEXT NOT NULL DEFAULT '',
    channel_id TEXT NOT NULL,
    channel_name TEXT NOT NULL DEFAULT '',
    message_id TEXT NOT NULL,
    author_id TEXT NOT NULL,
    author_username TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    timestamp TEXT NOT NULL,
    collected_at TEXT NOT NULL,
    collector_version TEXT NOT NULL,
    acquisition_method TEXT NOT NULL,
    coverage_status TEXT NOT NULL,
    UNIQUE(run_id, guild_id, channel_id, message_id)
);

CREATE TABLE IF NOT EXISTS gate_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    guild_id TEXT NOT NULL,
    gate_type TEXT NOT NULL,
    channel_id TEXT NOT NULL DEFAULT '',
    message_id TEXT NOT NULL DEFAULT '',
    action_taken TEXT NOT NULL,
    operator_decision TEXT NOT NULL,
    attempted_at TEXT NOT NULL,
    UNIQUE(run_id, guild_id, gate_type, channel_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_guild_scans_run_id ON guild_scans(run_id);
CREATE INDEX IF NOT EXISTS idx_observations_run_guild ON match_observations(run_id, guild_id);
CREATE INDEX IF NOT EXISTS idx_messages_run_guild ON messages(run_id, guild_id);
`

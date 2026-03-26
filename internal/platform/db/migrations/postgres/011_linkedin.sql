-- LinkedIn Post Generator schema
-- Migration 011: LinkedIn posts and topics

CREATE TABLE IF NOT EXISTS linkedin_posts (
    id              SERIAL PRIMARY KEY,
    topic           TEXT NOT NULL,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    diagram_desc    TEXT,
    source_project  TEXT,
    target          TEXT NOT NULL DEFAULT 'personal'
        CHECK (target IN ('personal', 'employer')),
    status          TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'pending_review', 'approved', 'posted', 'rejected')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    posted_at       TIMESTAMPTZ,
    feedback_rating INTEGER CHECK (feedback_rating IS NULL OR (feedback_rating >= 1 AND feedback_rating <= 5)),
    feedback_notes  TEXT,
    engagement_likes    INTEGER NOT NULL DEFAULT 0,
    engagement_comments INTEGER NOT NULL DEFAULT 0,
    engagement_reposts  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS linkedin_topics (
    id              SERIAL PRIMARY KEY,
    topic           TEXT NOT NULL,
    source_project  TEXT,
    priority        INTEGER NOT NULL DEFAULT 5,
    used            BOOLEAN NOT NULL DEFAULT FALSE,
    used_at         TIMESTAMPTZ,
    avg_rating      REAL NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_linkedin_posts_status ON linkedin_posts(status);
CREATE INDEX IF NOT EXISTS idx_linkedin_posts_source ON linkedin_posts(source_project);
CREATE INDEX IF NOT EXISTS idx_linkedin_topics_used ON linkedin_topics(used);

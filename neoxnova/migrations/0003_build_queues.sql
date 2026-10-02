-- ============================================================================
-- NEO-XNOVA BUILD QUEUE CONSTRAINTS
-- Idempotent: safe to re-run.
--
-- Enforces a single in-progress item per planet (construction + shipyard) and
-- per user (research), which the build APIs and the durable scheduler rely on.
-- ============================================================================

CREATE UNIQUE INDEX IF NOT EXISTS uq_active_construction
    ON construction_queues(celestial_id) WHERE status = 'IN_PROGRESS';

CREATE UNIQUE INDEX IF NOT EXISTS uq_active_shipyard
    ON shipyard_queues(celestial_id) WHERE status = 'IN_PROGRESS';

CREATE UNIQUE INDEX IF NOT EXISTS uq_active_research
    ON research_queues(user_id) WHERE status = 'IN_PROGRESS';

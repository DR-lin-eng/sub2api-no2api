-- Keep the highest-volume list and maintenance queries on ordered, bounded
-- index scans. These are partial where the runtime query always carries the
-- same predicate, which limits write amplification and index size.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_api_keys_active_user_id
    ON api_keys (user_id, id DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_image_jobs_owner_created_active
    ON batch_image_jobs (user_id, api_key_id, created_at DESC, id DESC)
    WHERE user_deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_image_jobs_stale_unsubmitted
    ON batch_image_jobs (updated_at, id)
    WHERE status IN ('created', 'uploading')
      AND provider_job_name IS NULL
      AND COALESCE(hold_amount, estimated_cost, 0) > 0;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_image_jobs_input_cleanup_due
    ON batch_image_jobs (id)
    WHERE input_deleted_at IS NULL
      AND provider_input_ref IS NOT NULL
      AND status IN ('completed', 'failed', 'cancelled', 'output_deleted');

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_image_jobs_output_cleanup_due
    ON batch_image_jobs (output_expires_at, id)
    WHERE output_deleted_at IS NULL
      AND provider_output_ref IS NOT NULL
      AND status = 'completed'
      AND output_expires_at IS NOT NULL;

-- The partial index above is the only runtime access path for this field and
-- avoids indexing the much larger set of rows whose expiry is NULL.
DROP INDEX CONCURRENTLY IF EXISTS batch_image_jobs_output_expires_at_idx;

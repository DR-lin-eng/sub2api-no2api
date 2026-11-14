-- Candidate order is decided at request time by the weighted rendezvous over
-- the backend-computed composite score, and the per-item weight only nudged
-- that score by at most 0.5 points. Both columns were dead configuration, so
-- drop them instead of keeping a switch that has no effect.

ALTER TABLE group_routing_schemes DROP COLUMN IF EXISTS sort_mode;
ALTER TABLE group_routing_scheme_items DROP COLUMN IF EXISTS weight;

-- Test-only historical acknowledgement for the synthetic local admin.
-- This does not call the acceptance UI/API or acknowledge terms for a real user.
INSERT INTO settings (key,value)
SELECT 'admin_compliance_acknowledgement:'||id,
 json_build_object('version','v2026.06.10','admin_user_id',id,'user_agent','synthetic upgrade fixture','accepted_at','2026-10-01T00:00:00Z')::text
FROM users WHERE email='admin@sub2api.local'
ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value;

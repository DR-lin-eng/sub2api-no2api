INSERT INTO users (email, password_hash, username, role, status)
SELECT 'runtime-legacy-reset@example.com', password_hash, 'runtime reset', 'user', 'active'
FROM users WHERE role = 'admin' ORDER BY id LIMIT 1;
INSERT INTO users (email, password_hash, username, role, status)
SELECT 'runtime-hash-reset@example.com', password_hash, 'runtime hash reset', 'user', 'active'
FROM users WHERE role = 'admin' ORDER BY id LIMIT 1;
INSERT INTO payment_orders (user_id, amount, pay_amount, expires_at, out_trade_no, payment_type, order_type, status)
SELECT id, 12.34, 12.34, NOW() + INTERVAL '1 day', 'upstream-sync-20261005-legacy-order', 'alipay', 'balance', 'PENDING'
FROM users WHERE role = 'admin' ORDER BY id LIMIT 1;
INSERT INTO settings (key, value) VALUES ('email_verify_enabled','true'), ('password_reset_enabled','true')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

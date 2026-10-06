-- Synthetic, isolated Docker data only.
INSERT INTO payment_orders (user_id, amount, pay_amount, expires_at, out_trade_no, payment_type, order_type, status)
SELECT id, 12.34, 12.34, NOW() + INTERVAL '1 day', 'typesafe-recharge-20261006-legacy', 'alipay', 'balance', 'PENDING'
FROM users WHERE role = 'admin' ORDER BY id LIMIT 1;
-- Dormant tiers written before upgrade must remain inactive.
INSERT INTO settings (key, value) VALUES ('RECHARGE_BONUS_TIERS','[{"min_amount":10,"bonus_percent":20}]'), ('RECHARGE_BONUS_MODE','discount')
ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value;

-- Preserve existing integer capacity while retaining the three fractional
-- digits already supported by the gateway currency contract (KWD, BHD, etc.).
ALTER TABLE payment_orders
    ALTER COLUMN pay_amount TYPE DECIMAL(21,3) USING pay_amount::DECIMAL(21,3);

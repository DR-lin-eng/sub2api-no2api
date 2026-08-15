-- Persist public default-route bandwidth samples with the existing Ops minute metrics.

ALTER TABLE ops_system_metrics
    ADD COLUMN IF NOT EXISTS network_receive_bytes_per_second DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS network_transmit_bytes_per_second DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS network_interfaces TEXT[];

COMMENT ON COLUMN ops_system_metrics.network_receive_bytes_per_second IS
    'Average bytes received per second on the default IPv4/IPv6 route interfaces since the previous sample.';
COMMENT ON COLUMN ops_system_metrics.network_transmit_bytes_per_second IS
    'Average bytes transmitted per second on the default IPv4/IPv6 route interfaces since the previous sample.';
COMMENT ON COLUMN ops_system_metrics.network_interfaces IS
    'Deduplicated interface names selected by the default IPv4/IPv6 routes for this sample.';

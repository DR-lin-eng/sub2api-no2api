# HTTP migration envelopes

The HPKE recipient public key is embedded in scoped migration credentials when they are issued or rotated. HTTP-only migration is explicit. Each RPC uses a fresh X25519 encapsulation, HKDF-SHA256 and AES-256-GCM through Go's RFC 9180 implementation. The encrypted request contains the bearer, its timestamp and the ordinary migration payload. Method, exact path/query and clone policy are bound to the context. The response key comes from the HPKE exporter and authenticates response status, policy and body. A database uniqueness check consumes each authenticated encapsulation once across nodes. The durable snapshot still uses the existing per-job ephemeral encryption.

The HTTP origin and operation metadata are visible on the network. TLS termination and strong clock synchronization are covered in the [operator guide](../../../../docs/SITE_MIGRATION.md).

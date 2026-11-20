package sitemigration

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/payment"
)

type row map[string]json.RawMessage

func (r row) str(k string) string { var s string; _ = json.Unmarshal(r[k], &s); return s }
func (r row) set(k, s string)     { r[k], _ = json.Marshal(s) }

type header struct {
	Version               int
	Site, Schema, Profile string
}
type record struct {
	Table   string
	Row     row
	Omitted bool `json:",omitempty"`
}

const maxRow = 64 << 20

func (s Store) exportSnapshot(ctx context.Context, tx *sql.Tx, profile string, out io.Writer) error {
	if profile == "clone" {
		return dumpClone(ctx, tx, s.DataDir, out)
	}
	c, err := readCatalog(ctx, tx, profile)
	if err != nil {
		return err
	}
	var site string
	if err = tx.QueryRowContext(ctx, "SELECT site FROM site_migration_identity").Scan(&site); err != nil {
		return err
	}
	secret, err := storageKey(ctx, tx)
	if err != nil {
		return err
	}
	if profile == "all" || profile == "login-quota" {
		var pending bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT FROM usage_billing_jobs)").Scan(&pending); err != nil {
			return err
		}
		if pending {
			return errors.New("source settlement queue must drain before migrating balances")
		}
	}
	enc := json.NewEncoder(out)
	if err = enc.Encode(header{1, site, c.hash, profile}); err != nil {
		return err
	}
	for _, name := range c.order {
		t := c.tables[name]
		rows, err := snapshotRows(ctx, tx, t, profile)
		if err != nil {
			return err
		}
		for rows.Next() {
			var b []byte
			var r row
			var omitted bool
			if err = rows.Scan(&b, &omitted); err == nil && len(b) > maxRow/2 {
				err = ErrLimit
			}
			if err == nil {
				err = json.Unmarshal(b, &r)
			}
			if err == nil && !omitted {
				err = portableSecrets(name, r, secret, true)
			}
			if err == nil {
				err = enc.Encode(record{Table: name, Row: r, Omitted: omitted})
			}
			if err != nil {
				_ = rows.Close()
				return fmt.Errorf("cannot export %s: %w", name, err)
			}
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
	}
	return nil
}
func storageKey(ctx context.Context, tx *sql.Tx) ([]byte, error) {
	var value string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM public.security_secrets WHERE key='totp_encryption_key'").Scan(&value); err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(value)
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid local credential encryption key")
	}
	return key, nil
}
func changeSecret(s string, key []byte, export bool) (string, error) {
	if s == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if export {
		b, e := base64.StdEncoding.DecodeString(s)
		if e != nil || len(b) < g.NonceSize() {
			return "", errors.New("unreadable stored credential")
		}
		p, e := g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], nil)
		if e != nil {
			return "", errors.New("unreadable stored credential")
		}
		return string(p), nil
	}
	nonce := make([]byte, g.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(s), nil)), nil
}
func portableSecrets(table string, r row, key []byte, export bool) error {
	field := ""
	switch table {
	case "users":
		field = "totp_secret_encrypted"
	case "channel_monitors":
		field = "api_key_encrypted"
	case "payment_provider_instances":
		conf := r.str("config")
		if conf == "" {
			return nil
		}
		if export {
			m, err := payment.DecodeProviderConfig(conf, key)
			if err != nil {
				return errors.New("unreadable payment configuration")
			}
			b, err := json.Marshal(m)
			if err != nil {
				return err
			}
			r.set("config", string(b))
		} else {
			c, err := payment.Encrypt(conf, key)
			if err != nil {
				return err
			}
			r.set("config", c)
		}
		return nil
	case "accounts":
		raw := r["extra"]
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return errors.New("invalid account extra")
		}
		if obj == nil {
			return nil
		}
		for _, k := range []string{"codex_remote_control_token_ciphertext", "ollama_cloud_usage_session"} {
			v, ok := obj[k]
			if !ok || bytes.Equal(v, []byte("null")) {
				continue
			}
			var text string
			if err := json.Unmarshal(v, &text); err != nil {
				return errors.New("invalid account credential")
			}
			changed, err := changeSecret(text, key, export)
			if err != nil {
				return err
			}
			obj[k], err = json.Marshal(changed)
			if err != nil {
				return err
			}
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		r["extra"] = b
		return nil
	default:
		return nil
	}
	if v := r.str(field); v != "" {
		s, err := changeSecret(v, key, export)
		if err != nil {
			return err
		}
		r.set(field, s)
	}
	return nil
}
func stage(ctx context.Context, tx *sql.Tx, j *session, input io.Reader, c catalog) (string, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), maxRow)
	var h header
	if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &h) != nil || h.Version != 1 || !validID(h.Site) || h.Profile != j.Profile || h.Schema != c.hash {
		return "", errors.New("migration schema or snapshot header mismatch")
	}
	var same bool
	if err := tx.QueryRowContext(ctx, "SELECT site=$1 FROM site_migration_identity", h.Site).Scan(&same); err != nil {
		return "", err
	}
	if same {
		return "", errors.New("source and destination are the same site")
	}
	key, err := storageKey(ctx, tx)
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `CREATE TEMP TABLE migration_rows(n bigint GENERATED ALWAYS AS IDENTITY, t text NOT NULL, data jsonb NOT NULL) ON COMMIT DROP;
        CREATE TEMP TABLE migration_map(t text,k text,src text,dst text, PRIMARY KEY(t,k,src)) ON COMMIT DROP`); err != nil {
		return "", err
	}
	// Block reads use this transaction, so a COPY stream cannot share the connection.
	var batch []string
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO pg_temp.migration_rows(t,data) SELECT v->>'Table',v->'Row' FROM jsonb_array_elements($1::jsonb) AS v", "["+strings.Join(batch, ",")+"]")
		batch = nil
		return e
	}
	size := 0
	for scanner.Scan() {
		var rec record
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			return "", ErrInvalid
		}
		t := c.tables[rec.Table]
		if t == nil {
			return "", ErrInvalid
		}
		if rec.Omitted {
			if err := recordPolicySkip(ctx, tx, j, t, rec); err != nil {
				return "", err
			}
			continue
		}
		if len(rec.Row) != len(t.cols) {
			return "", ErrInvalid
		}
		for _, k := range t.cols {
			if rec.Row[k] == nil {
				return "", ErrInvalid
			}
		}
		if err := validatePolicyRow(j.Profile, t, rec.Row); err != nil {
			return "", err
		}
		if err = portableSecrets(t.name, rec.Row, key, false); err != nil {
			return "", err
		}
		b, err := json.Marshal(rec)
		if err != nil {
			return "", err
		}
		batch = append(batch, string(b))
		size += len(b)
		if size >= BlockSize || len(batch) >= 256 {
			if err = flush(); err != nil {
				return "", err
			}
			size = 0
		}
	}
	if err = scanner.Err(); err != nil {
		return "", err
	}
	if err = flush(); err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `CREATE INDEX ON migration_rows(t,n); CREATE INDEX ON migration_rows(t,(data->>'user_id')); CREATE INDEX ON migration_rows(t,(data->>'identity_id')); ANALYZE migration_rows`)
	return h.Site, err
}

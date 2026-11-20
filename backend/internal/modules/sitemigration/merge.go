package sitemigration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

type importer struct {
	ctx  context.Context
	tx   *sql.Tx
	j    *session
	site string
	totp bool
}

func importSnapshot(ctx context.Context, tx *sql.Tx, j *session, input io.Reader) error {
	c, err := readCatalog(ctx, tx, j.Profile)
	if err != nil {
		return err
	}
	site, err := stage(ctx, tx, j, input, c)
	if err != nil {
		return err
	}
	im := importer{ctx: ctx, tx: tx, j: j, site: site}
	// Exclude concurrent business writes while checking natural keys and
	// allocating IDs. Ordinary reads remain available. Acquisition is bounded.
	names := make([]string, 0, len(c.order)+1)
	for _, t := range c.order {
		names = append(names, qt(t))
	}
	if c.tables["settings"] == nil {
		names = append(names, qt("settings"))
	}
	slices.Sort(names)
	if _, err = tx.ExecContext(ctx, "SET LOCAL lock_timeout = '10s'"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "LOCK TABLE "+strings.Join(names, ",")+" IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return err
	}
	if err = im.buildEmailIndex(); err != nil {
		return err
	}
	// Only presentation settings are portable. The destination MFA gate is checked under lock.
	order := slices.Clone(c.order)
	if i := slices.Index(order, "settings"); i >= 0 {
		order = append([]string{"settings"}, slices.Delete(order, i, i+1)...)
	}
	for _, name := range order {
		if name == "users" {
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM public.settings WHERE key='totp_enabled' AND value='true')`).Scan(&im.totp); err != nil {
				return err
			}
		}
		if err = im.mergeTable(c.tables[name]); err != nil {
			return fmt.Errorf("merge %s: %w", name, err)
		}
	}
	if j.Profile == "all" {
		return im.protectAccountBindings()
	}
	return nil
}

// Same inbox identity as registration: only Gmail folds aliases and local dots.
func emailExpr(col string) string {
	v := "lower(rtrim(btrim(" + col + "),'.'))"
	return "CASE WHEN split_part(" + v + ",'@',2) IN ('gmail.com','googlemail.com') THEN replace(split_part(split_part(" + v + ",'@',1),'+',1),'.','')||'@gmail.com' ELSE " + v + " END"
}
func (im *importer) buildEmailIndex() error {
	_, err := im.tx.ExecContext(im.ctx, "CREATE TEMP TABLE migration_emails(email text PRIMARY KEY) ON COMMIT DROP")
	if err != nil {
		return err
	}
	_, err = im.tx.ExecContext(im.ctx, "INSERT INTO migration_emails SELECT DISTINCT "+emailExpr("email")+" FROM public.users WHERE deleted_at IS NULL UNION SELECT "+emailExpr("provider_subject")+" FROM public.auth_identities WHERE provider_type='email' ON CONFLICT DO NOTHING")
	return err
}
func idText(r row, k string) string {
	if b := r[k]; len(b) > 0 && b[0] == '"' {
		return r.str(k)
	}
	return string(r[k])
}
func safeCell(s string) string {
	// Spreadsheet formula evaluation also accepts prefix whitespace/control characters.
	t := strings.TrimSpace(s)
	if strings.ContainsAny(s, "\r\n\t") || len(t) > 0 && strings.ContainsRune("=+-@＝＋－＠", []rune(t)[0]) {
		return "'" + s
	}
	return s
}
func (im *importer) reject(t *table, r row, reason string) error {
	ident := idText(r, "id")
	if ident == "null" || len(ident) > 64 {
		ident = ""
	}
	var out bytes.Buffer
	w := csv.NewWriter(&out)
	if err := w.Write([]string{t.name, safeCell(ident), safeCell(r.str("email")), reason}); err != nil {
		return err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	if im.j.Errors >= 2_000_000 {
		return ErrLimit
	}
	data := im.j.seal(1, im.j.Errors, true, out.Bytes())
	if err := saveBlock(im.ctx, im.tx, im.j.ID, 1, im.j.Errors, data); err != nil {
		return err
	}
	im.j.Errors++
	return nil
}

type stagedRow struct {
	seq    int64
	fields row
}

// Rows can point forwards within a table. Keep unresolved rows in the
// transaction's on-disk staging table and only retry while progress is made.
func (im *importer) mergeTable(t *table) error {
	self := slices.ContainsFunc(t.refs, func(r ref) bool { return r.parent == t.name })
	deferParents := self
	for {
		processed, waiting, err := im.mergePass(t, self, deferParents)
		if err != nil {
			return err
		}
		if !self || waiting == 0 {
			return nil
		}
		if processed == 0 {
			deferParents = false
		}
	}
}
func (im *importer) mergePass(t *table, self, deferParents bool) (int, int, error) {
	var cursor int64
	processed, waiting := 0, 0
	for {
		rows, err := im.tx.QueryContext(im.ctx, "SELECT n,data FROM pg_temp.migration_rows WHERE t=$1 AND n>$2 ORDER BY n LIMIT 64", t.name, cursor)
		if err != nil {
			return 0, 0, err
		}
		var page []stagedRow
		size := 0
		for rows.Next() {
			var b []byte
			var r row
			if err = rows.Scan(&cursor, &b); err == nil {
				err = json.Unmarshal(b, &r)
			}
			if err != nil {
				_ = rows.Close()
				return 0, 0, err
			}
			page = append(page, stagedRow{cursor, r})
			size += len(b)
			if size >= 4*BlockSize {
				break
			}
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		if err = rows.Close(); err != nil {
			return 0, 0, err
		}
		if len(page) == 0 {
			return processed, waiting, nil
		}
		var done []int64
		for _, record := range page {
			if deferParents {
				ready, err := im.selfRefsReady(t, record.fields)
				if err != nil {
					return 0, 0, err
				}
				if !ready {
					waiting++
					continue
				}
			}
			originalID := record.fields["id"]
			rebind := false
			if t.name == "api_keys" {
				var e error
				rebind, e = im.needsKeyRebinding(record.fields)
				if e != nil {
					return 0, 0, e
				}
				if rebind {
					detachForeignKeyGroups(record.fields)
				}
			}
			reason, err := im.insert(t, record.fields)
			if err != nil {
				return 0, 0, err
			}
			if reason != "" {
				if originalID != nil {
					record.fields["id"] = originalID
				}
				if err = im.reject(t, record.fields, reason); err != nil {
					return 0, 0, err
				}
			} else if rebind {
				if err = im.reject(t, row{"id": originalID}, "Key 已保留并停用，用户需在新站重新选择分组并启用"); err != nil {
					return 0, 0, err
				}
			}
			if self {
				done = append(done, record.seq)
			}
			processed++
		}
		if len(done) > 0 {
			if _, err := im.tx.ExecContext(im.ctx, "DELETE FROM pg_temp.migration_rows WHERE t=$1 AND n=ANY($2)", t.name, pq.Array(done)); err != nil {
				return 0, 0, err
			}
		}
	}
}
func (im *importer) selfRefsReady(t *table, r row) (bool, error) {
	for _, ref := range t.refs {
		if ref.parent != t.name {
			continue
		}
		src := idText(r, ref.col)
		if src == "" || src == "null" || src == idText(r, ref.key) {
			continue
		}
		_, ok, err := im.mapped(t.name, ref.key, src)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}
func (im *importer) userConflict(r row) (string, error) {
	var dup bool
	id := idText(r, "id")
	if err := im.tx.QueryRowContext(im.ctx, "SELECT EXISTS(SELECT FROM pg_temp.migration_emails WHERE email="+emailExpr("$1::text")+")", r.str("email")).Scan(&dup); err != nil {
		return "", err
	}
	if dup {
		return "相同邮箱或邮箱别名已存在，保留接收站账号、密码和额度", nil
	}
	if role := r["role"]; role != nil && r.str("role") != "user" {
		return "管理账号和权限组属于接收站的授权边界", nil
	}
	if string(r["totp_enabled"]) == "true" && (!im.totp || r.str("totp_secret_encrypted") == "") {
		return "双因素登录条件不满足，请先在接收站启用 TOTP 并核对源密钥", nil
	}
	// Skip the entire login principal if ANY of its credentials belongs to a
	// destination account. Never attach a new password to that account.
	err := im.tx.QueryRowContext(im.ctx, `SELECT EXISTS(
      SELECT FROM migration_rows s JOIN public.auth_identities d ON
        d.provider_type=s.data->>'provider_type' AND d.provider_subject=s.data->>'provider_subject'
        AND (d.provider_key=s.data->>'provider_key' OR (d.provider_type='wechat' AND d.provider_key IN ('wechat-main','wechat') AND s.data->>'provider_key' IN ('wechat-main','wechat')))
        WHERE s.t='auth_identities' AND s.data->>'user_id'=$1
      UNION ALL SELECT FROM migration_rows s JOIN public.passkey_credentials d ON d.credential_id=(s.data->>'credential_id')::bytea
        WHERE s.t='passkey_credentials' AND s.data->>'user_id'=$1
      UNION ALL SELECT FROM migration_rows s JOIN public.passkey_user_handles d ON d.user_handle=(s.data->>'user_handle')::bytea
        WHERE s.t='passkey_user_handles' AND s.data->>'user_id'=$1
      UNION ALL SELECT FROM migration_rows s JOIN migration_rows i ON i.t='auth_identities' AND i.data->>'id'=s.data->>'identity_id'
        JOIN public.auth_identity_channels d ON (d.provider_type,d.channel,d.channel_app_id,d.channel_subject)=
        (s.data->>'provider_type',s.data->>'channel',s.data->>'channel_app_id',s.data->>'channel_subject')
        AND (d.provider_key=s.data->>'provider_key' OR (d.provider_type='wechat' AND d.provider_key IN ('wechat-main','wechat') AND s.data->>'provider_key' IN ('wechat-main','wechat')))
        WHERE s.t='auth_identity_channels' AND i.data->>'user_id'=$1)`, id).Scan(&dup)
	if err != nil {
		return "", err
	}
	if dup {
		return "第三方登录身份或 Passkey 已绑定接收站用户", nil
	}
	return "", nil
}
func (im *importer) mapped(parent, key, src string) (string, bool, error) {
	var dest string
	err := im.tx.QueryRowContext(im.ctx, "SELECT dst FROM pg_temp.migration_map WHERE t=$1 AND k=$2 AND src=$3", parent, key, src).Scan(&dest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return dest, err == nil, err
}
func (im *importer) insert(t *table, r row) (string, error) {
	if reason := mergeBoundary(t.name, r); reason != "" {
		return reason, nil
	}
	original := make(row, len(r))
	for k, v := range r {
		original[k] = v
	}
	if t.name == "users" {
		reason, err := im.userConflict(r)
		if err != nil || reason != "" {
			return reason, err
		}
	}
	// Never resolve a dependency from an identically numbered row at the destination.
	pk := make([]json.RawMessage, len(t.pk))
	for i, k := range t.pk {
		pk[i] = r[k]
	}
	encoded, err := json.Marshal(pk)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	var seen bool
	if err = im.tx.QueryRowContext(im.ctx, "SELECT EXISTS(SELECT FROM site_migration_origins WHERE site=$1 AND relation=$2 AND source_key=$3)", im.site, t.name, digest[:]).Scan(&seen); err != nil {
		return "", err
	}
	if seen {
		return "该源记录已导入；保留接收站记录，本次不建立身份关联", nil
	}
	for _, ref := range t.refs {
		v := idText(r, ref.col)
		if v == "" || v == "null" {
			continue
		}
		// A self reference can keep its own value only after allocation.
		if ref.parent == t.name && v == idText(original, ref.key) {
			continue
		}
		dst, ok, e := im.mapped(ref.parent, ref.key, v)
		if e != nil {
			return "", e
		}
		if !ok {
			return "关联记录未导入或存在冲突：" + ref.parent + "." + ref.col, nil
		}
		if r[ref.col][0] == '"' {
			r.set(ref.col, dst)
		} else {
			r[ref.col] = json.RawMessage(dst)
		}
	}
	if err := rewriteEmbedded(t.name, r, im.mapped); err != nil {
		var missing *referenceConflict
		if errors.As(err, &missing) {
			return missing.Error(), nil
		}
		return "", err
	}
	if t.serial {
		id, e := strconv.ParseInt(idText(original, "id"), 10, 64)
		if e != nil || id <= 0 || id >= 1<<62 {
			return "", ErrInvalid
		}
		// Only destination sequences allocate new IDs, preventing reuse of IDs
		// of deleted users/keys and stale session or cache identity collisions.
		var newID int64
		if err = im.tx.QueryRowContext(im.ctx, "SELECT nextval(pg_get_serial_sequence($1,'id'))", qt(t.name)).Scan(&newID); err != nil {
			return "", err
		}
		// Older manual data imports sometimes leave a sequence behind its
		// rows. A primary-key collision there is not a business conflict.
		var occupied bool
		if err = im.tx.QueryRowContext(im.ctx, "SELECT EXISTS(SELECT FROM "+qt(t.name)+" WHERE id=$1)", newID).Scan(&occupied); err != nil {
			return "", err
		}
		if occupied {
			query := "SELECT nextval(pg_get_serial_sequence($1,'id')) FROM (SELECT setval(pg_get_serial_sequence($1,'id'), GREATEST(MAX(id),$2),true) FROM " + qt(t.name) + ") advanced"
			if err = im.tx.QueryRowContext(im.ctx, query, qt(t.name), newID).Scan(&newID); err != nil {
				return "", err
			}
		}
		r["id"] = json.RawMessage(strconv.FormatInt(newID, 10))
		for _, ref := range t.refs {
			if ref.parent == t.name && ref.key == "id" && idText(original, ref.col) == strconv.FormatInt(id, 10) {
				r[ref.col] = r["id"]
			}
		}
	}
	b, e := json.Marshal(r)
	if e != nil {
		return "", e
	}
	if _, e = im.tx.ExecContext(im.ctx, "SAVEPOINT migration_row"); e != nil {
		return "", e
	}
	list := columns(t.cols)
	result, e := im.tx.ExecContext(im.ctx, "INSERT INTO "+qt(t.name)+" ("+list+") SELECT "+list+" FROM jsonb_populate_record(NULL::"+qt(t.name)+",$1::jsonb) ON CONFLICT DO NOTHING", string(b))
	if e != nil {
		var pe *pq.Error
		if !errors.As(e, &pe) || !strings.HasPrefix(string(pe.Code), "23") {
			return "", e
		}
		if _, err = im.tx.ExecContext(im.ctx, "ROLLBACK TO SAVEPOINT migration_row"); err != nil {
			return "", err
		}
	}
	if _, err = im.tx.ExecContext(im.ctx, "RELEASE SAVEPOINT migration_row"); err != nil {
		return "", err
	}
	if e != nil {
		return "接收站的数据完整性约束冲突", nil
	}
	n, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "接收站已有相同业务唯一键或主键", nil
	}
	_, err = im.tx.ExecContext(im.ctx, "INSERT INTO site_migration_origins(site,relation,source_key) VALUES($1,$2,$3)", im.site, t.name, digest[:])
	if err != nil {
		return "", err
	}
	for _, k := range []string{"id", "batch_id", "hold_key"} {
		if original[k] != nil && r[k] != nil {
			_, err = im.tx.ExecContext(im.ctx, "INSERT INTO pg_temp.migration_map VALUES($1,$2,$3,$4)", t.name, k, idText(original, k), idText(r, k))
			if err != nil {
				return "", err
			}
		}
	}
	if t.name == "users" && (r["deleted_at"] == nil || string(r["deleted_at"]) == "null") {
		_, err = im.tx.ExecContext(im.ctx, "INSERT INTO migration_emails VALUES("+emailExpr("$1::text")+") ON CONFLICT DO NOTHING", r.str("email"))
		if err != nil {
			return "", err
		}
	}
	im.j.Inserted++
	return "", nil
}

// An account that loses its last group is eligible for the receiver's ungrouped
// gateway traffic. Check imported accounts before the merge transaction becomes
// visible. Retain their history, and keep them out of scheduling if source
// group membership or inherited network binding could not be preserved.
func (im *importer) protectAccountBindings() error {
	if _, err := im.tx.ExecContext(im.ctx, "CREATE INDEX ON pg_temp.migration_rows(t,(data->>'account_id'))"); err != nil {
		return err
	}
	var cursor string
	for {
		rows, err := im.tx.QueryContext(im.ctx, `WITH batch AS (
            SELECT m.src,a.id FROM pg_temp.migration_map m JOIN public.accounts a ON a.id=m.dst::bigint
            WHERE m.t='accounts' AND m.k='id' AND m.src>$1 AND a.schedulable AND a.deleted_at IS NULL
            AND (
                NOT EXISTS (SELECT FROM public.account_groups d WHERE d.account_id=a.id)
                OR EXISTS (SELECT FROM pg_temp.migration_rows s
                    WHERE s.t='account_groups' AND s.data->>'account_id'=m.src AND NOT EXISTS (
                        SELECT FROM pg_temp.migration_map g JOIN public.account_groups d ON d.group_id=g.dst::bigint
                        WHERE g.t='groups' AND g.k='id' AND g.src=s.data->>'group_id' AND d.account_id=a.id))
                OR EXISTS (SELECT FROM pg_temp.migration_rows s
                    WHERE s.t='account_egress_bindings' AND s.data->>'account_id'=m.src))
            ORDER BY m.src LIMIT 512
        ), changed AS (
            UPDATE public.accounts a SET schedulable=false,updated_at=now() FROM batch b WHERE a.id=b.id RETURNING b.src
        ) SELECT src FROM changed ORDER BY src`, cursor)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		for _, id := range ids {
			if err = im.reject(&table{name: "accounts"}, row{"id": json.RawMessage(id)}, "账号已导入并暂停调度，原分组或网络绑定与接收站不兼容"); err != nil {
				return err
			}
		}
		cursor = ids[len(ids)-1]
	}
}

// An all-data merge may reuse only groups that were actually imported by
// this job. One missing binding invalidates the entire binding set.
func (im *importer) needsKeyRebinding(r row) (bool, error) {
	if im.j.Profile == userKeysProfile {
		return true, nil
	}
	base := idText(r, "group_id")
	if base != "" && base != "null" {
		_, ok, err := im.mapped("groups", "id", base)
		if err != nil || !ok {
			return !ok, err
		}
	}
	// rewriteEmbedded may partially rewrite r on success; validate on a copy.
	probe := row{"group_bindings": r["group_bindings"]}
	if err := rewriteEmbedded("api_keys", probe, im.mapped); err != nil {
		var missing *referenceConflict
		if errors.As(err, &missing) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// The three-field profile does not import groups. A numeric group ID, including
// one that happens to exist locally, is not proof that both sites grant the same access.
func detachForeignKeyGroups(r row) {
	r["group_id"] = json.RawMessage("null")
	r["group_bindings"] = json.RawMessage("[]")
	r.set("status", "inactive")
}

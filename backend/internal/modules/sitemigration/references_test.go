package sitemigration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestMergeMapsPrivateAudienceAndScopedPrices(t *testing.T) {
	ids := map[string]map[string]string{"groups": {"7": "19"}, "accounts": {"21": "81"}}
	lookup := func(parent, key, id string) (string, bool, error) {
		result, ok := ids[parent][id]
		return result, ok, nil
	}
	cases := []struct{ table, field, input, want string }{
		{"announcements", "targeting", `{"any_of":[{"all_of":[{"group_ids":[7]}]}]}`, `{"any_of":[{"all_of":[{"group_ids":[19]}]}]}`},
		{"groups", "model_routing", `{"gpt-*":[21]}`, `{"gpt-*":[81]}`},
		{"channel_account_stats_pricing_rules", "group_ids", `[7]`, `[19]`},
		{"channel_account_stats_pricing_rules", "account_ids", `[21]`, `[81]`},
		{"api_keys", "group_bindings", `[{"group_id":7,"max_rate_multiplier":2.3}]`, `[{"group_id":19,"max_rate_multiplier":2.3}]`},
	}
	for _, tc := range cases {
		t.Run(tc.table+"."+tc.field, func(t *testing.T) {
			r := row{tc.field: json.RawMessage(tc.input)}
			if err := rewriteEmbedded(tc.table, r, lookup); err != nil {
				t.Fatal(err)
			}
			if string(r[tc.field]) != tc.want {
				t.Fatalf("got %s, want %s", r[tc.field], tc.want)
			}
			noMapping := func(string, string, string) (string, bool, error) { return "", false, nil }
			var missing *referenceConflict
			if !errors.As(rewriteEmbedded(tc.table, r, noMapping), &missing) {
				t.Fatal("missing parent must reject the entire row")
			}
		})
	}
}

// Run against a disposable database on which the application's migrations have
// already been applied. All fixture table changes are rolled back.
func TestMergeForwardReferencesAndSourceConflictCSV(t *testing.T) {
	dsn := os.Getenv("SITE_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("requires an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE migration_rows(n bigint GENERATED ALWAYS AS IDENTITY, t text, data jsonb) ON COMMIT DROP;
        CREATE TEMP TABLE migration_map(t text,k text,src text,dst text, PRIMARY KEY(t,k,src)) ON COMMIT DROP`)
	if err != nil {
		t.Fatal(err)
	}
	j := &session{State: State{ID: uuid.NewString(), Profile: "all"}}
	j.aead, err = makeCipher([]byte("fixture-only"), []byte(j.ID), "merge test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO public.site_migration_jobs(id,owner,mode,profile,local_public,wrapped_private) VALUES($1,'unit','import','all','','')", j.ID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := readCatalog(ctx, tx, "all")
	if err != nil {
		t.Fatal(err)
	}
	g := c.tables["groups"]
	var bytes []byte
	err = tx.QueryRowContext(ctx, "SELECT to_jsonb(g) FROM public.groups g WHERE deleted_at IS NULL LIMIT 1").Scan(&bytes)
	if err != nil {
		t.Fatal("fixture needs one group", err)
	}
	var seed row
	if err = json.Unmarshal(bytes, &seed); err != nil {
		t.Fatal(err)
	}
	prefix := "migration-fixture-" + uuid.NewString()
	add := func(id, n, fallback string) {
		t.Helper()
		seed.set("name", n)
		seed["id"] = json.RawMessage(id)
		seed["fallback_group_id"] = json.RawMessage(fallback)
		seed["model_routing"] = json.RawMessage("{}")
		raw, err := json.Marshal(seed)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO migration_rows(t,data) VALUES('groups',$1)", string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	// A lower-ID child points to its later-created parent. A subsequent
	// duplicate has a known source ID that must be retained in the CSV.
	add("1001", prefix+"-child", "1002")
	add("1002", prefix+"-parent", "null")
	add("81001", prefix+"-parent", "null")
	im := importer{ctx: ctx, tx: tx, j: j, site: uuid.NewString()}
	if err = im.mergeTable(g); err != nil {
		t.Fatal(err)
	}
	var valid bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT FROM groups child JOIN groups parent ON parent.id=child.fallback_group_id WHERE child.name=$1 AND parent.name=$2)", prefix+"-child", prefix+"-parent").Scan(&valid); err != nil || !valid {
		t.Fatal("invalid forward relation", err)
	}
	if j.Inserted != 2 || j.Errors != 1 {
		t.Fatalf("inserted %d, errors %d", j.Inserted, j.Errors)
	}
	b, err := getBlock(ctx, tx, j.ID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	report, _, err := j.plain(1, 0, b)
	if err != nil || !strings.Contains(string(report), "groups,81001,") {
		t.Fatalf("wrong source ID in report: %q (%v)", report, err)
	}
	// Dropping a missing source group must not make the upstream account
	// eligible for ungrouped traffic. Existing pool accounts are unrelated.
	var unbound, existing int64
	for i, out := range []*int64{&unbound, &existing} {
		err = tx.QueryRowContext(ctx, "INSERT INTO accounts(name,platform,type,schedulable) VALUES($1,'openai','apikey',true) RETURNING id", fmt.Sprintf("%s-account-%d", prefix, i)).Scan(out)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO pg_temp.migration_map VALUES('accounts','id','39',$1)", strconv.FormatInt(unbound, 10)); err != nil {
		t.Fatal(err)
	}
	if err = im.protectAccountBindings(); err != nil {
		t.Fatal(err)
	}
	var importedActive, existingActive bool
	if err = tx.QueryRowContext(ctx, "SELECT a.schedulable,b.schedulable FROM accounts a,accounts b WHERE a.id=$1 AND b.id=$2", unbound, existing).Scan(&importedActive, &existingActive); err != nil || importedActive || !existingActive {
		t.Fatal("unsafe ungrouped scheduling", err)
	}

}

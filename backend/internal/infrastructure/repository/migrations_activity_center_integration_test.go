//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

const activityPreflight = "236a_repair_activity_center_campaign_config.sql"
const activityConvergence = "238_preserve_legacy_activity_campaign_types.sql"

func activityMigrationFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	for _, name := range []string{activityPreflight, activityCenterMigration, activityConvergence} {
		b, err := migrations.FS.ReadFile(name)
		require.NoError(t, err)
		files[name] = &fstest.MapFile{Data: b}
	}
	return files
}

func activityMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	// Each scenario has its own public schema, including the preflight's public lookup.
	name := fmt.Sprintf("activity_migration_%d", time.Now().UnixNano())
	_, err := integrationDB.Exec("CREATE DATABASE " + name)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.Exec("DROP DATABASE " + name + " WITH (FORCE)") })
	dsn, err := url.Parse(integrationPostgresDSN)
	require.NoError(t, err)
	dsn.Path = "/" + name
	db, err := sql.Open("postgres", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(2)
	_, err = db.Exec("CREATE TABLE users (id BIGINT PRIMARY KEY); INSERT INTO users VALUES (7);")
	require.NoError(t, err)
	return db
}

func activityExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	_, err := db.Exec(query, args...)
	require.NoError(t, err)
}

func activitySnapshot(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	var value string
	// New compatibility columns are asserted separately; every existing value,
	// including IDs, timestamps, soft-deletion state and rewards, must be identical.
	err := db.QueryRow("SELECT COALESCE(jsonb_agg(to_jsonb(t) - 'config_json' - 'banner_html' ORDER BY id), '[]'::jsonb)::text FROM " + table + " t").Scan(&value)
	require.NoError(t, err)
	return value
}

func TestActivityCenterMigration_UpgradeMatrix(t *testing.T) {
	for _, scenario := range []string{"fresh", "legacy_without_banner", "populated_missing_config", "participation_checkin_only", "populated_with_config", "already_applied"} {
		t.Run(scenario, func(t *testing.T) {
			db := activityMigrationDB(t)
			files := activityMigrationFiles(t)
			original := originalActivityMigration(t)
			var campaignsBefore, recordsBefore, checkinsBefore, configBefore, bannerBefore string
			hasRecords := scenario != "fresh" && scenario != "legacy_without_banner"
			if scenario == "legacy_without_banner" {
				legacy, err := os.ReadFile("testdata/activity_center_legacy_campaigns.sql")
				require.NoError(t, err)
				activityExec(t, db, string(legacy))
				activityExec(t, db, "ALTER TABLE act_campaigns DROP COLUMN banner_html; ALTER TABLE act_campaigns DROP CONSTRAINT chk_act_campaigns_type;")
				for i, kind := range []string{"lottery", "redeem", "custom", "external_link", "announcement"} {
					activityExec(t, db, "INSERT INTO act_campaigns(id,title,type,ref_id,created_by) VALUES ($1,'legacy',$2,'https://example.invalid/preserved',7)", i+1, kind)
				}
			} else if hasRecords {
				activityExec(t, db, original)
				if scenario != "already_applied" {
					activityExec(t, db, string(files[activityConvergence].Data))
				}
				kinds := []string{"lottery", "inflate", "redeem", "custom", "checkin", "external_link", "announcement"}
				if scenario == "already_applied" {
					kinds = kinds[:5]
				}
				if scenario == "participation_checkin_only" {
					kinds = []string{"custom"}
				}
				for i, kind := range kinds {
					activityExec(t, db, `INSERT INTO act_campaigns(id,title,type,config_json,banner_html,created_by) VALUES ($1,'preserved',$2,'{"sentinel":"keep-config"}','<p>preserved banner</p>',7)`, i+1, kind)
					recordKind := kind
					if scenario == "participation_checkin_only" {
						recordKind = "checkin"
					}
					activityExec(t, db, `INSERT INTO act_participation_records(campaign_id,user_id,campaign_type,result_status,reward_status,reward_payload_json) VALUES ($1,7,$2,'won','granted','{"code":"fixture-reward","amount":1.25}')`, i+1, recordKind)
				}
				activityExec(t, db, `INSERT INTO act_checkin_records(campaign_id,user_id,checkin_date,cycle_day,streak_days,reward_type,reward_value,reward_status) VALUES (1,7,'2026-09-01',3,3,'balance','1.25','granted')`)
				if scenario == "already_applied" {
					activityExec(t, db, "INSERT INTO act_checkin_summaries(campaign_id,user_id,best_streak,checkin_count,last_checkin_date) VALUES (1,7,3,1,'2026-09-01')")
				}
				if scenario == "populated_missing_config" {
					activityExec(t, db, "ALTER TABLE act_campaigns DROP COLUMN config_json")
				} else {
					require.NoError(t, db.QueryRow("SELECT string_agg(config_json,',' ORDER BY id) FROM act_campaigns").Scan(&configBefore))
				}
				require.NoError(t, db.QueryRow("SELECT string_agg(banner_html,',' ORDER BY id) FROM act_campaigns").Scan(&bannerBefore))
				recordsBefore = activitySnapshot(t, db, "act_participation_records")
				checkinsBefore = activitySnapshot(t, db, "act_checkin_records")
			}
			if scenario != "fresh" {
				campaignsBefore = activitySnapshot(t, db, "act_campaigns")
			}
			if scenario == "already_applied" {
				activityExec(t, db, schemaMigrationsTableDDL)
				activityExec(t, db, "INSERT INTO schema_migrations(filename,checksum,applied_at) VALUES ($1,$2,'2026-09-01T00:00:00Z')", activityCenterMigration, activityCenterMigrationChecksum)
			}

			// First prove the unmodified SQL fails on the historical fixture.
			if scenario == "legacy_without_banner" || scenario == "populated_missing_config" || scenario == "participation_checkin_only" || scenario == "populated_with_config" {
				tx, err := db.Begin()
				require.NoError(t, err)
				_, err = tx.Exec(original)
				require.Error(t, err)
				if scenario == "legacy_without_banner" || scenario == "populated_missing_config" {
					require.Contains(t, err.Error(), `column "config_json"`)
				} else {
					require.Contains(t, err.Error(), "check constraint")
				}
				require.NoError(t, tx.Rollback())
			}

			for pass := 0; pass < 2; pass++ {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				err := applyMigrationsFS(ctx, db, files)
				cancel()
				require.NoError(t, err)
			}
			if scenario != "fresh" {
				require.Equal(t, campaignsBefore, activitySnapshot(t, db, "act_campaigns"))
			}
			if hasRecords {
				require.Equal(t, recordsBefore, activitySnapshot(t, db, "act_participation_records"))
				require.Equal(t, checkinsBefore, activitySnapshot(t, db, "act_checkin_records"))
				var count, streak int
				require.NoError(t, db.QueryRow("SELECT checkin_count,best_streak FROM act_checkin_summaries WHERE campaign_id=1 AND user_id=7").Scan(&count, &streak))
				require.Equal(t, 1, count)
				require.Equal(t, 3, streak)
			}
			if bannerBefore != "" {
				var after string
				require.NoError(t, db.QueryRow("SELECT string_agg(banner_html,',' ORDER BY id) FROM act_campaigns").Scan(&after))
				require.Equal(t, bannerBefore, after)
			}
			if scenario == "legacy_without_banner" || scenario == "populated_missing_config" {
				var defaults bool
				require.NoError(t, db.QueryRow("SELECT bool_and(config_json='{}') FROM act_campaigns").Scan(&defaults))
				require.True(t, defaults)
			}
			if configBefore != "" {
				var after string
				require.NoError(t, db.QueryRow("SELECT string_agg(config_json,',' ORDER BY id) FROM act_campaigns").Scan(&after))
				require.Equal(t, configBefore, after)
			}
			for _, column := range []string{"config_json", "banner_html"} {
				var notNull bool
				require.NoError(t, db.QueryRow("SELECT is_nullable='NO' FROM information_schema.columns WHERE table_schema='public' AND table_name='act_campaigns' AND column_name=$1", column).Scan(&notNull))
				require.True(t, notNull)
			}
			for _, table := range []string{"act_campaigns", "act_participation_records"} {
				var definition string
				var validated bool
				require.NoError(t, db.QueryRow("SELECT pg_get_constraintdef(oid), convalidated FROM pg_constraint WHERE conrelid=$1::regclass AND conname LIKE '%type'", table).Scan(&definition, &validated))
				require.True(t, validated)
				for _, kind := range []string{"lottery", "inflate", "redeem", "custom", "checkin", "external_link", "announcement"} {
					require.Contains(t, definition, "'"+kind+"'")
				}
			}
			var checksum string
			require.NoError(t, db.QueryRow("SELECT checksum FROM schema_migrations WHERE filename=$1", activityCenterMigration).Scan(&checksum))
			require.Equal(t, activityCenterMigrationChecksum, checksum)
			if scenario == "already_applied" {
				var unchanged bool
				require.NoError(t, db.QueryRow("SELECT applied_at='2026-09-01T00:00:00Z'::timestamptz FROM schema_migrations WHERE filename=$1", activityCenterMigration).Scan(&unchanged))
				require.True(t, unchanged)
			}
			_, err := db.Exec("INSERT INTO act_campaigns(id,title,type) VALUES (999999,'invalid','unknown_type')")
			require.ErrorContains(t, err, "chk_act_campaigns_type", "CHECK constraints must still enforce the allowed types")
		})
	}
}

func TestActivityCenterMigration_FailureRollsBack237(t *testing.T) {
	db := activityMigrationDB(t)
	legacy, err := os.ReadFile("testdata/activity_center_legacy_campaigns.sql")
	require.NoError(t, err)
	activityExec(t, db, string(legacy))
	activityExec(t, db, "ALTER TABLE act_campaigns DROP CONSTRAINT chk_act_campaigns_type; INSERT INTO act_campaigns(title,type) VALUES ('preserve-unknown','unknown_type')")
	before := activitySnapshot(t, db, "act_campaigns")
	files := activityMigrationFiles(t)
	for pass := 0; pass < 2; pass++ {
		err := applyMigrationsFS(context.Background(), db, files)
		require.ErrorContains(t, err, "apply migration "+activityCenterMigration)
		require.True(t, strings.Contains(err.Error(), "check constraint"), err)
		var completed, participationExists bool
		require.NoError(t, db.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename=$1), to_regclass('act_participation_records') IS NOT NULL", activityCenterMigration).Scan(&completed, &participationExists))
		require.False(t, completed)
		require.False(t, participationExists)
		require.Equal(t, before, activitySnapshot(t, db, "act_campaigns"))
	}
}

package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestConditionDetailPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_RESUBMIT_NOCHANGE_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_resubmit_nochange_test") {
		t.Fatal("non-test database rejected")
	}
	ctx := context.Background()
	r, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(r.Migrate(ctx))
	must(r.SeedPreviewIdentity(ctx, "test-admin-password", "test-worker-password"))
	var org, actor string
	must(r.db.Raw("SELECT id FROM organizations WHERE code='PREVIEW'").Scan(&org).Error)
	must(r.db.Raw("SELECT id FROM users WHERE organization_id=? AND role_key='admin' LIMIT 1", org).Scan(&actor).Error)
	item, err := r.CreateMasterItem(ctx, org, actor, "conditions", "0099", "A", "", "", 99, "", "")
	must(err)
	if item.Meaning != "" {
		t.Fatal("initial detail must be blank")
	}
	detail := "状態の詳細"
	updated, err := r.UpdateMasterItem(ctx, org, actor, "conditions", item.ID, nil, &detail, nil, nil, nil, nil, nil)
	must(err)
	if updated.ID != item.ID || updated.Code != item.Code || updated.Name != "A" || updated.Meaning != detail {
		t.Fatal("detail-only update changed identity or failed")
	}
	blank := ""
	updated, err = r.UpdateMasterItem(ctx, org, actor, "conditions", item.ID, nil, &blank, nil, nil, nil, nil, nil)
	must(err)
	if updated.Meaning != "" {
		t.Fatal("cannot clear detail")
	}
	if _, err = r.UpdateMasterItem(ctx, "other-org", actor, "conditions", item.ID, nil, &detail, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("cross-org update allowed")
	}
}

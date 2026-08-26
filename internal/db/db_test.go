package db

import (
	"path/filepath"
	"testing"
	"time"

	"hallo/internal/models"
)

func TestPlanAndUser(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	id, err := d.CreatePlan(models.Plan{Name: "admin", Note: "admin自用"})
	if err != nil {
		t.Fatal(err)
	}
	u := models.User{Email: "admin@local", UUID: "u", SubToken: "t", Enabled: true, PlanID: &id}
	if _, err := d.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	users, err := d.ActiveUsers()
	if err != nil || len(users) != 1 {
		t.Fatalf("active users: %v %v", users, err)
	}
}

func TestExpiredAndQuota(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	pid, err := d.CreatePlan(models.Plan{Name: "tiny", TrafficLimit: 10})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	expired := models.User{Email: "e@x", UUID: "u1", SubToken: "t1", Enabled: true, ExpireAt: &past}
	if _, err := d.CreateUser(expired); err != nil {
		t.Fatal(err)
	}
	over := models.User{Email: "o@x", UUID: "u2", SubToken: "t2", Enabled: true, PlanID: &pid, TrafficUp: 20}
	oid, err := d.CreateUser(over)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL.Exec(`UPDATE users SET traffic_up=20 WHERE id=?`, oid); err != nil {
		t.Fatal(err)
	}
	ok := models.User{Email: "ok@x", UUID: "u3", SubToken: "t3", Enabled: true}
	if _, err := d.CreateUser(ok); err != nil {
		t.Fatal(err)
	}
	act, err := d.ActiveUsers()
	if err != nil {
		t.Fatal(err)
	}
	if len(act) != 1 || act[0].Email != "ok@x" {
		t.Fatalf("active=%#v", act)
	}
}

func TestUserNodes(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	n1, err := d.CreateNode(models.Node{Name: "a", Token: "ta", Enabled: true, Subscribe: true, Port: 443})
	if err != nil {
		t.Fatal(err)
	}
	n2, err := d.CreateNode(models.Node{Name: "b", Token: "tb", Enabled: true, Subscribe: true, Port: 443})
	if err != nil {
		t.Fatal(err)
	}
	uid, err := d.CreateUser(models.User{Email: "u", UUID: "uu", SubToken: "st", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	all, err := d.UsersForNode(n1)
	if err != nil || len(all) != 1 {
		t.Fatalf("unrestricted: %v %v", all, err)
	}
	if err := d.SetUserNodes(uid, []int64{n2}); err != nil {
		t.Fatal(err)
	}
	only2, err := d.UsersForNode(n2)
	if err != nil || len(only2) != 1 {
		t.Fatalf("n2: %v %v", only2, err)
	}
	only1, err := d.UsersForNode(n1)
	if err != nil || len(only1) != 0 {
		t.Fatalf("n1 should be empty: %v %v", only1, err)
	}
}

func TestLocalNodeCannotDelete(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	id, err := d.CreateNode(models.Node{Name: "本机", Token: "x", IsLocal: true, Enabled: true, Subscribe: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteNode(id); err == nil {
		t.Fatal("expected error")
	}
}

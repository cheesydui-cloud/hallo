package db

import (
	"path/filepath"
	"testing"

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

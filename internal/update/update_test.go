package update

import "testing"

func TestNewer(t *testing.T) {
	if !Newer("v0.2.0", "v0.1.0") {
		t.Fatal("expected newer")
	}
	if Newer("v0.1.0", "v0.1.0") {
		t.Fatal("same should not be newer")
	}
	if !Newer("v0.1.1", "dev") {
		t.Fatal("dev should upgrade")
	}
}

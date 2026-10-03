package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		left  string
		right string
		want  int
	}{
		{"1.2.20", "1.2.19", 1},
		{"v1.2.19", "1.2.20", -1},
		{"1.3.0", "1.3.0", 0},
		{"1.2.21-beta.1", "1.2.21", 0},
	}
	for _, tc := range cases {
		got := CompareVersions(tc.left, tc.right)
		if got != tc.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tc.left, tc.right, got, tc.want)
		}
	}
}

func TestValidateSemver(t *testing.T) {
	if err := validateSemver("1.2.21"); err != nil {
		t.Fatalf("合法版本被拒绝: %v", err)
	}
	for _, value := range []string{"", "1.2", "main", "1.2.x"} {
		if err := validateSemver(value); err == nil {
			t.Fatalf("非法版本未被拒绝: %q", value)
		}
	}
}

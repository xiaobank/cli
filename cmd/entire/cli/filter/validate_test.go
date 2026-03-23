package filter

import (
	"testing"
)

func TestValidateFilter_BuiltIn_Valid(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "/home/user/project", Replace: "__ent__/repo"}
	if err := ValidateFilter(f, true); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateFilter_BuiltIn_WrongPrefix(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "/home/user/project", Replace: "__ent_user__/repo"}
	if err := ValidateFilter(f, true); err == nil {
		t.Error("expected error for wrong prefix on built-in filter")
	}
}

func TestValidateFilter_User_Valid(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "acme-corp.internal", Replace: "__ent_user__/hostname"}
	if err := ValidateFilter(f, false); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateFilter_User_WrongPrefix(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "acme-corp.internal", Replace: "__ent__/hostname"}
	if err := ValidateFilter(f, false); err == nil {
		t.Error("expected error for wrong prefix on user filter")
	}
}

func TestValidateFilter_User_TooShort(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "short", Replace: "__ent_user__/x"}
	if err := ValidateFilter(f, false); err == nil {
		t.Error("expected error for short match on user filter")
	}
}

func TestValidateFilter_BuiltIn_TooShort(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "/", Replace: "__ent__/repo"}
	if err := ValidateFilter(f, true); err == nil {
		t.Error("expected error for short match on built-in filter")
	}
	f2 := Filter{Match: "/ab", Replace: "__ent__/repo"}
	if err := ValidateFilter(f2, true); err == nil {
		t.Error("expected error for 3-char match on built-in filter")
	}
}

func TestValidateFilter_BuiltIn_MinLength(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "/abc", Replace: "__ent__/repo"}
	if err := ValidateFilter(f, true); err != nil {
		t.Errorf("unexpected error for 4-char built-in match: %v", err)
	}
}

func TestValidateFilter_EmptyMatch(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "", Replace: "__ent__/repo"}
	if err := ValidateFilter(f, true); err == nil {
		t.Error("expected error for empty match")
	}
}

func TestValidateFilter_EmptyReplace(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "/home/user", Replace: ""}
	if err := ValidateFilter(f, true); err == nil {
		t.Error("expected error for empty replace")
	}
}

func TestValidateFilter_SameMatchAndReplace(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "__ent__/repo", Replace: "__ent__/repo"}
	if err := ValidateFilter(f, true); err == nil {
		t.Error("expected error for same match and replace")
	}
}

func TestValidateFilter_ReplaceContainsMatch(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "foo", Replace: "__ent__/foo"}
	if err := ValidateFilter(f, true); err == nil {
		t.Error("expected error when replace contains match")
	}
}

func TestValidateFilter_MatchContainsReplace(t *testing.T) {
	t.Parallel()
	f := Filter{Match: "/home/__ent__/repo/project", Replace: "__ent__/repo"}
	if err := ValidateFilter(f, true); err == nil {
		t.Error("expected error when match contains replace")
	}
}

func TestValidateUserFilterKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "valid key", key: "hostname", wantErr: false},
		{name: "empty key", key: "", wantErr: true},
		{name: "key with slash", key: "foo/bar", wantErr: true},
		{name: "key with backslash", key: "foo\\bar", wantErr: true},
		{name: "key with __ent prefix", key: "__ent_something", wantErr: true},
		{name: "key with __ent__ prefix", key: "__ent__repo", wantErr: true},
		{name: "key that is just __ent", key: "__ent", wantErr: true},
		{name: "key starting with underscore", key: "_valid", wantErr: false},
		{name: "hyphenated key", key: "my-hostname", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateUserFilterKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateUserFilterKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

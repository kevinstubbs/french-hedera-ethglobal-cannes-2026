package naryo

import (
	"errors"
	"testing"
)

func TestIsNaryoRevisionHookFailure(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("something else"), false},
		{errors.New(`naryo: operation x failed: UNEXPECTED_ERROR RuntimeException: onAfterApply hook error`), true},
		{errors.New("OnAfterApply hook failed"), true},
		{errors.New("hook error in revision"), true},
		{errors.New("UNEXPECTED_ERROR foo"), false},
		{errors.New("UNEXPECTED_ERROR RuntimeException: other"), true},
	}
	for _, tc := range cases {
		if got := isNaryoRevisionHookFailure(tc.err); got != tc.want {
			t.Fatalf("isNaryoRevisionHookFailure(%v) = %v want %v", tc.err, got, tc.want)
		}
	}
}

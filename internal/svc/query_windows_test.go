//go:build windows

package svc

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/sys/windows"
)

func TestServiceLookupDoesNotMistakeAccessDeniedForAbsent(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{windows.ERROR_SERVICE_DOES_NOT_EXIST, "not-installed"},
		{fmt.Errorf("wrapped: %w", windows.ERROR_SERVICE_DOES_NOT_EXIST), "not-installed"},
		{windows.ERROR_ACCESS_DENIED, "unknown"},
		{windows.ERROR_INVALID_HANDLE, "unknown"},
		{windows.ERROR_SERVICE_MARKED_FOR_DELETE, "unknown"},
		{errors.New("fixture RPC unavailable"), "unknown"},
	} {
		if got := serviceLookupState(tc.err); got != tc.want {
			t.Fatalf("lookup=%s expected=%s", got, tc.want)
		}
	}
}

package logging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/require"
)

func TestErrorReasonOmitsPayloads(t *testing.T) {
	const secret = "private-message-marker"
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"plain", errors.New(secret), "*errors.errorString"},
		{"wrapped", fmt.Errorf("/schedule %s: %w", secret, errors.New(secret)), "*fmt.wrapError"},
		{"API details", &model.AppError{Id: secret, Message: secret, DetailedError: secret, RequestId: secret, StatusCode: 403}, "mattermost_error (HTTP 403)"},
		{"wrapped API", fmt.Errorf("%s: %w", secret, &model.AppError{Message: secret, StatusCode: 500}), "mattermost_error (HTTP 500)"},
		{"JSON type", &json.UnmarshalTypeError{Value: secret, Field: secret, Struct: secret}, "invalid_json"},
		{"time parsing", &time.ParseError{Value: secret, Message: secret}, "invalid_time"},
		{"canceled", fmt.Errorf("%s: %w", secret, context.Canceled), "canceled"},
		{"deadline", context.DeadlineExceeded, "deadline_exceeded"},
		{"not found", &fs.PathError{Op: secret, Path: secret, Err: fs.ErrNotExist}, "not_found"},
		{"permission", fs.ErrPermission, "permission_denied"},
		{"truncated input", io.ErrUnexpectedEOF, "incomplete_input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason := ErrorReason(tc.err)
			require.Equal(t, tc.want, reason)
			require.NotContains(t, reason, secret)
		})
	}
}

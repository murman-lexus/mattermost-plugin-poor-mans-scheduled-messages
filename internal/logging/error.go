// Package logging provides diagnostics that never include error payloads.
package logging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// ErrorReason describes an error without rendering its message or user data.
// API errors, including their IDs and details, can echo submitted post content.
// The log event describes the failed operation; this value adds a safe category.
func ErrorReason(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, pluginapi.ErrNotFound):
		return "not_found"
	case errors.Is(err, fs.ErrPermission):
		return "permission_denied"
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return "incomplete_input"
	}
	var appErr *model.AppError
	if errors.As(err, &appErr) && appErr != nil {
		return fmt.Sprintf("mattermost_error (HTTP %d)", appErr.StatusCode)
	}
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return "invalid_json"
	}
	var timeErr *time.ParseError
	if errors.As(err, &timeErr) {
		return "invalid_time"
	}
	// Type names are safe even when an unknown error's Error method prints
	// commands, stored JSON, HTTP bodies, or the original message.
	return fmt.Sprintf("%T", err)
}

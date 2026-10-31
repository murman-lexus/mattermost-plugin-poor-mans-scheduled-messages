package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/stretchr/testify/require"
)

type formEndpoint struct {
	author string
	input  types.ScheduleFormInput
	calls  int
	err    error
}

func (f *formEndpoint) Defaults(userID, channelID, rootID string) (*types.ScheduleFormDefaults, error) {
	f.author = userID
	f.calls++
	return &types.ScheduleFormDefaults{Timezone: "UTC", Destination: channelID, InThread: rootID != ""}, f.err
}
func (f *formEndpoint) Create(userID string, input types.ScheduleFormInput) (*types.ScheduledMessage, error) {
	f.author = userID
	f.input = input
	f.calls++
	return &types.ScheduledMessage{ID: "job-id"}, f.err
}
func TestFormEndpointsRequireAuthentication(t *testing.T) {
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/schedule/form"}, {"POST", "/api/v1/schedule"}} {
		f := &formEndpoint{}
		p := &Plugin{logger: testutil.FakeLogger{}, scheduleForm: f}
		w := httptest.NewRecorder()
		p.ServeHTTP(nil, w, httptest.NewRequest(route.method, route.path, strings.NewReader(`{"message":"private"}`)))
		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Zero(t, f.calls)
	}
}
func TestFormEndpointUsesServerAuthorAndDoesNotLogContent(t *testing.T) {
	f := &formEndpoint{}
	logger := &testutil.RecordingLogger{}
	p := &Plugin{logger: logger, scheduleForm: f}
	req := httptest.NewRequest("POST", "/api/v1/schedule?user_id=forged", strings.NewReader(`{"request_id":"nonce","channel_id":"channel","root_id":"thread","message":"private-marker","date":"2027-02-01","time":"13:10","timezone":"UTC"}`))
	req.Header.Set(constants.HTTPHeaderMattermostUserID, "author")
	w := httptest.NewRecorder()
	p.ServeHTTP(nil, w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "author", f.author)
	require.Equal(t, "private-marker", f.input.Message)
	require.NotContains(t, logger.String(), "private-marker")
	require.NotContains(t, w.Body.String(), "private-marker")
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
}
func TestFormRejectsUnknownFieldsTrailingJSONAndOversizedRequests(t *testing.T) {
	for _, body := range []string{`{"user_id":"forged","message":"secret"}`, `{} {}`, `{"message":"` + strings.Repeat("x", constants.MaxMessageBytes*6+4096) + `"}`} {
		f := &formEndpoint{}
		p := &Plugin{logger: testutil.FakeLogger{}, scheduleForm: f}
		req := httptest.NewRequest("POST", "/api/v1/schedule", strings.NewReader(body))
		req.Header.Set(constants.HTTPHeaderMattermostUserID, "author")
		w := httptest.NewRecorder()
		p.ServeHTTP(nil, w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Zero(t, f.calls)
	}
}
func TestFormReturnsSafeTechnicalErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{errors.New("private-marker"), 500, "storage"}, {&types.ScheduleFormError{Code: "access", Field: "channel_id"}, 403, "access"}, {&types.ScheduleFormError{Code: "conflict"}, 409, "conflict"}} {
		f := &formEndpoint{err: tc.err}
		logger := &testutil.RecordingLogger{}
		p := &Plugin{logger: logger, scheduleForm: f}
		req := httptest.NewRequest("POST", "/api/v1/schedule", strings.NewReader(`{"message":"private-marker"}`))
		req.Header.Set(constants.HTTPHeaderMattermostUserID, "author")
		w := httptest.NewRecorder()
		p.ServeHTTP(nil, w, req)
		require.Equal(t, tc.status, w.Code)
		var result struct{ Error struct{ Code string } }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		require.Equal(t, tc.code, result.Error.Code)
		require.NotContains(t, w.Body.String(), "private-marker")
		require.NotContains(t, logger.String(), "private-marker")
	}
}

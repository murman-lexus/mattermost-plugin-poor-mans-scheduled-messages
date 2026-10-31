package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mock"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/command"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAutocompleteUsesAuthenticatedUserAndOmitsQueryFromLogs(t *testing.T) {
	users := mock.NewMockUserService(gomock.NewController(t))
	users.EXPECT().Get("authenticated-user").Return(&model.User{Timezone: map[string]string{"manualTimezone": "Europe/Kaliningrad"}}, nil).Times(1)
	prefs := testutil.DisplayPreferences(func(id string) types.DisplaySettings {
		require.Equal(t, "authenticated-user", id)
		return types.DisplaySettings{Locale: "ru", Use24Hour: true}
	})
	logger := &testutil.RecordingLogger{}
	p := &Plugin{logger: logger, autocompleter: command.NewAutocompleter(users, prefs, testutil.FakeClock{NowTime: time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)})}
	query := url.Values{"user_id": {"other-user"}, "parsed": {"schedule at "}, "user_input": {"schedule at "}, "message": {"private-marker"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete/time?"+query.Encode(), nil)
	req.Header.Set(constants.HTTPHeaderMattermostUserID, "authenticated-user")
	w := httptest.NewRecorder()
	p.ServeHTTP(nil, w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	var items []model.AutocompleteListItem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	require.NotEmpty(t, items)
	require.Contains(t, items[0].HelpText, "Europe/Kaliningrad")
	require.NotContains(t, logger.String(), "private-marker")
	require.NotContains(t, logger.String(), "schedule at")
	require.NotContains(t, w.Body.String(), "private-marker")
}

func TestAutocompleteRequiresAuthentication(t *testing.T) {
	p := &Plugin{logger: testutil.FakeLogger{}}
	w := httptest.NewRecorder()
	p.ServeHTTP(nil, w, httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete/time?user_id=claimed-user", nil))
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

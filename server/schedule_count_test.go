package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mock"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestChannelScheduleCountRequiresAuthenticationAndValidChannel(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		p := &Plugin{logger: testutil.FakeLogger{}}
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/v1/schedule/count?channel_id=invalid", nil)
		if authenticated {
			req.Header.Set(constants.HTTPHeaderMattermostUserID, "author")
		}
		p.ServeHTTP(nil, w, req)
		if authenticated {
			require.Equal(t, 400, w.Code)
		} else {
			require.Equal(t, 401, w.Code)
		}
	}
}
func TestChannelScheduleCountIsPrivateAndScopedToAuthor(t *testing.T) {
	st := mock.NewMockStore(gomock.NewController(t))
	channel := model.NewId()
	ids := []string{"pending", "failed", "other-channel", "foreign", "sent", "canceled", "missing"}
	st.EXPECT().ListUserMessageIDs("author").Return(ids, nil)
	for _, id := range ids {
		if id == "missing" {
			st.EXPECT().GetScheduledMessage(id).Return(nil, types.ErrMessageNotFound)
			continue
		}
		msg := &types.ScheduledMessage{ID: id, UserID: "author", ChannelID: channel, MessageContent: "private-marker", State: types.StatePending}
		switch id {
		case "failed":
			msg.State = types.StateFailed
		case "foreign":
			msg.UserID = "another-author"
		case "other-channel":
			msg.ChannelID = model.NewId()
		case "sent":
			msg.State = types.StateSent
		case "canceled":
			msg.State = types.StateCancelled
		}
		st.EXPECT().GetScheduledMessage(id).Return(msg, nil)
	}
	logger := &testutil.RecordingLogger{}
	p := &Plugin{Store: st, logger: logger}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/schedule/count?channel_id="+channel+"&user_id=another-author", nil)
	req.Header.Set(constants.HTTPHeaderMattermostUserID, "author")
	p.ServeHTTP(nil, w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var result map[string]int
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Equal(t, map[string]int{"count": 2}, result)
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	require.NotContains(t, w.Body.String(), "private-marker")
	require.NotContains(t, logger.String(), "private-marker")
}
func TestChannelScheduleCountDoesNotExposeStorageErrors(t *testing.T) {
	st := mock.NewMockStore(gomock.NewController(t))
	st.EXPECT().ListUserMessageIDs("author").Return(nil, errors.New("private-marker"))
	p := &Plugin{Store: st, logger: testutil.FakeLogger{}}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/schedule/count?channel_id="+model.NewId(), nil)
	req.Header.Set(constants.HTTPHeaderMattermostUserID, "author")
	p.ServeHTTP(nil, w, req)
	require.Equal(t, 500, w.Code)
	require.NotContains(t, w.Body.String(), "private-marker")
}
func TestListActionRetainsOnlyItsOwnChannelScope(t *testing.T) {
	for _, tc := range []struct {
		scope  string
		scoped bool
	}{{"channel", true}, {"different", false}, {"", false}} {
		req := &model.PostActionIntegrationRequest{ChannelId: "channel", Context: map[string]any{"list_channel_id": tc.scope}}
		args := listArgsForAction("author", req)
		require.Equal(t, "author", args.UserId)
		if tc.scoped {
			require.Equal(t, "/schedule list channel", args.Command)
			require.Equal(t, "channel", args.ChannelId)
		} else {
			require.Empty(t, args.Command)
			require.Empty(t, args.ChannelId)
		}
	}
}

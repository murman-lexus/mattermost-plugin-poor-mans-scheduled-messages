package scheduler

import (
	"net/http"
	"testing"
	"time"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mm"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mock"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/ports"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/store"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestProcessDueMessages_RevokedPermissions(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		deleteAt   int64
		userErr    *model.AppError
		memberErr  *model.AppError
		readDenied bool
		reason     string
	}{
		{name: "disabled account", deleteAt: 1, reason: "author's account is deactivated"},
		{name: "removed from private channel", memberErr: model.NewAppError("test", "missing_member", nil, "", http.StatusNotFound), reason: "author is no longer a member"},
		{name: "restricted posting", reason: "author does not have permission to post"},
		{name: "content access revoked", readDenied: true, reason: "author no longer has access"},
		{name: "account lookup unavailable", userErr: model.NewAppError("test", "unavailable", nil, "", http.StatusServiceUnavailable), reason: "cannot verify the author's account"},
		{name: "membership lookup rate limited", memberErr: model.NewAppError("test", "rate_limit", nil, "", http.StatusTooManyRequests), reason: "cannot verify the author's membership"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := testutil.NewMemoryKV()
			st := store.NewKVStore(testutil.FakeLogger{}, &pluginapi.NewClient(db, nil).KV, nil, 100)
			poster := mock.NewMockPostService(ctrl)
			ch := mock.NewMockChannelService(ctrl)
			api := &plugintest.API{}
			api.On("GetUser", "author").Return(&model.User{DeleteAt: scenario.deleteAt}, scenario.userErr).Once()
			if scenario.deleteAt == 0 && scenario.userErr == nil {
				api.On("GetChannelMember", "private-channel", "author").Return(&model.ChannelMember{}, scenario.memberErr).Once()
				if scenario.memberErr == nil {
					api.On("GetServerVersion").Return("9.0.0").Once()
					api.On("HasPermissionToChannel", "author", "private-channel", model.PermissionReadChannelContent).Return(!scenario.readDenied).Once()
					if scenario.readDenied {
						api.On("HasPermissionToChannel", "author", "private-channel", model.PermissionReadChannel).Return(true).Maybe()
						api.On("HasPermissionToChannel", "author", "private-channel", model.PermissionCreatePost).Return(true).Maybe()
					} else {
						api.On("HasPermissionToChannel", "author", "private-channel", model.PermissionCreatePost).Return(false).Once()
					}
				}
			}
			guarded := mm.NewScheduledPostService(pluginapi.NewClient(api, nil))
			guarded.PostService = poster
			clk := testutil.FakeClock{NowTime: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}
			msg := &types.ScheduledMessage{
				ID: "scheduled", UserID: "author", ChannelID: "private-channel",
				PostAt: clk.Now().Add(-time.Minute), MessageContent: "original text",
			}
			require.NoError(t, st.SaveScheduledMessage(msg.UserID, msg))
			info := &ports.ChannelInfo{ChannelID: msg.ChannelID}
			ch.EXPECT().GetInfoOrUnknown(msg.ChannelID).Return(info)
			ch.EXPECT().MakeChannelLink(info, gomock.Any()).Return("destination")
			poster.EXPECT().DM("bot", msg.UserID, gomock.Any()).DoAndReturn(func(_, _ string, post *model.Post) error {
				assert.Contains(t, post.Message, scenario.reason)
				assert.Contains(t, post.Message, msg.MessageContent)
				return nil
			})
			// No CreatePost call is allowed after authorization fails.
			s := New(testutil.FakeLogger{}, guarded, st, ch, "bot", clk, nil)
			s.processDueMessages()
			saved, err := st.GetScheduledMessage(msg.ID)
			require.NoError(t, err)
			require.Equal(t, types.StateFailed, saved.State)
			require.Equal(t, "publication_blocked", saved.LastReason)
			require.Equal(t, msg.MessageContent, saved.MessageContent)
			require.NoError(t, st.DeleteScheduledMessage(msg.UserID, msg.ID))
			api.AssertExpectations(t)
		})
	}
}

func TestBlockedDeliveryCanBeExplicitlyRetriedAfterAccessRestored(t *testing.T) {
	s, st, _, poster, msg, _ := setupDelivery(t)
	api := &plugintest.API{}
	guarded := mm.NewScheduledPostService(pluginapi.NewClient(api, nil))
	guarded.PostService = poster
	s.poster = guarded
	api.On("GetUser", msg.UserID).Return(&model.User{DeleteAt: 1}, nil).Once()
	require.ErrorIs(t, s.SendNow(msg), types.ErrPublicationBlocked)
	saved, err := st.GetScheduledMessage(msg.ID)
	require.NoError(t, err)
	require.Equal(t, types.StateFailed, saved.State)
	s.processDueMessages()
	api.AssertNumberOfCalls(t, "GetUser", 1)
	api.On("GetUser", msg.UserID).Return(&model.User{}, nil).Once()
	api.On("GetChannelMember", msg.ChannelID, msg.UserID).Return(&model.ChannelMember{}, nil).Once()
	api.On("GetServerVersion").Return("9.0.0").Once()
	api.On("HasPermissionToChannel", msg.UserID, msg.ChannelID, model.PermissionReadChannelContent).Return(true).Once()
	api.On("HasPermissionToChannel", msg.UserID, msg.ChannelID, model.PermissionCreatePost).Return(true).Once()
	poster.EXPECT().CreatePost(gomock.Any()).DoAndReturn(func(post *model.Post) error {
		post.Id = "published"
		return nil
	}).Times(1)
	require.NoError(t, s.SendNow(msg))
	saved, err = st.GetScheduledMessage(msg.ID)
	require.NoError(t, err)
	require.Equal(t, types.StateSent, saved.State)
	require.Equal(t, "published", saved.PostID)
	api.AssertExpectations(t)
}

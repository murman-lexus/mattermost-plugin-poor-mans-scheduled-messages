package scheduler

import (
	"net/http"
	"testing"
	"time"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mm"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mock"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/ports"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestProcessDueMessages_RevokedPermissions(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		deleteAt   int64
		memberErr  *model.AppError
		readDenied bool
		reason     string
	}{
		{name: "disabled account", deleteAt: 1, reason: "author's account is deactivated"},
		{name: "removed from private channel", memberErr: model.NewAppError("test", "missing_member", nil, "", http.StatusNotFound), reason: "author is no longer a member"},
		{name: "restricted posting", reason: "author does not have permission to post"},
		{name: "content access revoked", readDenied: true, reason: "author no longer has access"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := mock.NewMockStore(ctrl)
			poster := mock.NewMockPostService(ctrl)
			ch := mock.NewMockChannelService(ctrl)
			api := &plugintest.API{}
			api.On("GetUser", "author").Return(&model.User{DeleteAt: scenario.deleteAt}, nil).Once()
			if scenario.deleteAt == 0 {
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
			st.EXPECT().ListScheduledMessages().Return([]*types.ScheduledMessage{msg}, nil)
			st.EXPECT().DeleteScheduledMessage(msg.UserID, msg.ID).Return(nil)
			info := &ports.ChannelInfo{ChannelID: msg.ChannelID}
			ch.EXPECT().GetInfoOrUnknown(msg.ChannelID).Return(info)
			ch.EXPECT().MakeChannelLink(info).Return("destination")
			poster.EXPECT().DM("bot", msg.UserID, gomock.Any()).DoAndReturn(func(_, _ string, post *model.Post) error {
				assert.Contains(t, post.Message, scenario.reason)
				assert.Contains(t, post.Message, msg.MessageContent)
				return nil
			})
			// No CreatePost call is allowed after authorization fails.
			s := New(testutil.FakeLogger{}, guarded, st, ch, "bot", clk)
			s.processDueMessages()
			api.AssertExpectations(t)
		})
	}
}

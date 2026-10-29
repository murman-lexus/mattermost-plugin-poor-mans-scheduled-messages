package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mock"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/ports"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type deliveryTestAPI struct {
	plugintest.API
}

func (*deliveryTestAPI) LogDebug(string, ...any) {}
func (*deliveryTestAPI) LogInfo(string, ...any)  {}
func (*deliveryTestAPI) LogWarn(string, ...any)  {}
func (*deliveryTestAPI) LogError(string, ...any) {}

// Exercise production wiring as well as the Send button's error notification.
func TestDelivery_RevokedPermissions(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		reason string
		setup  func(*plugintest.API)
	}{
		{"disabled account", "author's account is deactivated", func(api *plugintest.API) {
			api.On("GetUser", "author").Return(&model.User{Id: "author", DeleteAt: 1}, nil).Once()
		}},
		{"removed from private channel", "author is no longer a member of the destination channel", func(api *plugintest.API) {
			api.On("GetUser", "author").Return(&model.User{Id: "author"}, nil).Once()
			api.On("GetChannelMember", "private-channel", "author").Return(nil, model.NewAppError("test", "missing_member", nil, "", http.StatusNotFound)).Once()
		}},
		{"channel access revoked", "author no longer has access to the destination channel", func(api *plugintest.API) {
			api.On("GetUser", "author").Return(&model.User{Id: "author"}, nil).Once()
			api.On("GetChannelMember", "private-channel", "author").Return(&model.ChannelMember{}, nil).Once()
			api.On("HasPermissionToChannel", "author", "private-channel", model.PermissionReadChannel).Return(true).Maybe()
			api.On("HasPermissionToChannel", "author", "private-channel", model.PermissionCreatePost).Return(true).Maybe()
			api.On("HasPermissionToChannel", "author", "private-channel", model.PermissionReadChannelContent).Return(false).Once()
		}},
		{"restricted posting", "author does not have permission to post in the destination channel", func(api *plugintest.API) {
			api.On("GetUser", "author").Return(&model.User{Id: "author"}, nil).Once()
			api.On("GetChannelMember", "private-channel", "author").Return(&model.ChannelMember{}, nil).Once()
			api.On("HasPermissionToChannel", "author", "private-channel", model.PermissionReadChannelContent).Return(true).Once()
			api.On("HasPermissionToChannel", "author", "private-channel", model.PermissionCreatePost).Return(false).Once()
		}},
	} {
		for _, viaButton := range []bool{false, true} {
			path := "scheduler"
			if viaButton {
				path = "Send button"
			}
			t.Run(scenario.name+"/"+path, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				st := mock.NewMockStore(ctrl)
				ch := mock.NewMockChannelService(ctrl)
				api := &deliveryTestAPI{}
				api.On("GetServerVersion").Return("9.0.0").Maybe()
				cli := pluginapi.NewClient(api, nil)
				scenario.setup(&api.API)
				msg := &types.ScheduledMessage{
					ID: "scheduled", UserID: "author", ChannelID: "private-channel",
					MessageContent: "original text", PostAt: time.Now(), Timezone: "UTC",
				}
				st.EXPECT().DeleteScheduledMessage(msg.UserID, msg.ID).Return(nil)
				info := &ports.ChannelInfo{ChannelID: msg.ChannelID}
				ch.EXPECT().GetInfoOrUnknown(msg.ChannelID).Return(info)
				ch.EXPECT().MakeChannelLink(info).Return("destination")
				api.On("GetDirectChannel", "bot", "author").Return(&model.Channel{Id: "dm"}, nil).Once()
				api.On("CreatePost", testifymock.MatchedBy(func(post *model.Post) bool {
					return post.UserId == "bot" && post.ChannelId == "dm" &&
						strings.Contains(post.Message, scenario.reason) && strings.Contains(post.Message, msg.MessageContent)
				})).Return(&model.Post{}, nil).Once()
				s := (prodBuilder{}).NewScheduler(cli, st, ch, "bot", testutil.FakeClock{})
				if viaButton {
					postMock := mock.NewMockPostService(ctrl)
					cmd := &mockCommand{
						UserSendMessageFunc: func(userID, msgID string) (*types.ScheduledMessage, error) {
							require.Equal(t, msg.UserID, userID)
							require.Equal(t, msg.ID, msgID)
							return msg, nil
						},
						BuildEphemeralListFunc: func(*model.CommandArgs) *model.CommandResponse {
							return &model.CommandResponse{Props: map[string]any{}}
						},
					}
					postMock.EXPECT().UpdateEphemeralPost(msg.UserID, gomock.Any())
					postMock.EXPECT().SendEphemeralPost(msg.UserID, gomock.Any()).Do(func(_ string, post *model.Post) {
						assert.Contains(t, post.Message, scenario.reason)
						assert.Equal(t, "list-channel", post.ChannelId)
					})
					p := &Plugin{logger: testutil.FakeLogger{}, poster: postMock, Scheduler: s, Command: cmd}
					rr := httptest.NewRecorder()
					p.ServeHTTP(nil, rr, createSendRequest(t, msg.UserID, "list-post", "list-channel", "send", msg.ID))
					assert.Contains(t, rr.Body.String(), scenario.reason)
					assert.GreaterOrEqual(t, rr.Code, 400)
				} else {
					require.ErrorContains(t, s.SendNow(msg), scenario.reason)
				}
				api.AssertExpectations(t)
			})
		}
	}
}

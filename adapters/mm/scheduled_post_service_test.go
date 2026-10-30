package mm

import (
	"net/http"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/require"
)

func TestScheduledPostService_ContentAccessRevoked(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetUser", "author").Return(&model.User{Id: "author"}, nil).Once()
	api.On("GetChannelMember", "channel", "author").Return(&model.ChannelMember{}, nil).Once()
	api.On("GetServerVersion").Return("9.0.0").Maybe()
	// Metadata access and posting rights remain, but message access is revoked.
	api.On("HasPermissionToChannel", "author", "channel", model.PermissionReadChannel).Return(true).Maybe()
	api.On("HasPermissionToChannel", "author", "channel", model.PermissionCreatePost).Return(true).Maybe()
	api.On("HasPermissionToChannel", "author", "channel", model.PermissionReadChannelContent).Return(false).Once()
	post := &model.Post{UserId: "author", ChannelId: "channel", Message: "text"}
	api.On("CreatePost", post).Return(&model.Post{}, nil).Maybe()
	service := NewScheduledPostService(pluginapi.NewClient(api, nil))
	require.ErrorContains(t, service.CreatePost(post), "author no longer has access")
	api.AssertNotCalled(t, "CreatePost", post)
	api.AssertExpectations(t)
}

func TestScheduledPostService_RechecksPermissions(t *testing.T) {
	for _, tc := range []struct {
		version        string
		rootID         string
		readPermission *model.Permission
	}{
		{"6.2.1", "", model.PermissionReadChannel},
		{"8.1.0", "", model.PermissionReadChannel},
		{"9.0.0", "", model.PermissionReadChannelContent},
		{"9.0.0-rc1", "", model.PermissionReadChannelContent},
		{"11.1.0", "thread-root", model.PermissionReadChannelContent},
	} {
		t.Run(tc.version+"/root="+tc.rootID, func(t *testing.T) {
			api := &plugintest.API{}
			service := NewScheduledPostService(pluginapi.NewClient(api, nil))
			post := &model.Post{UserId: "author", ChannelId: "channel", RootId: tc.rootID, Message: "text"}
			api.On("GetUser", "author").Return(&model.User{Id: "author"}, nil).Twice()
			api.On("GetChannelMember", "channel", "author").Return(&model.ChannelMember{}, nil).Twice()
			api.On("GetServerVersion").Return(tc.version).Twice()
			api.On("HasPermissionToChannel", "author", "channel", tc.readPermission).Return(true).Twice()
			api.On("HasPermissionToChannel", "author", "channel", model.PermissionCreatePost).Return(true).Once()
			api.On("CreatePost", post).Return(&model.Post{
				UserId: post.UserId, ChannelId: post.ChannelId, RootId: post.RootId, Message: post.Message,
			}, nil).Once()
			require.NoError(t, service.CreatePost(post))

			// Rights change between two attempts using the same service and author.
			api.On("HasPermissionToChannel", "author", "channel", model.PermissionCreatePost).Return(false).Once()
			require.ErrorContains(t, service.CreatePost(post), "does not have permission to post")
			api.AssertNumberOfCalls(t, "CreatePost", 1)
			api.AssertExpectations(t)
		})
	}
}

func TestScheduledPostService_UnknownServerVersionBlocksPosting(t *testing.T) {
	api := &plugintest.API{}
	api.On("GetUser", "author").Return(&model.User{}, nil).Once()
	api.On("GetChannelMember", "channel", "author").Return(&model.ChannelMember{}, nil).Once()
	api.On("GetServerVersion").Return("").Once()
	service := NewScheduledPostService(pluginapi.NewClient(api, nil))
	require.ErrorContains(t, service.CreatePost(&model.Post{UserId: "author", ChannelId: "channel"}), "cannot determine the server version")
	api.AssertNotCalled(t, "CreatePost")
	api.AssertExpectations(t)
}

func TestScheduledPostService_LookupFailuresBlockPosting(t *testing.T) {
	lookupErr := model.NewAppError("test", "unavailable", nil, "", http.StatusInternalServerError)
	for _, tc := range []struct {
		name      string
		user      *model.User
		userErr   *model.AppError
		memberErr *model.AppError
		reason    string
	}{
		{name: "user lookup error", userErr: lookupErr, reason: "cannot verify the author's account"},
		{name: "missing user", reason: "author's account was not found"},
		{name: "membership lookup error", user: &model.User{}, memberErr: lookupErr, reason: "cannot verify the author's membership"},
		{name: "missing member", user: &model.User{}, reason: "author is no longer a member"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &plugintest.API{}
			api.On("GetUser", "author").Return(tc.user, tc.userErr).Once()
			if tc.user != nil {
				api.On("GetChannelMember", "channel", "author").Return(nil, tc.memberErr).Once()
			}
			service := NewScheduledPostService(pluginapi.NewClient(api, nil))
			require.ErrorContains(t, service.CreatePost(&model.Post{UserId: "author", ChannelId: "channel"}), tc.reason)
			api.AssertNotCalled(t, "CreatePost")
			api.AssertExpectations(t)
		})
	}
}

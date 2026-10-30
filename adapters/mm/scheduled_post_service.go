package mm

import (
	"errors"
	"fmt"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/ports"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// ScheduledPostService checks the author's current access before publishing.
// Bot DMs and ephemeral notifications use the underlying PostService directly.
type ScheduledPostService struct {
	ports.PostService
	client *pluginapi.Client
}

// NewScheduledPostService wraps the privileged plugin API for scheduled delivery.
func NewScheduledPostService(client *pluginapi.Client) *ScheduledPostService {
	return &ScheduledPostService{PostService: &client.Post, client: client}
}

// CreatePost checks live account, membership and channel permissions on every call.
// The plugin's CreatePost API does not enforce the author's permissions itself.
func (s *ScheduledPostService) CreatePost(post *model.Post) error {
	if err := s.checkAuthorAccess(post); err != nil {
		return fmt.Errorf("%w: %w", types.ErrPublicationBlocked, err)
	}
	return s.PostService.CreatePost(post)
}

func (s *ScheduledPostService) checkAuthorAccess(post *model.Post) error {
	user, err := s.client.User.Get(post.UserId)
	if err != nil {
		return fmt.Errorf("cannot verify the author's account: %w", err)
	}
	if user == nil {
		return errors.New("author's account was not found")
	}
	if user.DeleteAt != 0 {
		return errors.New("author's account is deactivated")
	}

	member, err := s.client.Channel.GetMember(post.ChannelId, post.UserId)
	if errors.Is(err, pluginapi.ErrNotFound) || (err == nil && member == nil) {
		return errors.New("author is no longer a member of the destination channel")
	}
	if err != nil {
		return fmt.Errorf("cannot verify the author's membership in the destination channel: %w", err)
	}
	major, _, _ := model.SplitVersion(s.client.System.GetServerVersion())
	if major <= 0 {
		return errors.New("cannot determine the server version to verify the author's channel access")
	}
	readPermission := model.PermissionReadChannelContent
	// Full content-access validation is supported on Mattermost 9.0 and later.
	// Older servers use read_channel as a best-effort check; permission changes
	// backported to older release branches are outside this compatibility policy.
	if major < 9 {
		readPermission = model.PermissionReadChannel
	}
	if !s.client.User.HasPermissionToChannel(post.UserId, post.ChannelId, readPermission) {
		return errors.New("author no longer has access to the destination channel")
	}
	if !s.client.User.HasPermissionToChannel(post.UserId, post.ChannelId, model.PermissionCreatePost) {
		return errors.New("author does not have permission to post in the destination channel")
	}
	return nil
}

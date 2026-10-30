package scheduler

import (
	"errors"
	"testing"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mock"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/ports"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/store"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDeliveryFailureLogsOmitConversationContent(t *testing.T) {
	const secret = "private-message-marker"
	ctrl := gomock.NewController(t)
	poster := mock.NewMockPostService(ctrl)
	logger := &testutil.RecordingLogger{}
	db := testutil.NewMemoryKV()
	st := store.NewKVStore(logger, &pluginapi.NewClient(db, nil).KV, nil, 100)
	ch := mock.NewMockChannelService(ctrl)
	msg := &types.ScheduledMessage{ID: "job-id", UserID: "author", ChannelID: "channel", MessageContent: secret}
	require.NoError(t, st.SaveScheduledMessage(msg.UserID, msg))
	postErr := &model.AppError{Message: secret, DetailedError: secret, StatusCode: 403}
	poster.EXPECT().CreatePost(gomock.Any()).Return(postErr)
	info := &ports.ChannelInfo{ChannelID: msg.ChannelID}
	ch.EXPECT().GetInfoOrUnknown(msg.ChannelID).Return(info)
	ch.EXPECT().MakeChannelLink(info, gomock.Any()).Return("destination")
	poster.EXPECT().DM("bot", msg.UserID, gomock.Any()).DoAndReturn(func(_, _ string, post *model.Post) error {
		// The private notification still contains the original message.
		require.Contains(t, post.Message, secret)
		return errors.New("failed DM body: " + post.Message)
	})
	s := New(logger, poster, st, ch, "bot", testutil.FakeClock{}, nil)
	require.ErrorIs(t, s.SendNow(msg), postErr)
	logs := logger.String()
	require.NotContains(t, logs, secret)
	require.Contains(t, logs, "job-id")
	require.Contains(t, logs, "HTTP 403")
	require.Contains(t, logs, "WARN")
	require.Contains(t, logs, "ERROR")
	require.Contains(t, logs, "publication_rejected_403")
	saved, err := st.GetScheduledMessage(msg.ID)
	require.NoError(t, err)
	require.Equal(t, types.StateFailed, saved.State)
	require.Equal(t, secret, saved.MessageContent)
}

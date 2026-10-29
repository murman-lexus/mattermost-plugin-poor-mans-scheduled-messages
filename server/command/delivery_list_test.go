package command

import (
	"testing"
	"time"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mock"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/ports"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDeliveryStateControlsVisibleActions(t *testing.T) {
	for _, tc := range []struct {
		state   types.DeliveryState
		actions []string
	}{
		{types.StateCreating, []string{"delete"}},
		{types.StatePending, []string{"send", "delete"}},
		{types.StateProcessing, []string{"delete"}},
		{types.StateSending, nil}, {types.StateReconciling, nil},
		{types.StateUncertain, []string{"send"}},
		{types.StateRetry, []string{"send", "delete"}},
		{types.StateFailed, []string{"send", "delete"}},
		{types.StateSent, nil}, {types.StateCancelled, nil},
	} {
		t.Run(string(tc.state), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := mock.NewMockStore(ctrl)
			ch := mock.NewMockChannelService(ctrl)
			msg := &types.ScheduledMessage{ID: "job", UserID: "user", ChannelID: "channel", State: tc.state, Attempts: 2, LastReason: "rate_limited", MessageContent: "text", Timezone: "invalid/timezone", PostAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
			st.EXPECT().ListUserMessageIDs("user").Return([]string{msg.ID}, nil)
			st.EXPECT().GetScheduledMessage(msg.ID).Return(msg, nil)
			if msg.Terminal() {
				st.EXPECT().CleanupMessageFromUserIndex(msg.UserID, msg.ID).Return(nil)
			} else {
				info := &ports.ChannelInfo{ChannelID: msg.ChannelID}
				ch.EXPECT().GetInfoOrUnknown(msg.ChannelID).Return(info)
				ch.EXPECT().MakeChannelLink(info).Return("channel")
			}
			resp := NewListService(testutil.FakeLogger{}, st, ch).Build(msg.UserID)
			if msg.Terminal() {
				require.Equal(t, constants.EmptyListMessage, resp.Text)
				return
			}
			atts := resp.Props["attachments"].([]*model.MessageAttachment)
			require.Len(t, atts, 1)
			require.Contains(t, atts[0].Text, string(tc.state))
			require.Contains(t, atts[0].Text, "attempts 2/5")
			var ids []string
			for _, a := range atts[0].Actions {
				ids = append(ids, a.Id)
			}
			require.Equal(t, tc.actions, ids)
			if tc.state == types.StateUncertain {
				require.Equal(t, "Check delivery", atts[0].Actions[0].Name)
			}
		})
	}
}

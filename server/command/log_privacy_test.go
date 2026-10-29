package command

import (
	"errors"
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

func TestSchedulingLogsOmitConversationContent(t *testing.T) {
	const secret = "private-message-marker"
	for _, scenario := range []string{"success", "invalid command", "invalid time", "store failure", "validation failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := mock.NewMockStore(ctrl)
			user := mock.NewMockUserService(ctrl)
			ch := mock.NewMockChannelService(ctrl)
			logger := &testutil.RecordingLogger{}
			text := "at 5pm message " + secret
			if scenario == "invalid command" {
				text = "invalid " + secret
			}
			if scenario == "invalid time" {
				text = "at 99pm message " + secret
			}
			if scenario == "validation failure" {
				st.EXPECT().ListUserMessageIDs("author").Return(nil, errors.New("storage echoed "+secret))
			} else {
				st.EXPECT().ListUserMessageIDs("author").Return(nil, nil)
			}
			if scenario != "invalid command" && scenario != "validation failure" {
				user.EXPECT().Get("author").Return(&model.User{}, nil)
			}
			if scenario == "success" || scenario == "store failure" {
				st.EXPECT().GenerateMessageID().Return("job-id")
				var saveErr error
				if scenario == "store failure" {
					saveErr = &model.AppError{Id: secret, Message: secret, DetailedError: secret, StatusCode: 500}
				}
				st.EXPECT().SaveScheduledMessage("author", gomock.Any()).DoAndReturn(func(_ string, msg *types.ScheduledMessage) error {
					require.Equal(t, secret, msg.MessageContent)
					return saveErr
				})
				info := &ports.ChannelInfo{ChannelID: "channel"}
				ch.EXPECT().GetInfoOrUnknown("channel").Return(info)
				ch.EXPECT().MakeChannelLink(info).Return("destination")
			}
			clk := testutil.FakeClock{NowTime: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)}
			svc := NewScheduleService(logger, user, st, ch, clk, constants.MaxUserMessages)
			h := NewHandler(logger, nil, user, st, ch, nil, svc, "help")
			args := &model.CommandArgs{UserId: "author", ChannelId: "channel", Command: "/schedule " + text}
			resp, err := h.Execute(args)
			require.Nil(t, err)
			require.NotNil(t, resp)
			logs := logger.String()
			require.NotContains(t, logs, secret)
			require.NotContains(t, logs, args.Command)
			require.NotContains(t, logs, text)
			if scenario == "success" || scenario == "store failure" {
				require.Contains(t, logs, "job-id")
			}
		})
	}
}

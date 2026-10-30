package store

import (
	"testing"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mock"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRecoveryReadFailureLogsOmitStoredContent(t *testing.T) {
	const secret = "private-stored-message-marker"
	ctrl := gomock.NewController(t)
	kv := mock.NewMockKVService(ctrl)
	logger := &testutil.RecordingLogger{}
	key := constants.SchedPrefix + "job-id"
	kv.EXPECT().ListKeys(0, constants.MaxFetchScheduledMessages).Return([]string{key}, nil)
	kv.EXPECT().Get(key, gomock.Any()).Return(&model.AppError{
		Id: secret, Message: secret, DetailedError: secret, StatusCode: 500,
	})
	st := NewKVStore(logger, kv, nil, 100)
	jobs, err := st.ListScheduledMessages()
	require.NoError(t, err)
	require.Empty(t, jobs)
	logs := logger.String()
	require.NotContains(t, logs, secret)
	require.Contains(t, logs, "job-id")
	require.Contains(t, logs, "HTTP 500")
	require.Contains(t, logs, "WARN")
}

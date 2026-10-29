package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/require"
)

type privateCommand struct{ mockCommand }

func (*privateCommand) Execute(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	return nil, &model.AppError{Id: args.Command, Message: args.Command, DetailedError: args.Command, StatusCode: 400}
}

func TestExecuteCommandLogsOmitCommandAndAPIErrorPayload(t *testing.T) {
	logger := &testutil.RecordingLogger{}
	p := &Plugin{logger: logger, Command: &privateCommand{}}
	args := &model.CommandArgs{UserId: "author", ChannelId: "channel", Command: "/schedule at 5pm message private-message-marker"}
	_, err := p.ExecuteCommand(nil, args)
	require.NotNil(t, err)
	logs := logger.String()
	require.NotContains(t, logs, "private-message-marker")
	require.NotContains(t, logs, args.Command)
	require.Contains(t, logs, "HTTP 400")
}

func TestHTTPLogsOmitRequestPayloads(t *testing.T) {
	const secret = "private-message-marker"
	for _, action := range []string{"send", "delete"} {
		for _, invalidType := range []bool{false, true} {
			t.Run(action+"/invalid-type="+map[bool]string{false: "false", true: "true"}[invalidType], func(t *testing.T) {
				logger := &testutil.RecordingLogger{}
				p := &Plugin{logger: logger}
				body := map[string]any{"context": map[string]any{"action": secret, "id": "job-id", "message": secret, "command": "/schedule " + secret}}
				if invalidType {
					body["user_id"] = map[string]string{secret: secret}
				}
				encoded, err := json.Marshal(body)
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/"+action+"?message="+secret, strings.NewReader(string(encoded)))
				req.Header.Set(constants.HTTPHeaderMattermostUserID, "author")
				rr := httptest.NewRecorder()
				p.ServeHTTP(nil, rr, req)
				require.Equal(t, http.StatusBadRequest, rr.Code)
				logs := logger.String()
				require.NotContains(t, logs, secret)
				require.Contains(t, logs, "error_reason")
				if !invalidType {
					require.Contains(t, logs, "job-id")
				}
			})
		}
	}
}

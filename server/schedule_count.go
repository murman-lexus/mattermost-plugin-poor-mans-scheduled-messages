package main

import (
	"errors"
	"net/http"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/mattermost/mattermost/server/public/model"
)

// countScheduledMessages exposes only the caller's active jobs in one channel.
// It does not require publication rights: failed jobs can still be inspected.
func (p *Plugin) countScheduledMessages(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get(constants.HTTPHeaderMattermostUserID)
	channelID := r.URL.Query().Get("channel_id")
	if !model.IsValidId(channelID) {
		p.writeFormError(w, userID, &types.ScheduleFormError{Code: "channel", Field: "channel_id"})
		return
	}
	ids, err := p.Store.ListUserMessageIDs(userID)
	if err != nil {
		p.writeFormError(w, userID, err)
		return
	}
	count := 0
	for _, id := range ids {
		msg, err := p.Store.GetScheduledMessage(id)
		if errors.Is(err, types.ErrMessageNotFound) {
			continue
		}
		if err != nil {
			p.writeFormError(w, userID, err)
			return
		}
		if msg != nil && msg.ID != "" && msg.UserID == userID && msg.ChannelID == channelID && !msg.Terminal() {
			count++
		}
	}
	writeFormJSON(w, http.StatusOK, map[string]int{"count": count})
}

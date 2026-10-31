package command

import (
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/mattermost/mattermost/server/public/model"
)

func userTimezone(user *model.User) string {
	if user == nil {
		return constants.DefaultTimezone
	}
	if user.Timezone["useAutomaticTimezone"] == "true" && user.Timezone["automaticTimezone"] != "" {
		return user.Timezone["automaticTimezone"]
	}
	if user.Timezone["manualTimezone"] != "" {
		return user.Timezone["manualTimezone"]
	}
	return constants.DefaultTimezone
}

package main

import (
	"encoding/json"
	"net/http"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/logging"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
)

func (p *Plugin) commandAutocomplete(w http.ResponseWriter, r *http.Request) {
	// Use the authenticated header, not the caller-supplied query's user_id.
	// Mattermost includes the current user header when proxying autocomplete.
	userID := r.Header.Get(constants.HTTPHeaderMattermostUserID)
	items := []model.AutocompleteListItem{}
	if p.autocompleter != nil {
		query := r.URL.Query()
		items = p.autocompleter.Items(userID, mux.Vars(r)["kind"], query.Get("user_input"), query.Get("parsed"))
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		p.logger.Warn("Cannot encode command suggestions", "error_reason", logging.ErrorReason(err))
	}
}

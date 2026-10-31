package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/formatter"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
)

func (p *Plugin) scheduleFormDefaults(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get(constants.HTTPHeaderMattermostUserID)
	settings, err := p.scheduleForm.Defaults(userID, r.URL.Query().Get("channel_id"), r.URL.Query().Get("root_id"))
	if err != nil {
		p.writeFormError(w, userID, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeFormJSON(w, http.StatusOK, settings)
}

func (p *Plugin) createScheduledMessage(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get(constants.HTTPHeaderMattermostUserID)
	r.Body = http.MaxBytesReader(w, r.Body, int64(constants.MaxMessageBytes*6+4096))
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input types.ScheduleFormInput
	if err := decoder.Decode(&input); err != nil {
		p.writeFormError(w, userID, &types.ScheduleFormError{Code: "request"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		p.writeFormError(w, userID, &types.ScheduleFormError{Code: "request"})
		return
	}
	msg, err := p.scheduleForm.Create(userID, input)
	if err != nil {
		p.writeFormError(w, userID, err)
		return
	}
	p.logger.Info("Scheduled message created through form", "user_id", userID, "message_id", msg.ID)
	writeFormJSON(w, http.StatusOK, map[string]string{"id": msg.ID})
}

func (p *Plugin) writeFormError(w http.ResponseWriter, userID string, err error) {
	code, field, status := "storage", "", http.StatusInternalServerError
	var formErr *types.ScheduleFormError
	if errors.As(err, &formErr) {
		code, field = formErr.Code, formErr.Field
	}
	switch code {
	case "access":
		status = http.StatusForbidden
	case "conflict", "processed", "pending":
		status = http.StatusConflict
	case "storage", "settings":
	default:
		status = http.StatusBadRequest
	}
	r := formatter.ForUser(p.presentation, userID)
	writeFormJSON(w, status, map[string]any{"error": map[string]string{"code": code, "field": field, "message": r.Text("form_error_" + code)}})
}

func writeFormJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

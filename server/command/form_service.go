package command

import (
	"errors"
	"strings"
	"time"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/ports"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/formatter"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/google/uuid"
	"github.com/mattermost/mattermost/server/public/model"
)

// FormService validates structured input and uses the existing durable store.
type FormService struct {
	users       ports.UserService
	preferences ports.DisplayPreferences
	store       ports.Store
	access      ports.PublicationAccess
	posts       ports.PostReader
	channels    ports.ChannelService
	clock       ports.Clock
}

// NewFormService constructs the server-side form workflow.
func NewFormService(users ports.UserService, preferences ports.DisplayPreferences, store ports.Store, access ports.PublicationAccess, posts ports.PostReader, channels ports.ChannelService, clock ports.Clock) *FormService {
	return &FormService{users: users, preferences: preferences, store: store, access: access, posts: posts, channels: channels, clock: clock}
}

func formError(code, field string) error { return &types.ScheduleFormError{Code: code, Field: field} }

func (s *FormService) validateDestination(userID, channelID, rootID string) error {
	if !model.IsValidId(channelID) {
		return formError("channel", "channel_id")
	}
	if err := s.access.CheckAuthorAccess(userID, channelID); err != nil {
		return formError("access", "channel_id")
	}
	if rootID != "" {
		if !model.IsValidId(rootID) {
			return formError("thread", "root_id")
		}
		post, err := s.posts.GetPost(rootID)
		if err != nil || post == nil || post.ChannelId != channelID || post.RootId != "" || post.DeleteAt != 0 {
			return formError("thread", "root_id")
		}
	}
	return nil
}

// Defaults resolves the user's time zone and validates the captured destination.
func (s *FormService) Defaults(userID, channelID, rootID string) (*types.ScheduleFormDefaults, error) {
	if err := s.validateDestination(userID, channelID, rootID); err != nil {
		return nil, err
	}
	user, err := s.users.Get(userID)
	if err != nil || user == nil {
		return nil, formError("settings", "")
	}
	loc, err := time.LoadLocation(userTimezone(user))
	if err != nil || loc.String() == "Local" {
		loc = time.UTC
	}
	r := formatter.ForUser(s.preferences, userID)
	next := s.clock.Now().In(loc).Truncate(15 * time.Minute).Add(15 * time.Minute)
	return &types.ScheduleFormDefaults{
		Locale: r.Locale, Use24Hour: r.Use24Hour, Timezone: loc.String(), Date: next.Format(constants.DateParseLayoutYYYYMMDD), Time: next.Format("15:04"),
		Destination: s.channels.MakeChannelLink(s.channels.GetInfoOrUnknown(channelID), r.DisplaySettings), InThread: rootID != "",
		MaxMessageBytes: constants.MaxMessageBytes, Labels: formLabels(r),
	}, nil
}

// Create enforces ownership, destination access, quotas, and idempotent retries.
func (s *FormService) Create(userID string, input types.ScheduleFormInput) (*types.ScheduledMessage, error) {
	requestID, err := uuid.Parse(input.RequestID)
	if err != nil || requestID == uuid.Nil || requestID.String() != input.RequestID {
		return nil, formError("request", "request_id")
	}
	if strings.TrimSpace(input.Message) == "" {
		return nil, formError("empty", "message")
	}
	if len(input.Message) > constants.MaxMessageBytes {
		return nil, formError("length", "message")
	}
	when, err := resolveFormTime(input.Date, input.Time, input.Timezone)
	if err != nil {
		return nil, err
	}
	if err := s.validateDestination(userID, input.ChannelID, input.RootID); err != nil {
		return nil, err
	}
	// Fixed-length, user-scoped IDs preserve the store's atomic create semantics
	// and existing pending-post marker length. No client-supplied author is used.
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(userID+":"+input.RequestID)).String()
	if existing, err := s.store.GetScheduledMessage(id); err == nil && existing != nil {
		return matchFormRetry(existing, userID, input, when)
	} else if err != nil && !errors.Is(err, types.ErrMessageNotFound) {
		return nil, formError("storage", "")
	}
	if !when.After(s.clock.Now()) {
		return nil, formError("past", "time")
	}
	msg := &types.ScheduledMessage{ID: id, UserID: userID, ChannelID: input.ChannelID, RootID: input.RootID, MessageContent: input.Message, PostAt: when.UTC(), Timezone: input.Timezone}
	if err := s.store.SaveScheduledMessage(userID, msg); err != nil {
		if errors.Is(err, types.ErrMessageLimit) {
			return nil, formError("limit", "message")
		}
		// Another identical request may have won the atomic creation, or a
		// transient write failure may have left a durable creating record.
		if saved, loadErr := s.store.GetScheduledMessage(id); loadErr == nil && saved != nil {
			return matchFormRetry(saved, userID, input, when)
		}
		return nil, formError("storage", "")
	}
	return msg, nil
}

func matchFormRetry(msg *types.ScheduledMessage, userID string, input types.ScheduleFormInput, when time.Time) (*types.ScheduledMessage, error) {
	if msg.UserID != userID || msg.ChannelID != input.ChannelID || msg.RootID != input.RootID || !msg.PostAt.Equal(when) || msg.Timezone != input.Timezone {
		return nil, formError("conflict", "")
	}
	if msg.Terminal() {
		return nil, formError("processed", "")
	}
	if msg.MessageContent != input.Message {
		return nil, formError("conflict", "")
	}
	if msg.State == types.StateCreating {
		return nil, formError("pending", "")
	}
	return msg, nil
}

// Resolve wall time on the server. Reject DST gaps and ambiguous repeated times
// instead of silently changing the hour or guessing which occurrence to use.
func resolveFormTime(date, clock, zone string) (time.Time, error) {
	if zone == "" || zone == "Local" {
		return time.Time{}, formError("timezone", "timezone")
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, formError("timezone", "timezone")
	}
	day, err := time.Parse(constants.DateParseLayoutYYYYMMDD, date)
	if err != nil {
		return time.Time{}, formError("date", "date")
	}
	tod, err := parseTimeStr(normalizeTime(clock), loc)
	if err != nil {
		return time.Time{}, formError("time", "time")
	}
	wall := time.Date(day.Year(), day.Month(), day.Day(), tod.Hour(), tod.Minute(), 0, 0, time.UTC)
	candidate := time.Date(day.Year(), day.Month(), day.Day(), tod.Hour(), tod.Minute(), 0, 0, loc)
	matches := make(map[int64]time.Time)
	for _, delta := range []time.Duration{-24 * time.Hour, 0, 24 * time.Hour} {
		_, offset := candidate.Add(delta).Zone()
		guess := wall.Add(-time.Duration(offset) * time.Second).In(loc)
		if guess.Format("2006-01-02 15:04") == wall.Format("2006-01-02 15:04") {
			matches[guess.Unix()] = guess
		}
	}
	if len(matches) == 0 {
		return time.Time{}, formError("time", "time")
	}
	if len(matches) > 1 {
		return time.Time{}, formError("ambiguous", "time")
	}
	for _, resolved := range matches {
		return resolved, nil
	}
	return time.Time{}, formError("time", "time")
}

func formLabels(r formatter.Renderer) map[string]string {
	labels := make(map[string]string)
	for _, key := range []string{"title", "message", "date", "time", "timezone", "destination", "thread", "submit", "cancel", "close", "loading", "success", "error", "unsupported", "draft_changed", "retry"} {
		labels[key] = r.Text("form_" + key)
	}
	return labels
}

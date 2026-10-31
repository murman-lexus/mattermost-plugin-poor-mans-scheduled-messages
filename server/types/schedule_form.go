package types

// ScheduleFormInput describes wall-clock time in an explicit IANA time zone.
// RequestID is reused when retrying the same form submission.
type ScheduleFormInput struct {
	RequestID string `json:"request_id"`
	ChannelID string `json:"channel_id"`
	RootID    string `json:"root_id"`
	Message   string `json:"message"`
	Date      string `json:"date"`
	Time      string `json:"time"`
	Timezone  string `json:"timezone"`
}

// ScheduleFormDefaults contains recipient-specific presentation and context.
type ScheduleFormDefaults struct {
	Locale          string            `json:"locale"`
	Use24Hour       bool              `json:"use_24_hour"`
	Timezone        string            `json:"timezone"`
	Date            string            `json:"date"`
	Time            string            `json:"time"`
	Destination     string            `json:"destination"`
	InThread        bool              `json:"in_thread"`
	MaxMessageBytes int               `json:"max_message_bytes"`
	Labels          map[string]string `json:"labels"`
}

// ScheduleFormError exposes a stable error code and field, without payloads.
type ScheduleFormError struct{ Code, Field string }

func (e *ScheduleFormError) Error() string { return e.Code }

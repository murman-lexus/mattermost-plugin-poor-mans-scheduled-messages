package command

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mock"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/ports"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/store"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/google/uuid"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type formAccess struct{ err error }

func (a *formAccess) CheckAuthorAccess(_, _ string) error { return a.err }

type formPosts struct{ post *model.Post }

func (p formPosts) GetPost(string) (*model.Post, error) { return p.post, nil }

func formFixture(t *testing.T) (*FormService, ports.Store, *formAccess, types.ScheduleFormInput) {
	t.Helper()
	st := store.NewKVStore(testutil.FakeLogger{}, &pluginapi.NewClient(testutil.NewMemoryKV(), nil).KV, nil, 100)
	access := &formAccess{}
	input := types.ScheduleFormInput{RequestID: uuid.NewString(), ChannelID: model.NewId(), Message: "private message", Date: "2027-02-01", Time: "13:10", Timezone: "Europe/Kaliningrad"}
	s := NewFormService(nil, nil, st, access, formPosts{}, nil, testutil.FakeClock{NowTime: time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)})
	return s, st, access, input
}

func requireFormCode(t *testing.T, err error, code string) {
	t.Helper()
	var target *types.ScheduleFormError
	require.ErrorAs(t, err, &target)
	require.Equal(t, code, target.Code)
}

func TestFormCreatesDurableJobAndIdempotentRetries(t *testing.T) {
	s, st, _, in := formFixture(t)
	first, err := s.Create("author", in)
	require.NoError(t, err)
	require.Equal(t, time.Date(2027, 2, 1, 11, 10, 0, 0, time.UTC), first.PostAt)
	saved, err := st.GetScheduledMessage(first.ID)
	require.NoError(t, err)
	require.Equal(t, types.StatePending, saved.State)
	again, err := s.Create("author", in)
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID)
	in.Message = "edited"
	_, err = s.Create("author", in)
	requireFormCode(t, err, "conflict")
	ids, err := st.ListUserMessageIDs("author")
	require.NoError(t, err)
	require.Equal(t, []string{first.ID}, ids)
	in.Message = "private message"
	other, err := s.Create("another-author", in)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, other.ID)
}

func TestFormConcurrentRetriesCreateOneJob(t *testing.T) {
	s, st, _, in := formFixture(t)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Create("author", in); results <- err }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			requireFormCode(t, err, "pending")
		}
	}
	ids, err := st.ListUserMessageIDs("author")
	require.NoError(t, err)
	require.Len(t, ids, 1)
	_, err = s.Create("author", in)
	require.NoError(t, err)
}

func TestFormRejectsInvalidOrUnauthorizedInput(t *testing.T) {
	tests := []struct {
		name, code string
		edit       func(*types.ScheduleFormInput)
	}{
		{"empty", "empty", func(i *types.ScheduleFormInput) { i.Message = " \n\t" }},
		{"bytes", "length", func(i *types.ScheduleFormInput) { i.Message = strings.Repeat("я", 30000) }},
		{"request", "request", func(i *types.ScheduleFormInput) { i.RequestID = uuid.Nil.String() }},
		{"channel", "channel", func(i *types.ScheduleFormInput) { i.ChannelID = "bad" }},
		{"timezone", "timezone", func(i *types.ScheduleFormInput) { i.Timezone = "Local" }},
		{"date", "date", func(i *types.ScheduleFormInput) { i.Date = "02/01/2027" }},
		{"clock", "time", func(i *types.ScheduleFormInput) { i.Time = "25:00" }},
		{"past", "past", func(i *types.ScheduleFormInput) { i.Date = "2020-01-01" }},
		{"thread", "thread", func(i *types.ScheduleFormInput) { i.RootID = model.NewId() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, st, _, in := formFixture(t)
			tt.edit(&in)
			_, err := s.Create("author", in)
			requireFormCode(t, err, tt.code)
			ids, err := st.ListUserMessageIDs("author")
			require.NoError(t, err)
			require.Empty(t, ids)
		})
	}
	s, _, access, in := formFixture(t)
	access.err = errors.New("disabled or removed or read-only")
	_, err := s.Create("author", in)
	requireFormCode(t, err, "access")
}

func TestFormValidatesThreadAndRetainsRoot(t *testing.T) {
	s, _, _, in := formFixture(t)
	in.RootID = model.NewId()
	for _, post := range []*model.Post{{ChannelId: model.NewId()}, {ChannelId: in.ChannelID, RootId: model.NewId()}, {ChannelId: in.ChannelID, DeleteAt: 1}} {
		s.posts = formPosts{post}
		_, err := s.Create("author", in)
		requireFormCode(t, err, "thread")
	}
	s.posts = formPosts{&model.Post{Id: in.RootID, ChannelId: in.ChannelID}}
	msg, err := s.Create("author", in)
	require.NoError(t, err)
	require.Equal(t, in.RootID, msg.RootID)
}

func TestFormWallTimeAndDST(t *testing.T) {
	for _, tt := range []struct{ date, clock, zone, code, want string }{
		{"2027-02-01", "1:10 PM", "Europe/Kaliningrad", "", "2027-02-01T11:10:00Z"},
		{"2027-02-01", "00:00", "Asia/Kathmandu", "", "2027-01-31T18:15:00Z"},
		{"2027-03-28", "02:30", "Europe/Berlin", "time", ""},
		{"2026-11-01", "01:30", "America/New_York", "ambiguous", ""},
		{"2027-04-04", "01:45", "Australia/Lord_Howe", "ambiguous", ""},
	} {
		t.Run(tt.date+tt.zone, func(t *testing.T) {
			got, err := resolveFormTime(tt.date, tt.clock, tt.zone)
			if tt.code != "" {
				requireFormCode(t, err, tt.code)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got.UTC().Format(time.RFC3339))
			}
		})
	}
}

func TestFormDefaultsUsePersonalSettings(t *testing.T) {
	for _, military := range []bool{false, true} {
		s, _, _, in := formFixture(t)
		ctrl := gomock.NewController(t)
		users := mock.NewMockUserService(ctrl)
		users.EXPECT().Get("author").Return(&model.User{Timezone: map[string]string{"manualTimezone": "Europe/Kaliningrad"}}, nil)
		channels := mock.NewMockChannelService(ctrl)
		info := &ports.ChannelInfo{ChannelID: in.ChannelID}
		channels.EXPECT().GetInfoOrUnknown(in.ChannelID).Return(info)
		channels.EXPECT().MakeChannelLink(info, types.DisplaySettings{Locale: "ru", Use24Hour: military}).Return("town-square")
		s.users = users
		s.channels = channels
		s.preferences = testutil.DisplayPreferences(func(string) types.DisplaySettings { return types.DisplaySettings{Locale: "ru", Use24Hour: military} })
		out, err := s.Defaults("author", in.ChannelID, "")
		require.NoError(t, err)
		require.Equal(t, military, out.Use24Hour)
		require.Equal(t, "10:15", out.Time)
		require.Equal(t, "Europe/Kaliningrad", out.Timezone)
		require.Equal(t, "Сообщение", out.Labels["message"])
	}
}

func TestFormRetryDoesNotAcknowledgeIncompleteOrTerminalCreation(t *testing.T) {
	for _, tc := range []struct {
		state types.DeliveryState
		code  string
	}{{types.StateCreating, "pending"}, {types.StateSent, "processed"}, {types.StateCancelled, "processed"}} {
		t.Run(string(tc.state), func(t *testing.T) {
			s, _, _, in := formFixture(t)
			id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("author:"+in.RequestID)).String()
			when, err := resolveFormTime(in.Date, in.Time, in.Timezone)
			require.NoError(t, err)
			st := mock.NewMockStore(gomock.NewController(t))
			s.store = st
			st.EXPECT().GetScheduledMessage(id).Return(&types.ScheduledMessage{ID: id, UserID: "author", ChannelID: in.ChannelID, MessageContent: in.Message, PostAt: when, Timezone: in.Timezone, State: tc.state}, nil)
			_, err = s.Create("author", in)
			requireFormCode(t, err, tc.code)
		})
	}
}

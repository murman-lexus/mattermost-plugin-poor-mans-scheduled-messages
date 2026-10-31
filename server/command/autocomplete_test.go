package command

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/adapters/mock"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/testutil"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/types"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func autocompleteFixture(t *testing.T, now time.Time, zone string, military bool) *Autocompleter {
	t.Helper()
	users := mock.NewMockUserService(gomock.NewController(t))
	users.EXPECT().Get("user").Return(&model.User{Timezone: map[string]string{"manualTimezone": zone}}, nil).AnyTimes()
	prefs := testutil.DisplayPreferences(func(userID string) types.DisplaySettings {
		require.Equal(t, "user", userID)
		return types.DisplaySettings{Locale: "ru", Use24Hour: military}
	})
	return NewAutocompleter(users, prefs, testutil.FakeClock{NowTime: now})
}

func TestAutocompleteDefinitionMatchesCommandGrammar(t *testing.T) {
	data := (&Handler{}).getScheduleAutocompleteData()
	require.NoError(t, data.IsValid())
	require.Equal(t, []string{"at", "list", "help"}, []string{data.SubCommands[0].Trigger, data.SubCommands[1].Trigger, data.SubCommands[2].Trigger})
	at := data.SubCommands[0]
	require.Len(t, at.Arguments, 4)
	require.True(t, at.Arguments[0].Required)
	require.False(t, at.Arguments[1].Required)
	require.Equal(t, model.AutocompleteArgTypeDynamicList, at.Arguments[1].Type)
	message := at.Arguments[2].Data.(*model.AutocompleteStaticListArg)
	require.Equal(t, "message", message.PossibleArguments[0].Item)
	require.Equal(t, model.AutocompleteArgTypeText, at.Arguments[3].Type)
	base, _ := url.Parse("/plugins/com.mattermost.plugin-poor-mans-scheduled-messages")
	require.NoError(t, data.UpdateRelativeURLsForPluginCommands(base))
	for i, kind := range []string{"time", "date"} {
		require.Equal(t, base.Path+"/api/v1/autocomplete/"+kind, at.Arguments[i].Data.(*model.AutocompleteDynamicListArg).FetchURL)
	}
}

func TestTimeSuggestionsRespectClockAndTimezone(t *testing.T) {
	for _, military := range []bool{false, true} {
		t.Run(map[bool]string{false: "12h", true: "24h"}[military], func(t *testing.T) {
			a := autocompleteFixture(t, time.Date(2026, 10, 29, 23, 50, 0, 0, time.UTC), "Europe/Kaliningrad", military)
			items := a.Items("user", "time", "schedule at ", "schedule at ")
			require.NotEmpty(t, items)
			want := map[bool]string{false: "1:30PM", true: "13:30"}[military]
			found := false
			for _, item := range items {
				if item.Item == want {
					found = true
				}
				require.Contains(t, item.HelpText, "Europe/Kaliningrad")
				parsed, err := parseScheduleInput("at " + item.Item + " message hello")
				require.NoError(t, err)
				_, err = resolveScheduledTime(parsed.TimeStr, "", a.clock.Now().In(testutil.MustLoadLocation(t, "Europe/Kaliningrad")), testutil.MustLoadLocation(t, "Europe/Kaliningrad"))
				require.NoError(t, err)
			}
			require.True(t, found)
			require.Contains(t, items[0].HelpText, "30 октября 2026")
		})
	}
}

func TestDateSuggestionsUseLocalDayAndExcludePastTimes(t *testing.T) {
	a := autocompleteFixture(t, time.Date(2026, 10, 29, 23, 50, 0, 0, time.UTC), "Europe/Kaliningrad", true)
	for _, tc := range []struct{ clock, first, label string }{
		{"13:30", "on 2026-10-30", "Сегодня"},
		{"00:30", "on 2026-10-31", "Завтра"},
	} {
		items := a.Items("user", "date", "schedule at "+tc.clock+" on ", "schedule at "+tc.clock+" ")
		require.NotEmpty(t, items)
		require.Equal(t, tc.first, items[0].Item)
		require.Contains(t, items[0].HelpText, tc.label)
		loc := testutil.MustLoadLocation(t, "Europe/Kaliningrad")
		for _, item := range items {
			parsed, err := parseScheduleInput("at " + tc.clock + " " + item.Item + " message hello")
			require.NoError(t, err)
			when, err := resolveScheduledTime(parsed.TimeStr, parsed.DateStr, a.clock.Now().In(loc), loc)
			require.NoError(t, err)
			require.True(t, when.After(a.clock.Now()))
		}
	}
}

func TestAutocompletePreservesManualValuesAndMessagePrivacy(t *testing.T) {
	a := autocompleteFixture(t, time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC), "UTC", true)
	for _, value := range []string{"10:17", "1:42PM", "01:42 PM", "00:30", "10:17   ", "01:42 \t PM   "} {
		items := a.Items("user", "time", "schedule at "+value+" message private-marker", "schedule at ")
		require.Len(t, items, 1)
		require.Equal(t, value, items[0].Item)
	}
	for _, value := range []string{"2050-01-01", "fri", "25dec", "2050-01-01   "} {
		items := a.Items("user", "date", "schedule at 10:17 on "+value+" message private-marker", "schedule at 10:17 ")
		require.NotEmpty(t, items)
		require.Equal(t, "on "+value, items[0].Item)
		for _, item := range items {
			require.NotContains(t, item.Item+item.HelpText, "private-marker")
		}
	}
	// Neither a completed time nor the message branch needs account lookups.
	noAPIs := NewAutocompleter(nil, nil, nil)
	require.NotEmpty(t, noAPIs.Items("user", "time", "schedule at 10:17 message text", "schedule at "))
	require.Empty(t, noAPIs.Items("user", "date", "schedule at 10:17 message text", "schedule at 10:17 "))
}

func TestSuggestedManualTimeCanBeScheduled(t *testing.T) {
	now := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		want  time.Time
	}{
		{"1:42\tPM", time.Date(2026, 10, 30, 13, 42, 0, 0, time.UTC)},
		{"01:42 \t pM", time.Date(2026, 10, 30, 13, 42, 0, 0, time.UTC)},
		{"09:05\tAM", time.Date(2026, 10, 30, 9, 5, 0, 0, time.UTC)},
		{"00:30", time.Date(2026, 10, 31, 0, 30, 0, 0, time.UTC)},
	} {
		t.Run(tc.value, func(t *testing.T) {
			a := autocompleteFixture(t, now, "UTC", true)
			items := a.Items("user", "time", "schedule at "+tc.value+" message body", "schedule at ")
			require.Len(t, items, 1)
			svc, mocks := setupScheduleServiceTest(t)
			mocks.clock.NowTime = now
			mocks.store.EXPECT().ListUserMessageIDs(testUserID).Return(nil, nil)
			mocks.userAPI.EXPECT().Get(testUserID).Return(&model.User{}, nil)
			mocks.store.EXPECT().GenerateMessageID().Return("job")
			mocks.store.EXPECT().SaveScheduledMessage(testUserID, gomock.Any()).DoAndReturn(func(_ string, msg *types.ScheduledMessage) error {
				require.Equal(t, tc.want, msg.PostAt)
				require.Equal(t, "body\nsecond line", msg.MessageContent)
				return nil
			})
			mocks.channel.EXPECT().GetInfoOrUnknown(testChannelID).Return(nil)
			mocks.channel.EXPECT().MakeChannelLink(nil, gomock.Any()).Return("destination")
			response := svc.Build(defaultArgs(), "at "+items[0].Item+" message body\nsecond line")
			require.Contains(t, response.Text, "✅")
		})
	}
}

func TestAutocompleteFallbacks(t *testing.T) {
	users := mock.NewMockUserService(gomock.NewController(t))
	users.EXPECT().Get("missing").Return(nil, nil)
	users.EXPECT().Get("unavailable").Return(nil, errors.New("private-marker"))
	a := NewAutocompleter(users, nil, testutil.FakeClock{})
	for _, id := range []string{"missing", "unavailable"} {
		require.Empty(t, a.Items(id, "time", "schedule at ", "schedule at "))
	}
	a = autocompleteFixture(t, time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC), "invalid/zone", true)
	for _, item := range a.Items("user", "time", "schedule at ", "schedule at ") {
		require.True(t, strings.HasSuffix(item.HelpText, "(UTC)"))
	}
	require.Empty(t, a.Items("user", "unknown", "", ""))
	require.Empty(t, a.Items("user", "time", "wrong", "schedule at "))
}

func TestStepByStepSuggestionsFollowMattermostListContract(t *testing.T) {
	a := autocompleteFixture(t, time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC), "UTC", true)
	args := (&Handler{}).getScheduleAutocompleteData().SubCommands[0].Arguments
	for _, tc := range []struct{ input, want string }{
		{"schedule at ", "schedule at 13:30"},
		{"schedule at 13:30 ", "schedule at 13:30 message"},
		{"schedule at 13:30 ", "schedule at 13:30 on 2026-10-30"},
		{"schedule at 13:30 on ", "schedule at 13:30 on 2026-10-31"},
		{"schedule at 13:30 on 2026-10-31 ", "schedule at 13:30 on 2026-10-31 message"},
		{"schedule at 10:17 on fri ", "schedule at 10:17 on fri message"},
		{"schedule at 1:42 PM ", "schedule at 1:42 PM message"},
	} {
		t.Run(tc.input+tc.want, func(t *testing.T) {
			got := followAutocompleteArguments(a, args, "schedule at ", strings.TrimPrefix(tc.input, "schedule at "))
			require.Contains(t, got, tc.want)
		})
	}
}

func TestExtraSpacesDoNotInterruptSuggestions(t *testing.T) {
	a := autocompleteFixture(t, time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC), "UTC", true)
	args := (&Handler{}).getScheduleAutocompleteData().SubCommands[0].Arguments
	for _, tc := range []struct{ input, want string }{
		{"schedule at  10:17 ", "schedule at 10:17 message"},
		{"schedule at    10:17 ", "schedule at 10:17 message"},
		{"schedule at 10:17    ", "schedule at 10:17 message"},
		{"schedule at 10:17 on  ", "schedule at 10:17 on 2026-10-30"},
		{"schedule at 10:17 on    2050-01-01 ", "schedule at 10:17 on 2050-01-01 message"},
		{"schedule at 10:17 on 2050-01-01    ", "schedule at 10:17 on 2050-01-01 message"},
		{"schedule at   01:42 PM   on   fri   ", "schedule at 01:42 PM on fri message"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got := followAutocompleteArguments(a, args, "schedule at ", strings.TrimPrefix(tc.input, "schedule at "))
			var normalized []string
			for _, complete := range got {
				normalized = append(normalized, strings.Join(strings.Fields(complete), " "))
				text := strings.TrimPrefix(complete, "schedule ")
				if !strings.HasSuffix(strings.TrimSpace(text), "message") {
					text += " message"
				}
				_, err := parseScheduleInput(text + " body")
				require.NoError(t, err)
			}
			require.Contains(t, normalized, tc.want)
		})
	}
}

// Mattermost 10.11.4 consumes the longest matching list item plus a space,
// then advances to the next argument. An optional list can also be skipped.
// Exercise that external contract against the registered tree and real items.
// See server/channels/app/command_autocomplete.go in the Mattermost repository.
func followAutocompleteArguments(a *Autocompleter, args []*model.AutocompleteArg, parsed, rest string) []string {
	if len(args) == 0 {
		return nil
	}
	arg := args[0]
	var result []string
	if !arg.Required {
		result = append(result, followAutocompleteArguments(a, args[1:], parsed, rest)...)
	}
	var items []model.AutocompleteListItem
	switch arg.Type {
	case model.AutocompleteArgTypeDynamicList:
		fetchURL := arg.Data.(*model.AutocompleteDynamicListArg).FetchURL
		kind := fetchURL[strings.LastIndex(fetchURL, "/")+1:]
		items = a.Items("user", kind, parsed+rest, parsed)
	case model.AutocompleteArgTypeStaticList:
		items = arg.Data.(*model.AutocompleteStaticListArg).PossibleArguments
	default:
		return result
	}
	longest := ""
	in := strings.TrimPrefix(rest, " ")
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(in), strings.ToLower(item.Item)+" ") && len(item.Item) > len(longest) {
			longest = item.Item
		}
	}
	if longest != "" {
		n := len(longest) + 1
		return append(result, followAutocompleteArguments(a, args[1:], parsed+in[:n], in[n:])...)
	}
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Item), strings.ToLower(in)) {
			result = append(result, parsed+item.Item)
		}
	}
	return result
}

package command

import (
	"regexp"
	"strings"
	"time"

	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/internal/ports"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/constants"
	"github.com/apartmentlines/mattermost-plugin-poor-mans-scheduled-messages/server/formatter"
	"github.com/mattermost/mattermost/server/public/model"
)

var autocompleteTime = regexp.MustCompile(`(?i)^([0-9]{1,2}(?::[0-9]{2})?(?:[ \t]*[ap]m)?)(?:[ \t]|$)`)
var autocompleteDate = regexp.MustCompile(`(?i)^on[ \t]+([^ \t\r\n]+)(?:[ \t]|$)`)

// Mattermost prefixes plugin-relative dynamic URLs during command registration.
const autocompleteURL = "/api/v1/autocomplete"

// Autocompleter supplies values for the existing schedule command grammar.
type Autocompleter struct {
	users       ports.UserService
	preferences ports.DisplayPreferences
	clock       ports.Clock
}

// NewAutocompleter constructs a recipient-aware command suggestion provider.
func NewAutocompleter(users ports.UserService, preferences ports.DisplayPreferences, clock ports.Clock) *Autocompleter {
	return &Autocompleter{users: users, preferences: preferences, clock: clock}
}

// Items returns list items in the format consumed by Mattermost's server-side
// autocomplete parser. It never echoes the message body into suggestions.
func (a *Autocompleter) Items(userID, kind, input, parsed string) []model.AutocompleteListItem {
	items := []model.AutocompleteListItem{}
	if kind != "time" && kind != "date" {
		return items
	}
	if !strings.HasPrefix(input, parsed) {
		return items
	}
	// Match the server's parseListItems: it strips exactly one leading space.
	// Retain any additional padding in items so literal prefix matching works.
	nativeInput := strings.TrimPrefix(strings.TrimPrefix(input, parsed), " ")
	remaining := strings.TrimLeft(nativeInput, " \t")
	padding := nativeInput[:len(nativeInput)-len(remaining)]
	// Mattermost replays earlier arguments on every keystroke. Completed values
	// only need to be recognized here, not resolved again through user APIs.
	if kind == "time" {
		if match := autocompleteTime.FindStringSubmatch(remaining); match != nil && len(remaining) > len(match[1]) {
			if _, err := parseTimeStr(normalizeTime(match[1]), time.UTC); err == nil {
				return []model.AutocompleteListItem{{Item: padding + completedAutocompleteValue(remaining, len(match[1]))}}
			}
		}
	} else {
		fields := strings.Fields(remaining)
		if len(fields) > 0 && !strings.HasPrefix("on", strings.ToLower(fields[0])) {
			return items
		}
	}
	user, err := a.users.Get(userID)
	if err != nil || user == nil {
		return items
	}
	loc, err := time.LoadLocation(userTimezone(user))
	if err != nil {
		loc = time.UTC
	}
	r := formatter.ForUser(a.preferences, userID)
	now := a.clock.Now().In(loc)
	if kind == "time" {
		// Suggestions are optional examples: accept any manually entered time
		// as a completed argument so the next grammar step remains reachable.
		if match := autocompleteTime.FindStringSubmatch(remaining); match != nil {
			if candidate, err := resolveScheduledTime(normalizeTime(match[1]), "", now, loc); err == nil {
				items = append(items, timeItem(match[1], candidate, r))
			}
		}
		seen := make(map[string]bool)
		for _, candidate := range []time.Time{
			now.Truncate(15 * time.Minute).Add(15 * time.Minute),
			now.Truncate(15 * time.Minute).Add(time.Hour),
			time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, loc),
			time.Date(now.Year(), now.Month(), now.Day(), 13, 30, 0, 0, loc),
			time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, loc),
		} {
			value := strings.ReplaceAll(r.ClockTime(candidate), " ", "")
			if seen[value] {
				continue
			}
			seen[value] = true
			resolved, err := resolveScheduledTime(normalizeTime(value), "", now, loc)
			if err == nil && (len(items) == 0 || !strings.EqualFold(items[0].Item, value)) {
				items = append(items, timeItem(value, resolved, r))
			}
		}
		return padAutocompleteItems(items, padding)
	}
	// The optional date is a single list argument containing both literal `on`
	// and its value. This lets Mattermost also offer the required `message`
	// keyword when the date is omitted.
	prefix := strings.TrimPrefix(strings.TrimSpace(parsed), "/")
	prefixFields := strings.Fields(prefix)
	if len(prefixFields) < 3 || !strings.EqualFold(prefixFields[0], constants.CommandTrigger) || !strings.EqualFold(prefixFields[1], "at") {
		return items
	}
	timeValue := normalizeTime(strings.Join(prefixFields[2:], ""))
	if _, err := parseTimeStr(timeValue, loc); err != nil {
		return items
	}
	datePrefix := "on "
	if len(remaining) > 2 && strings.EqualFold(remaining[:2], "on") {
		afterOn := strings.TrimLeft(remaining[2:], " \t")
		if len(afterOn) < len(remaining)-2 {
			datePrefix = remaining[:len(remaining)-len(afterOn)]
		}
	}
	if match := autocompleteDate.FindStringSubmatchIndex(remaining); match != nil && determineDateFormat(strings.ToLower(remaining[match[2]:match[3]])) != dateFormatInvalid {
		value := strings.ToLower(remaining[match[2]:match[3]])
		items = append(items, model.AutocompleteListItem{Item: completedAutocompleteValue(remaining, match[3]), Hint: "message <text>", HelpText: loc.String()})
		if candidate, err := resolveScheduledTime(timeValue, value, now, loc); err == nil {
			items[0].HelpText = r.DateTime(candidate) + " (" + loc.String() + ")"
		}
	}
	for offset := 0; offset < 8; offset++ {
		day := now.AddDate(0, 0, offset)
		value := datePrefix + day.Format(constants.DateParseLayoutYYYYMMDD)
		candidate, err := resolveScheduledTime(timeValue, day.Format(constants.DateParseLayoutYYYYMMDD), now, loc)
		if err != nil || (len(items) > 0 && strings.EqualFold(items[0].Item, value)) {
			continue
		}
		description := r.DateTime(candidate) + " (" + loc.String() + ")"
		if offset == 0 {
			description = r.Text("today") + " · " + description
		}
		if offset == 1 {
			description = r.Text("tomorrow") + " · " + description
		}
		items = append(items, model.AutocompleteListItem{Item: value, Hint: "message <text>", HelpText: description})
	}
	return padAutocompleteItems(items, padding)
}

// Include all completed separator spaces except the last: the server consumes
// one literal space after each item. Only the value and its separator are kept,
// never the following message text.
func completedAutocompleteValue(input string, end int) string {
	rest := input[end:]
	separator := rest[:len(rest)-len(strings.TrimLeft(rest, " \t"))]
	if strings.HasSuffix(separator, " ") {
		end += len(separator) - 1
	}
	return input[:end]
}

func padAutocompleteItems(items []model.AutocompleteListItem, padding string) []model.AutocompleteListItem {
	for i := range items {
		items[i].Item = padding + items[i].Item
	}
	return items
}

func timeItem(value string, candidate time.Time, r formatter.Renderer) model.AutocompleteListItem {
	return model.AutocompleteListItem{Item: value, Hint: "[on <date>] message <text>", HelpText: r.DateTime(candidate) + " (" + candidate.Location().String() + ")"}
}

package modules

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/config"
	"github.com/uasneppy/fuku_robot/fuku/db/timezone"
)

// The times the bot shows are in the zone of the group the message is about (Part 3 of the plan of
// 2026-11-18): chatClock gives a chat's *time.Location and formatChatTime is the one formatter every
// display site calls. Log lines, file names, Redis hour stamps and anything stored stay UTC.

// chatClockTimeout bounds the one cached read of a chat's zone.
const chatClockTimeout = 2 * time.Second

var (
	// chatTimezoneLookup reads a chat's stored zone ("" for none), a variable only so a test can fail it.
	chatTimezoneLookup = timezone.Get

	fallbackLocationOnce sync.Once
	fallbackLocation     *time.Location

	// chatClockWarnOnce makes a failed zone read log one warning per process, not one per message.
	chatClockWarnOnce sync.Once
)

// defaultChatLocation is the bot-wide zone: DEFAULT_TIMEZONE as config loaded it, or, before the
// config was validated (tests, CLI modes), the default zone name loaded once, or UTC.
func defaultChatLocation() *time.Location {
	if config.AppConfig != nil && config.AppConfig.DefaultLocation != nil {
		return config.AppConfig.DefaultLocation
	}
	fallbackLocationOnce.Do(func() {
		loc, err := config.LoadDefaultLocation(config.DefaultTimezoneName)
		if err != nil {
			loc = time.UTC
		}
		fallbackLocation = loc
	})
	return fallbackLocation
}

// chatTimezoneDefaultName is the name of the bot-wide zone.
func chatTimezoneDefaultName() string {
	return defaultChatLocation().String()
}

// parseStoredTimezone turns a stored zone back into a location: an IANA name, or a fixed offset
// written as UTC+hh:mm or UTC-hh:mm (the form applyTimezone stores).
func parseStoredTimezone(value string) (*time.Location, bool) {
	if value == "" {
		return nil, false
	}
	if offset, ok := parseUTCOffset(value); ok {
		return time.FixedZone(timezoneOffsetLabel(offset), offset), true
	}
	if value == "Local" {
		return nil, false
	}
	loc, err := time.LoadLocation(value)
	if err != nil {
		return nil, false
	}
	return loc, true
}

// parseUTCOffset reads an offset in seconds from "+3", "-5", "+5:30", "UTC+2", "utc-03:00" or
// "GMT+1": a sign, hours 0 to 14 and optional minutes 00, 15, 30 or 45. "UTC" alone is not an
// offset here (it is the IANA name UTC).
func parseUTCOffset(raw string) (int, bool) {
	s := strings.TrimSpace(raw)
	if upper := strings.ToUpper(s); strings.HasPrefix(upper, "UTC") || strings.HasPrefix(upper, "GMT") {
		s = s[3:]
	}
	if len(s) < 2 || (s[0] != '+' && s[0] != '-') {
		return 0, false
	}
	sign := 1
	if s[0] == '-' {
		sign = -1
	}
	hoursPart, minutesPart, hasMinutes := strings.Cut(s[1:], ":")
	hours, ok := parseOffsetHours(hoursPart)
	if !ok {
		return 0, false
	}
	minutes := 0
	if hasMinutes {
		if minutes, ok = parseOffsetMinutes(minutesPart); !ok {
			return 0, false
		}
	}
	offset := sign * (hours*3600 + minutes*60)
	if offset > 14*3600 || offset < -12*3600 {
		return 0, false
	}
	return offset, true
}

// parseOffsetHours reads the hours of an offset: one or two digits, 0 to 14.
func parseOffsetHours(raw string) (int, bool) {
	if len(raw) == 0 || len(raw) > 2 {
		return 0, false
	}
	hours, err := strconv.Atoi(raw)
	return hours, err == nil && hours >= 0 && hours <= 14
}

// parseOffsetMinutes reads the minutes of an offset: exactly two digits, 00, 15, 30 or 45.
func parseOffsetMinutes(raw string) (int, bool) {
	if len(raw) != 2 {
		return 0, false
	}
	minutes, err := strconv.Atoi(raw)
	return minutes, err == nil && (minutes == 0 || minutes == 15 || minutes == 30 || minutes == 45)
}

// timezoneOffsetLabel writes an offset in seconds as the short label the formatter appends:
// UTC, UTC+2, UTC-5, UTC+5:30.
func timezoneOffsetLabel(offset int) string {
	if offset == 0 {
		return "UTC"
	}
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	h, m := offset/3600, (offset%3600)/60
	if m == 0 {
		return fmt.Sprintf("UTC%s%d", sign, h)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, h, m)
}

// storedOffsetName writes an offset as the zone value applyTimezone stores: UTC+hh:mm.
func storedOffsetName(offset int) string {
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, (offset%3600)/60)
}

// chatClock is the zone a chat's times are shown in: its stored choice, else the bot's default. A read
// that fails falls back to the default and logs one warning per process.
func chatClock(ctx context.Context, chatID int64) *time.Location {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, chatClockTimeout)
	defer cancel()
	value, err := chatTimezoneLookup(ctx, chatID)
	if err != nil {
		chatClockWarnOnce.Do(func() {
			log.Warnf("[Timezone] the timezone of chat %d could not be read, showing the default: %v", chatID, err)
		})
		return defaultChatLocation()
	}
	if loc, ok := parseStoredTimezone(value); ok {
		return loc
	}
	return defaultChatLocation()
}

// chatClockOf is chatClock with a fresh context.
func chatClockOf(chatID int64) *time.Location {
	return chatClock(context.Background(), chatID)
}

// timezoneLabelAt is the short label of loc at t: UTC+2 in Kyiv's winter, UTC+3 in its summer.
func timezoneLabelAt(t time.Time, loc *time.Location) string {
	_, offset := t.In(loc).Zone()
	return timezoneOffsetLabel(offset)
}

// formatChatTime renders t in loc with layout and the zone's short label: "14:05 (UTC+2)". A nil loc
// is the bot's default.
func formatChatTime(t time.Time, loc *time.Location, layout string) string {
	if loc == nil {
		loc = defaultChatLocation()
	}
	return t.In(loc).Format(layout) + " (" + timezoneLabelAt(t, loc) + ")"
}

// formatChatDate renders the day of t in loc, with no zone label (a date alone needs none).
func formatChatDate(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = defaultChatLocation()
	}
	return t.In(loc).Format("2006-01-02")
}

package daemon

import (
	"strings"
	"testing"
	"time"
)

func TestNextCronTimeSupportsAliasesRangesNamesAndDayOrSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expression string
		timezone   string
		after      time.Time
		want       time.Time
	}{
		{
			name:       "alias is strictly after the supplied instant",
			expression: "@daily",
			timezone:   "UTC",
			after:      time.Date(2026, time.July, 12, 0, 0, 0, 0, time.UTC),
			want:       time.Date(2026, time.July, 13, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "steps ranges and names",
			expression: "*/15 9-10 * jan,jul mon-fri",
			timezone:   "UTC",
			after:      time.Date(2026, time.January, 5, 9, 0, 30, 0, time.UTC),
			want:       time.Date(2026, time.January, 5, 9, 15, 0, 0, time.UTC),
		},
		{
			name:       "seven is Sunday",
			expression: "0 8 * * 7",
			timezone:   "UTC",
			after:      time.Date(2026, time.July, 11, 8, 0, 0, 0, time.UTC),
			want:       time.Date(2026, time.July, 12, 8, 0, 0, 0, time.UTC),
		},
		{
			name:       "restricted month-day and weekday use crontab OR",
			expression: "0 0 13 * mon",
			timezone:   "UTC",
			after:      time.Date(2026, time.January, 6, 0, 0, 0, 0, time.UTC),
			want:       time.Date(2026, time.January, 12, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := nextCronTime(test.expression, test.timezone, test.after)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(test.want) {
				t.Fatalf("nextCronTime(%q) = %s, want %s", test.expression, got, test.want)
			}
		})
	}
}

func TestNextCronTimeSkipsNonexistentDSTWallTime(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}
	after := time.Date(2026, time.September, 26, 2, 30, 0, 0, location)
	want := time.Date(2026, time.September, 28, 2, 30, 0, 0, location).UTC()
	got, err := nextCronTime("30 2 * * *", "Pacific/Auckland", after)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Fatalf("next cron time across DST gap = %s (%s), want %s (%s)", got, got.In(location), want, want.In(location))
	}
}

func TestNextCronTimeRunsBothRepeatedDSTWallTimes(t *testing.T) {
	t.Parallel()

	if _, err := time.LoadLocation("Pacific/Auckland"); err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}
	first := time.Date(2026, time.April, 4, 13, 30, 0, 0, time.UTC)
	second := time.Date(2026, time.April, 4, 14, 30, 0, 0, time.UTC)
	got, err := nextCronTime("30 2 * * *", "Pacific/Auckland", first.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(first) {
		t.Fatalf("first repeated wall time = %s, want %s", got, first)
	}
	got, err = nextCronTime("30 2 * * *", "Pacific/Auckland", first)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(second) {
		t.Fatalf("second repeated wall time = %s, want %s", got, second)
	}
}

func TestCronValidationRejectsMalformedExpressionsAndTimezone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expression string
		timezone   string
		want       string
	}{
		{name: "field count", expression: "* * * *", timezone: "UTC", want: "five fields"},
		{name: "minute range", expression: "60 * * * *", timezone: "UTC", want: "between 0 and 59"},
		{name: "zero step", expression: "*/0 * * * *", timezone: "UTC", want: "positive integer"},
		{name: "step on scalar", expression: "5/2 * * * *", timezone: "UTC", want: "requires * or a range"},
		{name: "backwards range", expression: "20-10 * * * *", timezone: "UTC", want: "runs backwards"},
		{name: "unknown month", expression: "0 0 1 nope *", timezone: "UTC", want: "between 1 and 12"},
		{name: "impossible month day", expression: "0 0 31 2 *", timezone: "UTC", want: "can never occur"},
		{name: "unknown timezone", expression: "0 0 * * *", timezone: "Mars/Olympus_Mons", want: "invalid schedule timezone"},
	}

	after := time.Date(2026, time.July, 12, 0, 0, 0, 0, time.UTC)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := nextCronTime(test.expression, test.timezone, after)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("nextCronTime(%q, %q) error = %v, want substring %q", test.expression, test.timezone, err, test.want)
			}
		})
	}
}

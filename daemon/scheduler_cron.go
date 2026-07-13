package daemon

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var cronAliases = map[string]string{
	"@hourly":   "0 * * * *",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@weekly":   "0 0 * * 0",
	"@monthly":  "0 0 1 * *",
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
}

var cronMonths = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

var cronWeekdays = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

type cronExpression struct {
	minute, hour, dayOfMonth, month, dayOfWeek cronField
}

type cronField struct {
	allowed  []bool
	wildcard bool
}

func parseCronExpression(value string) (cronExpression, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if alias, ok := cronAliases[value]; ok {
		value = alias
	}
	parts := strings.Fields(value)
	if len(parts) != 5 {
		return cronExpression{}, errors.New("daemon: cron expression must contain five fields: minute hour day-of-month month day-of-week")
	}
	minute, err := parseCronField(parts[0], 0, 59, nil)
	if err != nil {
		return cronExpression{}, fmt.Errorf("daemon: invalid cron minute: %w", err)
	}
	hour, err := parseCronField(parts[1], 0, 23, nil)
	if err != nil {
		return cronExpression{}, fmt.Errorf("daemon: invalid cron hour: %w", err)
	}
	dayOfMonth, err := parseCronField(parts[2], 1, 31, nil)
	if err != nil {
		return cronExpression{}, fmt.Errorf("daemon: invalid cron day-of-month: %w", err)
	}
	month, err := parseCronField(parts[3], 1, 12, cronMonths)
	if err != nil {
		return cronExpression{}, fmt.Errorf("daemon: invalid cron month: %w", err)
	}
	// Seven is accepted as a second spelling of Sunday.
	dayOfWeek, err := parseCronField(parts[4], 0, 7, cronWeekdays)
	if err != nil {
		return cronExpression{}, fmt.Errorf("daemon: invalid cron day-of-week: %w", err)
	}
	expression := cronExpression{minute, hour, dayOfMonth, month, dayOfWeek}
	if !expression.hasPossibleDate() {
		return cronExpression{}, errors.New("daemon: cron day-of-month can never occur in the selected month")
	}
	return expression, nil
}

func (expression cronExpression) hasPossibleDate() bool {
	// A restricted weekday is ORed with day-of-month and therefore always
	// supplies a possible date. Only an exact wildcard weekday makes the
	// day-of-month/month combination decisive.
	if !expression.dayOfWeek.wildcard || expression.dayOfMonth.wildcard {
		return true
	}
	maximumDays := [...]int{0, 31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	for month := 1; month <= 12; month++ {
		if !expression.month.allowed[month] {
			continue
		}
		for day := 1; day <= maximumDays[month]; day++ {
			if expression.dayOfMonth.allowed[day] {
				return true
			}
		}
	}
	return false
}

func parseCronField(value string, minimum, maximum int, names map[string]int) (cronField, error) {
	field := cronField{allowed: make([]bool, maximum+1), wildcard: value == "*"}
	if strings.TrimSpace(value) == "" {
		return cronField{}, errors.New("field is empty")
	}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return cronField{}, errors.New("empty list item")
		}
		base, stepText, hasStep := strings.Cut(item, "/")
		if hasStep && strings.Contains(stepText, "/") {
			return cronField{}, fmt.Errorf("invalid step %q", item)
		}
		step := 1
		if hasStep {
			parsed, err := strconv.Atoi(stepText)
			if err != nil || parsed <= 0 {
				return cronField{}, fmt.Errorf("step must be a positive integer in %q", item)
			}
			step = parsed
		}
		start, end := minimum, maximum
		switch {
		case base == "*":
		case strings.Contains(base, "-"):
			left, right, ok := strings.Cut(base, "-")
			if !ok || strings.Contains(right, "-") {
				return cronField{}, fmt.Errorf("invalid range %q", base)
			}
			var err error
			start, err = parseCronValue(left, minimum, maximum, names)
			if err != nil {
				return cronField{}, err
			}
			end, err = parseCronValue(right, minimum, maximum, names)
			if err != nil {
				return cronField{}, err
			}
			if start > end {
				return cronField{}, fmt.Errorf("range %q runs backwards", base)
			}
		default:
			if hasStep {
				return cronField{}, fmt.Errorf("step in %q requires * or a range", item)
			}
			parsed, err := parseCronValue(base, minimum, maximum, names)
			if err != nil {
				return cronField{}, err
			}
			start, end = parsed, parsed
		}
		for candidate := start; candidate <= end; candidate += step {
			field.allowed[candidate] = true
		}
	}
	return field, nil
}

func parseCronValue(value string, minimum, maximum int, names map[string]int) (int, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if named, ok := names[value]; ok {
		return named, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("value %q must be between %d and %d", value, minimum, maximum)
	}
	return parsed, nil
}

func loadScheduleLocation(name string) (*time.Location, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "local") {
		label := time.Local.String()
		if label == "" {
			label = "Local"
		}
		return time.Local, label, nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, "", fmt.Errorf("daemon: invalid schedule timezone %q: %w", name, err)
	}
	return location, name, nil
}

func nextCronTime(expression, timezone string, after time.Time) (time.Time, error) {
	parsed, err := parseCronExpression(expression)
	if err != nil {
		return time.Time{}, err
	}
	location, _, err := loadScheduleLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	// Iterating absolute minutes handles timezone offsets and DST transitions
	// without constructing nonexistent wall-clock times. Eight years covers the
	// sparsest valid five-field schedules while bounding malformed searches.
	candidate := after.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := candidate.AddDate(8, 0, 0)
	for !candidate.After(limit) {
		local := candidate.In(location)
		if parsed.matches(local) {
			return candidate.UTC(), nil
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, errors.New("daemon: cron expression has no matching time within eight years")
}

func (expression cronExpression) matches(value time.Time) bool {
	if !expression.minute.allowed[value.Minute()] || !expression.hour.allowed[value.Hour()] || !expression.month.allowed[int(value.Month())] {
		return false
	}
	dayOfMonth := expression.dayOfMonth.allowed[value.Day()]
	weekday := int(value.Weekday())
	dayOfWeek := expression.dayOfWeek.allowed[weekday] || (weekday == 0 && len(expression.dayOfWeek.allowed) > 7 && expression.dayOfWeek.allowed[7])
	switch {
	case expression.dayOfMonth.wildcard && expression.dayOfWeek.wildcard:
		return true
	case expression.dayOfMonth.wildcard:
		return dayOfWeek
	case expression.dayOfWeek.wildcard:
		return dayOfMonth
	default:
		// Traditional crontab semantics use OR when both day fields are
		// restricted, unlike the AND used for every other field.
		return dayOfMonth || dayOfWeek
	}
}

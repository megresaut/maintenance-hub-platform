package recurring

import (
	"fmt"
	"strings"
	"time"
)

func CalculateNextRun(
	frequency RecurringFrequency,
	interval int,
	fromDate time.Time,
	byDay []string,
	dayOfMonth *int,
	weekOfMonth *int,
	weekdayOfMonth *string, // <-- updated to string
) (time.Time, error) {

	// enforce sane interval
	if interval <= 0 {
		interval = 1
	}

	switch frequency {

	///////////////////////////////////////////////////////////////////////////////
	// DAILY
	///////////////////////////////////////////////////////////////////////////////
	case FrequencyDaily:
		return fromDate.AddDate(0, 0, interval), nil

	///////////////////////////////////////////////////////////////////////////////
	// WEEKLY
	///////////////////////////////////////////////////////////////////////////////
	case FrequencyWeekly:
		if len(byDay) == 0 {
			return time.Time{}, fmt.Errorf("weekly recurrence requires byDay")
		}

		targetDays := map[int]bool{}
		for _, d := range byDay {
			idx := weekdayStringToIndex(d)
			if idx >= 0 {
				targetDays[idx] = true
			}
		}

		for i := 1; i <= interval*7; i++ {
			d := fromDate.AddDate(0, 0, i)
			if targetDays[int(d.Weekday())] {
				return d, nil
			}
		}

		return time.Time{}, fmt.Errorf("no valid weekly date found within lookahead window")

	///////////////////////////////////////////////////////////////////////////////
	// MONTHLY
	///////////////////////////////////////////////////////////////////////////////
	case FrequencyMonthly:

		// Move forward by N months
		base := fromDate.AddDate(0, interval, 0)
		year, month := base.Year(), base.Month()

		// ---- Option A: specific day of month
		if dayOfMonth != nil {
			day := clampDayInMonth(year, month, *dayOfMonth)

			return time.Date(
				year, month, day,
				fromDate.Hour(), fromDate.Minute(), 0, 0,
				fromDate.Location(),
			), nil
		}

		// ---- Option B: Nth weekday of month ("2nd Tuesday")
		if weekOfMonth != nil && weekdayOfMonth != nil {
			wk := *weekOfMonth // 1–5

			idx := weekdayStringToIndex(*weekdayOfMonth)
			if idx < 0 {
				return time.Time{}, fmt.Errorf("invalid weekdayOfMonth: %s", *weekdayOfMonth)
			}

			cur := time.Date(
				year, month, 1,
				fromDate.Hour(), fromDate.Minute(), 0, 0,
				fromDate.Location(),
			)

			found := 0
			for i := 0; i < 31; i++ {
				if cur.Month() != month {
					break
				}
				if int(cur.Weekday()) == idx {
					found++
					if found == wk {
						return cur, nil
					}
				}
				cur = cur.AddDate(0, 0, 1)
			}

			return time.Time{}, fmt.Errorf("no valid weekday occurrence found in month")
		}

		return time.Time{}, fmt.Errorf("monthly recurrence missing required fields")

	///////////////////////////////////////////////////////////////////////////////
	// YEARLY
	///////////////////////////////////////////////////////////////////////////////
	case FrequencyYearly:
		base := fromDate.AddDate(interval, 0, 0)

		if dayOfMonth != nil {
			day := clampDayInMonth(base.Year(), fromDate.Month(), *dayOfMonth)

			return time.Date(
				base.Year(), fromDate.Month(), day,
				fromDate.Hour(), fromDate.Minute(), 0, 0,
				fromDate.Location(),
			), nil
		}

		// otherwise just advance interval years
		return base, nil
	}

	return time.Time{}, fmt.Errorf("unknown recurring frequency: %s", frequency)
}

////////////////////////////////////////////////////////////////////////////////
// HELPERS
////////////////////////////////////////////////////////////////////////////////

func lastDayOfMonth(year int, month time.Month) int {
	t := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	return t.Day()
}

func clampDayInMonth(year int, month time.Month, day int) int {
	if day < 1 {
		return 1
	}
	if day > 28 {
		last := lastDayOfMonth(year, month)
		if day > last {
			return last
		}
	}
	return day
}

// Accepts: "mon", "monday", "Tue", "THURSDAY", etc.
func weekdayStringToIndex(s string) int {
	x := strings.ToLower(strings.TrimSpace(s))

	switch x {
	case "sun", "sunday":
		return 0
	case "mon", "monday":
		return 1
	case "tue", "tues", "tuesday":
		return 2
	case "wed", "weds", "wednesday":
		return 3
	case "thu", "thur", "thurs", "thursday":
		return 4
	case "fri", "friday":
		return 5
	case "sat", "saturday":
		return 6
	}
	return -1
}

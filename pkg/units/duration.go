// Package units provides helper function to parse and print size and time units
// in human-readable format.
package units

import (
	"fmt"
	"time"
)

// HumanDuration returns a human-readable approximation of a duration
// (eg. "About a minute", "4 hours ago", etc.).
func HumanDuration(d time.Duration) string {
	if seconds := int(d.Seconds()); seconds < 60 {
		return fmt.Sprintf("%d secs", seconds)
	} else if minutes := int(d.Minutes()); minutes == 1 {
		return "1 min"
	} else if minutes < 60 {
		return fmt.Sprintf("%d mins", minutes)
	} else if hours := int(d.Hours()); hours == 1 {
		return "1 hour"
	} else if hours < 48 {
		return fmt.Sprintf("%d hours", hours)
	} else if hours < 24*7*2 {
		return fmt.Sprintf("%d days", hours/24)
	} else if hours < 24*30*3 {
		return fmt.Sprintf("%d weeks", hours/24/7)
	} else if hours < 24*365*2 {
		return fmt.Sprintf("%d months", hours/24/30)
	}
	return fmt.Sprintf("%d years", int(d.Hours())/24/365)
}

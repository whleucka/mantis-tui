package service

import "regexp"

var durationRe = regexp.MustCompile(`^\d{1,3}:[0-5]\d$`)

// ValidDuration reports whether s is a Mantis time-tracking value (H:MM).
func ValidDuration(s string) bool { return durationRe.MatchString(s) }

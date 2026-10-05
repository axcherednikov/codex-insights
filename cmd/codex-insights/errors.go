package main

import "fmt"

type cliError string

func (err cliError) Error() string {
	return string(err)
}

const (
	errInvalidConcurrency       cliError = "invalid semantic concurrency"
	errDuplicateFollowup        cliError = "duplicate explicit follow-up"
	errMissingSemanticResult    cliError = "missing semantic result for explicit follow-up"
	errMissingSteeringResult    cliError = "missing steering result for explicit follow-up"
	errMissingOutputPath        cliError = "--output is required"
	errInvalidGoldenLimit       cliError = "invalid golden case limit"
	errMissingFixturePath       cliError = "--fixture is required"
	errGoldenExpectedMismatch   cliError = "golden regression: approved expectations were not met"
	errGoldenPreventionMismatch cliError = "golden regression: prevention expectations were not met"
	errUnsupportedBrowserOS     cliError = "unsupported operating system for browser auto-open"
	errInvalidBrowserURL        cliError = "browser report URL must be an allowed HTTP loopback URL"
	errMissingJudgeRunner       cliError = "semantic Judge runner is unavailable"
	errNilHTMLContext           cliError = "HTML report context is nil"
)

type cliValueRangeError struct {
	option   string
	value    int
	minimum  int
	maximum  int
	identity cliError
}

type followupRecordError struct {
	identity cliError
	turnID   string
}

func (err followupRecordError) Error() string {
	switch err.identity {
	case errDuplicateFollowup:
		return fmt.Sprintf("duplicate explicit follow-up for turn %q", err.turnID)
	case errMissingSemanticResult:
		return fmt.Sprintf("missing semantic result for explicit follow-up %q", err.turnID)
	default:
		return fmt.Sprintf("missing steering result for explicit follow-up %q", err.turnID)
	}
}

func (err followupRecordError) Unwrap() error {
	return err.identity
}

type localizedCLIError struct {
	message string
	cause   error
}

func (err localizedCLIError) Error() string { return err.message }

func (err localizedCLIError) Unwrap() error { return err.cause }

func (err cliValueRangeError) Error() string {
	return fmt.Sprintf("invalid %s %d; must be between %d and %d", err.option, err.value, err.minimum, err.maximum)
}

func (err cliValueRangeError) Unwrap() error {
	return err.identity
}

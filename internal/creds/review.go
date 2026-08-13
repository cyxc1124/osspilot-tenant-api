package creds

import "fmt"

type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

func reviewTarget(action, current string) (string, error) {
	switch action {
	case "approve":
		if current != "pending" {
			return "", &ConflictError{Msg: fmt.Sprintf("Cannot approve request in status %q", current)}
		}
		return "approved", nil
	case "reject":
		if current != "pending" {
			return "", &ConflictError{Msg: fmt.Sprintf("Cannot reject request in status %q", current)}
		}
		return "rejected", nil
	case "disable":
		if current != "approved" {
			return "", &ConflictError{Msg: fmt.Sprintf("Cannot disable access in status %q", current)}
		}
		return "disabled", nil
	default:
		return "", fmt.Errorf("unknown review action")
	}
}

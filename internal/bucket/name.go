package bucket

import (
	"fmt"
	"regexp"
	"strings"
)

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func validateName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("bucket_name must be 3-63 lowercase letters, numbers, dots, or hyphens")
	}
	if strings.Contains(name, "..") || strings.Contains(name, ".-") || strings.Contains(name, "-.") {
		return fmt.Errorf("bucket_name contains invalid character sequences")
	}
	return nil
}

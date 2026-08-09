package commands

import (
	"fmt"
	"strings"
)

func formatBool(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

func formatChannel(id string) string {
	if id == "" {
		return "-"
	}

	if strings.HasPrefix(id, "<#") {
		return id
	}

	return fmt.Sprintf("<#%v>", id)
}

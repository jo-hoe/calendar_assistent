package smtp

import (
	"encoding/json"
	"fmt"
	"os"
)

// SMTPCredentials holds authentication data loaded from the credentials file.
type SMTPCredentials struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Organizer string `json:"organizer"` // ORGANIZER in iCal (e.g. "mailto:me@example.com")
}

// loadCredentials reads and parses an SMTPCredentials JSON file from path.
// The organizer field must be non-empty.
func loadCredentials(path string) (*SMTPCredentials, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from trusted config
	if err != nil {
		return nil, fmt.Errorf("reading credentials file %q: %w", path, err)
	}

	var creds SMTPCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials file %q: %w", path, err)
	}

	return &creds, nil
}

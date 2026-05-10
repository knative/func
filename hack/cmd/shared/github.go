package shared

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// HTTPClient is used for all outbound HTTP requests; has a 30-second timeout.
var HTTPClient = &http.Client{Timeout: 30 * time.Second}

// RunCmd runs name with args, streaming stdout/stderr to the process output.
func RunCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", name+" "+strings.Join(args, " "), err)
	}
	return nil
}

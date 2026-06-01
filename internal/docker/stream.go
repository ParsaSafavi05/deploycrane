package docker

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ParsaSafavi05/deploycrane/internal/sse"
)

type BuildLogEntry struct {
	Stream   string `json:"stream"`
	Status   string `json:"status"`
	Progress string `json:"progress"`
	Error    string `json:"error"`
	Aux      struct {
		ID string `json:"ID"`
	} `json:"aux"`
}

func StreamBuildLogs(dst io.Writer, src io.Reader) error {
	decoder := json.NewDecoder(src)

	for {
		var entry BuildLogEntry

		err := decoder.Decode(&entry)

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("decode build stream: %w", err)
		}

		if entry.Error != "" {
			sse.WriteEvent(dst, "error", entry.Error)
			return fmt.Errorf("build error: %s", entry.Error)
		}

		if entry.Stream != "" {
			sse.WriteEvent(dst, "", entry.Stream)
		}

		if entry.Aux.ID != "" {
			sse.WriteEvent(dst, "", fmt.Sprintf("image built: %s", entry.Aux.ID))
		}

		if entry.Status != "" {
			line := entry.Status
			if entry.Progress != "" {
				line += " " + entry.Progress
			}
			sse.WriteEvent(dst, "", line)
		}
	}

	sse.WriteEvent(dst, "complete", "build finished")

	return nil
}

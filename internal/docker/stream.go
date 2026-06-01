package docker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	flusher, ok := dst.(http.Flusher)
	if !ok {
		return fmt.Errorf("destination does not support flushing")
	}

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
			fmt.Fprintf(dst, "event: error\ndata: %s\n\n", entry.Error)
			flusher.Flush()
			return fmt.Errorf("build error: %s", entry.Error)
		}

		if entry.Stream != "" {
			fmt.Fprintf(dst, "data: %s\n\n", entry.Stream)
			flusher.Flush()
		}

		if entry.Aux.ID != "" {
			fmt.Fprintf(dst, "data: image built: %s\n\n", entry.Aux.ID)
			flusher.Flush()
		}

		if entry.Status != "" {
			line := entry.Status
			if entry.Progress != "" {
				line += " " + entry.Progress
			}
			fmt.Fprintf(dst, "data: %s\n\n", line)
			flusher.Flush()
		}
	}

	fmt.Fprintf(dst, "event: complete\ndata: build finished\n\n")
	flusher.Flush()

	return nil
}

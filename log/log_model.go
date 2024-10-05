package log

type LogEntry struct {
	Term    int    `json:"term"`
	Command string `json:"command"`
}

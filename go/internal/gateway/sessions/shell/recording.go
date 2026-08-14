package shell

import (
	"log"
	"path/filepath"

	auditstore "github.com/ops-bastion/ops/go/internal/gateway/infra/audit"
)

// startRecording owns creation of the terminal cast while depending only on
// generic recording-directory capabilities supplied by the composition root.
func startRecording(d Deps, operationID, title string) (*auditstore.CastRecorder, string) {
	if d.RecordingDir == nil || d.RecordingRelPath == nil {
		return nil, ""
	}
	dir, err := d.RecordingDir(operationID)
	if err != nil || dir == "" {
		if err != nil {
			log.Printf("ops-audit recording dir op=%s: %v", operationID, err)
		}
		return nil, ""
	}
	path := filepath.Join(dir, "session.cast")
	rec, err := auditstore.NewCastRecorder(path, title)
	if err != nil {
		log.Printf("ops-audit cast %s: %v", path, err)
		return nil, ""
	}
	return rec, d.RecordingRelPath(path)
}

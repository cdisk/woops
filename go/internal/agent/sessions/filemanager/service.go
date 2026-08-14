package filemanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type req struct {
	ID     int            `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

type resp struct {
	ID     int    `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

type entry struct {
	Name  string `json:"name"`
	Path  string `json:"path,omitempty"` // absolute path when set (e.g. Windows drive roots)
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
}

// ServeJSONRPC handles filemanager RPCs (list/stat/mkdir/remove/rename) over one WebSocket.
// File content upload/download uses the separate filetransfer session, not this RPC.
func ServeJSONRPC(ws *websocket.Conn) {
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		var r req
		if err := json.Unmarshal(data, &r); err != nil {
			_ = ws.WriteJSON(resp{ID: 0, Error: "bad json"})
			continue
		}
		out := handle(r)
		_ = ws.WriteJSON(out)
	}
}

func handle(r req) resp {
	pathParam, _ := r.Params["path"].(string)
	switch r.Method {
	case "list":
		entries, err := listDir(pathParam)
		if err != nil {
			return resp{ID: r.ID, Error: err.Error()}
		}
		return resp{ID: r.ID, Result: map[string]any{
			"entries":  entries,
			"platform": runtime.GOOS,
			"path":     normalizeListPath(pathParam),
		}}
	case "roots":
		entries, err := listRoots()
		if err != nil {
			return resp{ID: r.ID, Error: err.Error()}
		}
		return resp{ID: r.ID, Result: map[string]any{
			"entries":  entries,
			"platform": runtime.GOOS,
		}}
	case "stat":
		st, err := os.Stat(cleanPath(pathParam))
		if err != nil {
			return resp{ID: r.ID, Error: err.Error()}
		}
		return resp{ID: r.ID, Result: map[string]any{
			"name": st.Name(), "isDir": st.IsDir(), "size": st.Size(), "mtime": st.ModTime().Unix(),
		}}
	case "mkdir":
		if err := os.MkdirAll(cleanPath(pathParam), 0o755); err != nil {
			return resp{ID: r.ID, Error: err.Error()}
		}
		return resp{ID: r.ID, Result: map[string]any{"ok": true}}
	case "remove":
		if err := os.RemoveAll(cleanPath(pathParam)); err != nil {
			return resp{ID: r.ID, Error: err.Error()}
		}
		return resp{ID: r.ID, Result: map[string]any{"ok": true}}
	case "rename":
		to, _ := r.Params["to"].(string)
		if to == "" {
			return resp{ID: r.ID, Error: "missing params.to"}
		}
		if err := os.Rename(cleanPath(pathParam), cleanPath(to)); err != nil {
			return resp{ID: r.ID, Error: err.Error()}
		}
		return resp{ID: r.ID, Result: map[string]any{"ok": true}}
	default:
		return resp{ID: r.ID, Error: fmt.Sprintf("unknown method: %s", r.Method)}
	}
}

func isRootsPath(p string) bool {
	p = strings.TrimSpace(p)
	return p == "" || p == "/" || p == "\\" || strings.EqualFold(p, "roots")
}

func normalizeListPath(p string) string {
	if isRootsPath(p) {
		if runtime.GOOS == "windows" {
			return "roots"
		}
		return "/"
	}
	return cleanPath(p)
}

func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	if isRootsPath(p) {
		if runtime.GOOS == "windows" {
			if wd, err := os.Getwd(); err == nil {
				vol := filepath.VolumeName(wd)
				if vol != "" {
					return vol + `\`
				}
			}
		}
		return string(os.PathSeparator)
	}
	if runtime.GOOS == "windows" && len(p) >= 2 && p[1] == ':' {
		rest := ""
		if len(p) > 2 {
			rest = p[2:]
		}
		rest = strings.ReplaceAll(rest, "/", `\`)
		if rest == "" || rest == `\` {
			return strings.ToUpper(p[:1]) + `:\`
		}
		return filepath.Clean(strings.ToUpper(p[:1]) + `:` + rest)
	}
	return filepath.Clean(p)
}

func listDir(p string) ([]entry, error) {
	if isRootsPath(p) {
		return listRoots()
	}
	dir := cleanPath(p)
	infos, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]entry, 0, len(infos))
	for _, info := range infos {
		full := filepath.Join(dir, info.Name())
		e := entry{Name: info.Name(), Path: full, IsDir: info.IsDir()}
		if fi, err := info.Info(); err == nil {
			e.Size = fi.Size()
			e.Mtime = fi.ModTime().Unix()
		} else {
			e.Mtime = time.Now().Unix()
		}
		out = append(out, e)
	}
	sortEntries(out)
	return out, nil
}

// sortEntries: directories first, then files; case-insensitive name within each group.
func sortEntries(entries []entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}

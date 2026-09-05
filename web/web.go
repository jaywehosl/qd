package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}

func Handler(basePath string) (http.Handler, error) {
	root, err := FS()
	if err != nil {
		return nil, err
	}
	files := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, basePath)
		name = strings.TrimPrefix(name, "/")
		if name == "" {
			name = "index.html"
		}

		if name == "index.html" {
			serveIndex(w, r, root)
			return
		}

		if _, err := fs.Stat(root, name); err != nil {
			if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			serveIndex(w, r, root)
			return
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + name
		files.ServeHTTP(w, r2)
	}), nil
}

func serveIndex(w http.ResponseWriter, r *http.Request, root fs.FS) {
	body, err := fs.ReadFile(root, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(body)
}

func IndexWith(vars map[string]string) ([]byte, error) {
	root, err := FS()
	if err != nil {
		return nil, err
	}
	body, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, err
	}

	var script strings.Builder
	script.WriteString("<script>")
	for k, v := range vars {
		script.WriteString("window.")
		script.WriteString(k)
		script.WriteString("=")
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		script.Write(encoded)
		script.WriteString(";")
	}
	script.WriteString("</script>")

	return []byte(strings.Replace(string(body), "<head>", "<head>"+script.String(), 1)), nil
}

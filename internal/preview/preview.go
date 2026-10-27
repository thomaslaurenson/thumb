// Package preview serves a rendered site over HTTP for its author to read
// before publishing, and tells open pages to reload when the site changes.
package preview

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// ReloadPath is where pages listen for a newer version of the site. The site
// has no path starting with an underscore, so it cannot clash with a page.
const ReloadPath = "/_thumb/reload"

// reloadScript is added to every HTML page served. It keeps the first version
// the server reports and reloads once a different one arrives, which also
// covers the server restarting, since EventSource reconnects by itself.
const reloadScript = `<script>
(() => {
  let seen;
  new EventSource("` + ReloadPath + `").onmessage = (e) => {
    if (seen === undefined) seen = e.data;
    else if (e.data !== seen) location.reload();
  };
})();
</script>
`

// Server serves the files of one site and swaps in newer versions as they are
// rendered. Build one with New.
type Server struct {
	// id tells this process's versions apart from a previous run's
	id string

	mu    sync.Mutex
	files map[string][]byte
	gen   int
	// changed is closed when Set replaces the site, then replaced
	changed chan struct{}
}

var _ http.Handler = (*Server)(nil)

// New returns a server for files, keyed by their path in the site as
// site.Render returns them.
func New(files map[string][]byte) *Server {
	return &Server{id: rand.Text(), files: files, changed: make(chan struct{})}
}

// Set replaces the site with files and tells listening pages to reload. It
// reports false, and changes nothing, when files match the site already
// served, so a save that changes nothing visible leaves the browser alone.
func (s *Server) Set(files map[string][]byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maps.EqualFunc(s.files, files, bytes.Equal) {
		return false
	}
	s.files = files
	s.gen++
	close(s.changed)
	s.changed = make(chan struct{})
	return true
}

// version returns the site's current version and a channel closed once it
// changes.
func (s *Server) version() (string, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id + "-" + strconv.Itoa(s.gen), s.changed
}

func (s *Server) file(name string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.files[name]
	return b, ok
}

// ServeHTTP answers as GitHub Pages does for the same files: a folder serves
// its index.html, a folder named without its trailing slash redirects to it, a
// missing path gets 404.html with a 404 status, and a file type it does not
// know, such as .plan, is sent as bytes.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == ReloadPath {
		s.serveReload(w, r)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if strings.HasSuffix(r.URL.Path, "/") {
		name = path.Join(name, "index.html")
	}
	status := http.StatusOK
	body, ok := s.file(name)
	if !ok {
		// Redirect to the cleaned name, never the raw path: "//host" would
		// otherwise send the browser off the site
		if _, dir := s.file(path.Join(name, "index.html")); dir && name != "" {
			http.Redirect(w, r, "/"+name+"/", http.StatusMovedPermanently)
			return
		}
		name, status = "404.html", http.StatusNotFound
		if body, ok = s.file(name); !ok {
			http.NotFound(w, r)
			return
		}
	}
	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if strings.HasPrefix(ctype, "text/html") {
		body = withReload(body)
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

// serveReload streams the site's version as server-sent events: once on
// connecting, then again each time Set changes the site.
func (s *Server) serveReload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	rc := http.NewResponseController(w)
	for {
		v, changed := s.version()
		if _, err := fmt.Fprintf(w, "data: %s\n\n", v); err != nil {
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}
		select {
		case <-changed:
		case <-r.Context().Done():
			return
		}
	}
}

// withReload returns a copy of page with the reload script before its closing
// body tag, leaving the stored page as it was rendered.
func withReload(page []byte) []byte {
	i := bytes.LastIndex(page, []byte("</body>"))
	if i < 0 {
		i = len(page)
	}
	out := make([]byte, 0, len(page)+len(reloadScript))
	out = append(out, page[:i]...)
	out = append(out, reloadScript...)
	return append(out, page[i:]...)
}

// Stamp summarises every file under paths by name, size and modification
// time, so it changes when a file under them is added, removed or saved. A
// path that does not exist is part of the summary rather than an error.
func Stamp(paths ...string) (string, error) {
	var b strings.Builder
	for _, root := range paths {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			// A file removed mid-walk is caught by the next stamp
			if errors.Is(err, fs.ErrNotExist) {
				if p == root {
					fmt.Fprintf(&b, "%s missing\n", p)
				}
				return nil
			}
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, "%s %d %d\n", p, info.Size(), info.ModTime().UnixNano())
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

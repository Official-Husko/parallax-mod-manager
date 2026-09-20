package backgrounds

import (
	"net/http"
	"os"
	"strings"
)

// URLPrefix is where the offline images are served, from inside the app: the
// webview cannot open files in the config folder itself, so the app answers these
// requests on its own asset route.
const URLPrefix = "/backgrounds/"

// LocalURL is the address the UI uses for one offline image.
func LocalURL(gameID, name string) string {
	return URLPrefix + gameID + "/" + escapeName(name)
}

// Middleware serves URLPrefix/<gameID>/<file> from the store and hands every other
// request to next. store is looked up per request because the config folder is
// only known once the app has started.
func Middleware(store func() Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, URLPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			serve(w, r, store())
		})
	}
}

func serve(w http.ResponseWriter, r *http.Request, s Store) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	gameID, name, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, URLPrefix), "/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	path, ok := s.Path(gameID, name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", MimeType(name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

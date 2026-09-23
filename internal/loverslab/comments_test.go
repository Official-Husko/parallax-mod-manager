package loverslab

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostCommentSubmitsTheExpectedFormFields(t *testing.T) {
	var gotMethod string
	var gotFields map[string]string

	mux := http.NewServeMux()
	mux.HandleFunc("/topic/269604-tether/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte(`<input type="hidden" name="csrfKey" value="abc123def456">`))
			return
		}
		gotMethod = r.Method
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
			t.Fatalf("expected a multipart request, got Content-Type %q (%v)", r.Header.Get("Content-Type"), err)
		}
		gotFields = map[string]string{}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("reading form part: %v", err)
			}
			data, _ := io.ReadAll(part)
			gotFields[part.FormName()] = string(data)
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	topicURL := srv.URL + "/topic/269604-tether/"
	if err := c.PostComment(context.Background(), topicURL, "<p>hello</p>"); err != nil {
		t.Fatalf("PostComment: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("expected a POST, got %q", gotMethod)
	}
	want := map[string]string{
		"commentform_269604_submitted": "1",
		"csrfKey":                      "abc123def456",
		"_contentReply":                "1",
		"topic_comment_269604":         "<p>hello</p>",
		"topic_auto_follow":            "0",
	}
	for k, v := range want {
		if gotFields[k] != v {
			t.Errorf("field %q = %q, want %q", k, gotFields[k], v)
		}
	}
}

func TestPostCommentRejectsANonTopicURL(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = c.PostComment(context.Background(), "https://www.loverslab.com/files/file/12345-something/", "hi")
	if err == nil {
		t.Fatal("expected an error for a URL that isn't a topic")
	}
}

func TestPostCommentSurfacesAnHTTPErrorStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/topic/1-x/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte(`<input type="hidden" name="csrfKey" value="key">`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = c.PostComment(context.Background(), srv.URL+"/topic/1-x/", "hi")
	if err == nil {
		t.Fatal("expected an error for a rejected post")
	}
}

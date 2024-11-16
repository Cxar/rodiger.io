package server

import (
	"context"
	"cxar/rodiger.io/internal/config"
	"cxar/rodiger.io/internal/docs"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/gomarkdown/markdown"
)

type Server struct {
	cfg  *config.Config
	docs *docs.Client
	srv  *http.Server
	tmpl *template.Template

	content struct {
		sync.RWMutex
		html    template.HTML
		updated time.Time
	}

	clients struct {
		sync.RWMutex
		list map[chan string]bool
	}
}

func New(cfg *config.Config) (*Server, error) {
	docsClient, err := docs.NewClient(cfg.GoogleCredPath)
	if err != nil {
		return nil, fmt.Errorf("creating docs client: %w", err)
	}

	tmpl, err := template.ParseGlob("web/templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}

	s := &Server{
		cfg:  cfg,
		docs: docsClient,
		tmpl: tmpl,
	}

	s.clients.list = make(map[chan string]bool)

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/content", s.handleContent)
	mux.HandleFunc("/last-update", s.handleLastUpdate)
	mux.HandleFunc("/updates", s.handleSSE)
	mux.HandleFunc("/static/", s.handleStatic)

	s.srv = &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: s.htmxMiddleware(mux),
	}

	return s, nil
}

func (s *Server) Start() error {
	go s.contentUpdater()
	return s.srv.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/static/"):]
	filePath := filepath.Join("static", path)
	ext := filepath.Ext(filePath)

	switch ext {
	case ".css":
		w.Header().Set("Content-Type", "text/css")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	}

	http.ServeFile(w, r, filePath)
}

func (s *Server) htmxMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "HX-Request")
		if r.Header.Get("HX-Trigger") != "" {
			w.Header().Set("HX-Trigger", r.Header.Get("HX-Trigger"))
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	s.content.RLock()
	data := struct {
		Content    template.HTML
		LastUpdate time.Time
	}{
		Content:    s.content.html,
		LastUpdate: s.content.updated,
	}
	s.content.RUnlock()

	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleContent(w http.ResponseWriter, r *http.Request) {
	s.content.RLock()
	defer s.content.RUnlock()
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(s.content.html))
}

func (s *Server) handleLastUpdate(w http.ResponseWriter, r *http.Request) {
	s.content.RLock()
	lastUpdate := s.content.updated
	s.content.RUnlock()
	fmt.Fprintf(w, "Last updated: %s", lastUpdate.Format("January 2, 2006"))
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChan := make(chan string)

	s.clients.Lock()
	s.clients.list[clientChan] = true
	s.clients.Unlock()

	defer func() {
		s.clients.Lock()
		delete(s.clients.list, clientChan)
		s.clients.Unlock()
		close(clientChan)
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-clientChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			w.(http.Flusher).Flush()
		case <-time.After(30 * time.Second):
			fmt.Fprintf(w, ":\n\n")
			w.(http.Flusher).Flush()
		}
	}
}

func (s *Server) contentUpdater() {
	ticker := time.NewTicker(time.Duration(s.cfg.UpdateInterval))
	defer ticker.Stop()

	s.updateContent()

	for range ticker.C {
		s.updateContent()
	}
}

func (s *Server) updateContent() {
	content, err := s.docs.GetDocument(s.cfg.DocID)
	if err != nil {
		s.broadcastToClients(fmt.Sprintf("error:%s", err.Error()))
		return
	}

	html := markdown.ToHTML([]byte(content), nil, nil)

	s.content.Lock()
	s.content.html = template.HTML(html)
	s.content.updated = time.Now()
	s.content.Unlock()

	s.broadcastToClients("contentUpdated")
}

func (s *Server) broadcastToClients(message string) {
	s.clients.RLock()
	defer s.clients.RUnlock()

	for clientChan := range s.clients.list {
		select {
		case clientChan <- message:
		default:
		}
	}
}

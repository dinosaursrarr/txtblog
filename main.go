package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Post struct {
	Filename string
	Content  string
	Preview  string
}

type Store struct {
	sync.RWMutex
	posts map[string]Post
}

type PageData struct {
	Posts       []Post
	Query       string
	CurrentPage int
	TotalPages  int
	PrevPage    int
	NextPage    int
	HasPrev     bool
	HasNext     bool
	TotalCount  int
}

const tpl = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Text Feed</title>
    <style>
        body { font-family: monospace; max-width: 750px; margin: 20px auto; padding: 0 10px; background: #1a1a1a; color: #d0d0d0; }
        input { width: 100%; padding: 8px; background: #2a2a2a; border: 1px solid #444; color: #fff; box-sizing: border-box; }
        .post { border-bottom: 1px solid #333; margin-top: 20px; padding-bottom: 15px; }
        a { color: #6db6ff; text-decoration: none; }
        pre { white-space: pre-wrap; word-wrap: break-word; color: #aaa; margin-top: 5px; }
        .meta { color: #888; font-size: 0.9em; margin: 10px 0; }
        .pagination { margin: 25px 0; display: flex; justify-content: space-between; align-items: center; }
        .pagination a { padding: 6px 12px; background: #2a2a2a; border: 1px solid #444; border-radius: 4px; }
    </style>
</head>
<body>
    <form method="GET">
        <input type="text" name="q" placeholder="Search filenames or full text..." value="{{.Query}}">
    </form>

    <div class="meta">Found {{.TotalCount}} file(s) — Page {{.CurrentPage}} of {{.TotalPages}}</div>

    {{range .Posts}}
    <div class="post">
        <strong><a href="/raw?name={{.Filename}}">{{.Filename}}</a></strong>
        <pre>{{.Preview}}</pre>
    </div>
    {{else}}
    <p>No matching text files found.</p>
    {{end}}

    <div class="pagination">
        {{if .HasPrev}}
            <a href="/?q={{.Query}}&page={{.PrevPage}}">&laquo; Previous</a>
        {{else}}<span></span>{{end}}

        {{if .HasNext}}
            <a href="/?q={{.Query}}&page={{.NextPage}}">Next &raquo;</a>
        {{end}}
    </div>
</body>
</html>`

func NewStore() *Store {
	return &Store{posts: make(map[string]Post)}
}

func (s *Store) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	newPosts := make(map[string]Post)
	for _, entry := range entries {
		filename := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(filename, ".txt") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			continue
		}
		content := string(data)
		preview := content
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		newPosts[filename] = Post{
			Filename: filename,
			Content:  content,
			Preview:  preview,
		}
	}

	s.Lock()
	s.posts = newPosts
	s.Unlock()
	return nil
}

func (s *Store) Search(query string) []Post {
	s.RLock()
	defer s.RUnlock()

	q := strings.ToLower(query)
	var filtered []Post
	for _, post := range s.posts {
		if q == "" || strings.Contains(strings.ToLower(post.Filename), q) || strings.Contains(strings.ToLower(post.Content), q) {
			filtered = append(filtered, post)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Filename > filtered[j].Filename
	})

	return filtered
}

func main() {
	port := flag.String("port", "8080", "Port to listen on")
	dir := flag.String("dir", ".", "Path to folder with .txt files")
	pageSize := flag.Int("size", 20, "Number of posts per page")
	pollInterval := flag.Duration("poll", 2*time.Minute, "Directory refresh interval (e.g. 30s, 2m)")
	flag.Parse()

	if *pollInterval <= 0 {
		log.Fatalf("Poll interval not set to a positive number")
	}

	store := NewStore()
	if err := store.LoadDir(*dir); err != nil {
		log.Fatalf("Failed to initial scan directory: %v", err)
	}

	go func() {
		ticker := time.NewTicker(*pollInterval)
		defer ticker.Stop()
		for range ticker.C {
			if err := store.LoadDir(*dir); err != nil {
				log.Printf("Error reloading directory %s: %v", *dir, err)
			}
		}
	}()

	t := template.Must(template.New("web").Parse(tpl))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rawQuery := r.URL.Query().Get("q")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}

		filtered := store.Search(rawQuery)

		totalCount := len(filtered)
		totalPages := (totalCount + *pageSize - 1) / *pageSize
		if totalPages == 0 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}

		start := (page - 1) * *pageSize
		end := start + *pageSize
		if start > totalCount {
			start = totalCount
		}
		if end > totalCount {
			end = totalCount
		}

		pagePosts := filtered[start:end]

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		t.Execute(w, PageData{
			Posts:       pagePosts,
			Query:       rawQuery,
			CurrentPage: page,
			TotalPages:  totalPages,
			PrevPage:    page - 1,
			NextPage:    page + 1,
			HasPrev:     page > 1,
			HasNext:     page < totalPages,
			TotalCount:  totalCount,
		})
	})

	http.HandleFunc("/raw", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Query().Get("name"))
		store.RLock()
		post, ok := store.posts[name]
		store.RUnlock()

		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(post.Content))
	})

	fmt.Printf("Serving in-memory text blog from %s on http://localhost:%s\n", *dir, *port)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}

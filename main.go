package main

import (
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Post struct {
	Filename string
	Preview  string
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

func main() {
	port := flag.String("port", "8080", "Port to listen on")
	dir := flag.String("dir", ".", "Path to folder with .txt files")
	pageSize := flag.Int("size", 20, "Number of posts per page")
	flag.Parse()

	t := template.Must(template.New("web").Parse(tpl))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rawQuery := r.URL.Query().Get("q")
		query := strings.ToLower(rawQuery)
		
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}

		entries, err := os.ReadDir(*dir)
		if err != nil {
			http.Error(w, "Unable to read directory", 500)
			return
		}

		// Sort newest-first by filename descending
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name() > entries[j].Name()
		})

		var filtered []Post
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
				continue
			}

			data, err := os.ReadFile(filepath.Join(*dir, entry.Name()))
			if err != nil {
				continue
			}
			content := string(data)

			nameMatch := strings.Contains(strings.ToLower(entry.Name()), query)
			contentMatch := strings.Contains(strings.ToLower(content), query)

			if query == "" || nameMatch || contentMatch {
				preview := content
				if len(preview) > 300 {
					preview = preview[:300] + "..."
				}
				filtered = append(filtered, Post{Filename: entry.Name(), Preview: preview})
			}
		}

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
		http.ServeFile(w, r, filepath.Join(*dir, name))
	})

	fmt.Printf("Serving text blog on http://localhost:%s\n", *port)
	http.ListenAndServe(":"+*port, nil)
}


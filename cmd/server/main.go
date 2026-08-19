package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"asmr-downloader/internal/asmr"
	"asmr-downloader/internal/downloader"
	"asmr-downloader/internal/selector"
)

var indexTemplate = template.Must(
	template.ParseFiles(
		"web/index.html",
	),
)

type Server struct {
	ASMR       *asmr.Client
	Downloader *downloader.Downloader

	Tasks map[string]*downloader.Task
	Mu    sync.RWMutex
}

var taskIDRegex = regexp.MustCompile(
	`^[a-zA-Z0-9_-]+$`,
)

func main() {

	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	downloadRoot := os.Getenv(
		"DOWNLOAD_DIR",
	)

	if downloadRoot == "" {
		downloadRoot = "./downloads"
	}

	if err := os.MkdirAll(
		downloadRoot,
		0755,
	); err != nil {
		log.Fatal(err)
	}

	server := &Server{
		ASMR:       asmr.NewClient(),
		Downloader: downloader.New(
			downloadRoot,
		),
		Tasks: make(
			map[string]*downloader.Task,
		),
	}

	mux := http.NewServeMux()

	mux.HandleFunc(
		"/",
		server.handleIndex,
	)

	mux.HandleFunc(
		"/api/work",
		server.handleWork,
	)

	mux.HandleFunc(
		"/api/download",
		server.handleDownload,
	)

	mux.HandleFunc(
		"/api/task/",
		server.handleTask,
	)

	mux.Handle(
		"/static/",
		http.StripPrefix(
			"/static/",
			http.FileServer(
				http.Dir("web"),
			),
		),
	)

	addr := ":" + port

	log.Printf(
		"ASMR Downloader listening on %s",
		addr,
	)

	log.Printf(
		"Download directory: %s",
		downloadRoot,
	)

	if err := http.ListenAndServe(
		addr,
		logMiddleware(mux),
	); err != nil {
		log.Fatal(err)
	}
}

func logMiddleware(
	next http.Handler,
) http.Handler {

	return http.HandlerFunc(
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {

			start := time.Now()

			next.ServeHTTP(w, r)

			log.Printf(
				"%s %s %s",
				r.Method,
				r.URL.Path,
				time.Since(start),
			)
		},
	)
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	data any,
) {

	w.Header().Set(
		"Content-Type",
		"application/json; charset=utf-8",
	)

	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) handleIndex(
	w http.ResponseWriter,
	r *http.Request,
) {

	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if err := indexTemplate.Execute(
		w,
		nil,
	); err != nil {

		http.Error(
			w,
			err.Error(),
			http.StatusInternalServerError,
		)
	}
}

type WorkRequest struct {
	RJ string `json:"rj"`
}

func (s *Server) handleWork(
	w http.ResponseWriter,
	r *http.Request,
) {

	if r.Method != http.MethodPost {
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	var request WorkRequest

	if err := json.NewDecoder(
		r.Body,
	).Decode(&request); err != nil {

		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]any{
				"error": "请求格式错误",
			},
		)

		return
	}

	id, err := asmr.NormalizeRJ(
		request.RJ,
	)

	if err != nil {

		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]any{
				"error": err.Error(),
			},
		)

		return
	}

	work, err := s.ASMR.GetWork(id)

	if err != nil {

		writeJSON(
			w,
			http.StatusBadGateway,
			map[string]any{
				"error": err.Error(),
			},
		)

		return
	}

	work.Files =
		selector.Select(work.Files)

	writeJSON(
		w,
		http.StatusOK,
		work,
	)
}

type DownloadRequest struct {
	RJ    string      `json:"rj"`
	Files []asmr.File `json:"files"`
}

func (s *Server) handleDownload(
	w http.ResponseWriter,
	r *http.Request,
) {

	if r.Method != http.MethodPost {
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	var request DownloadRequest

	if err := json.NewDecoder(
		r.Body,
	).Decode(&request); err != nil {

		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]any{
				"error": "请求格式错误",
			},
		)

		return
	}

	id, err := asmr.NormalizeRJ(
		request.RJ,
	)

	if err != nil {

		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]any{
				"error": err.Error(),
			},
		)

		return
	}

	if len(request.Files) == 0 {

		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]any{
				"error": "没有可下载文件",
			},
		)

		return
	}

	// 服务端再次筛选，避免客户端篡改
	request.Files =
		selector.Select(
			request.Files,
		)

	taskID := fmt.Sprintf(
		"%d",
		time.Now().UnixNano(),
	)

	files := make(
		[]*downloader.FileProgress,
		len(request.Files),
	)

	var totalBytes int64

	for i, file := range request.Files {

		files[i] = &downloader.FileProgress{
			Path:   file.Path,
			Size:   file.Size,
			Status: "waiting",
		}

		totalBytes += file.Size
	}

	task := &downloader.Task{
		ID:         taskID,
		RJ:         "RJ" + id,
		Status:     "waiting",
		Total:      len(files),
		Files:      files,
		TotalBytes: totalBytes,
		CreatedAt:  time.Now(),
	}

	s.Mu.Lock()
	s.Tasks[taskID] = task
	s.Mu.Unlock()

	go func() {

		ctx := context.Background()

		s.Downloader.Download(
			ctx,
			task,
			id,
			request.Files,
		)

	}()

	writeJSON(
		w,
		http.StatusOK,
		map[string]string{
			"taskId": taskID,
		},
	)
}

func (s *Server) handleTask(
	w http.ResponseWriter,
	r *http.Request,
) {

	id := strings.TrimPrefix(
		r.URL.Path,
		"/api/task/",
	)

	if !taskIDRegex.MatchString(id) {

		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": "invalid task id",
			},
		)

		return
	}

	s.Mu.RLock()

	task, exists := s.Tasks[id]

	s.Mu.RUnlock()

	if !exists {

		writeJSON(
			w,
			http.StatusNotFound,
			map[string]string{
				"error": "task not found",
			},
		)

		return
	}

	writeJSON(
		w,
		http.StatusOK,
		task.Snapshot(),
	)
}

// 防止路径误用 path 包导致问题，保留此辅助函数
func cleanURLPath(value string) string {
	return path.Clean(
		strings.ReplaceAll(
			value,
			"\\",
			"/",
		),
	)
}
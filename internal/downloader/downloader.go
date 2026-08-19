package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"asmr-downloader/internal/asmr"
)

type FileProgress struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Downloaded int64 `json:"downloaded"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type Task struct {
	ID      string `json:"id"`
	RJ      string `json:"rj"`
	Status  string `json:"status"`
	Total   int     `json:"total"`
	Success int     `json:"success"`
	Failed  int     `json:"failed"`
	Files   []*FileProgress `json:"files"`

	BytesDownloaded int64 `json:"bytesDownloaded"`
	TotalBytes      int64 `json:"totalBytes"`

	CreatedAt time.Time `json:"createdAt"`

	mu sync.RWMutex
}

func (t *Task) Snapshot() *Task {
	t.mu.RLock()
	defer t.mu.RUnlock()

	copyTask := *t

	copyTask.Files = make(
		[]*FileProgress,
		len(t.Files),
	)

	for i, file := range t.Files {

		copyFile := *file

		copyTask.Files[i] = &copyFile
	}

	return &copyTask
}

type Downloader struct {
	Client       *http.Client
	RootDir      string
	Concurrency  int
	RetryCount   int
}

func New(root string) *Downloader {
	return &Downloader{
		Client: &http.Client{
			Timeout: 0,
		},
		RootDir:     root,
		Concurrency: 4,
		RetryCount:  3,
	}
}

func safePath(root, relative string) (string, error) {

	clean := filepath.Clean(relative)

	if clean == "." ||
		clean == ".." ||
		len(clean) == 0 {

		return "", fmt.Errorf(
			"非法文件路径: %s",
			relative,
		)
	}

	full := filepath.Join(
		root,
		clean,
	)

	absRoot, err := filepath.Abs(root)

	if err != nil {
		return "", err
	}

	absFull, err := filepath.Abs(full)

	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(
		absRoot,
		absFull,
	)

	if err != nil {
		return "", err
	}

	if rel == ".." ||
		len(rel) >= 3 &&
			rel[:3] == ".."+string(os.PathSeparator) {

		return "", fmt.Errorf(
			"文件路径越界: %s",
			relative,
		)
	}

	return absFull, nil
}

func (d *Downloader) Download(
	ctx context.Context,
	task *Task,
	workID string,
	files []asmr.File,
) {

	workDir := filepath.Join(
		d.RootDir,
		"RJ"+workID,
	)

	if err := os.MkdirAll(
		workDir,
		0755,
	); err != nil {

		task.mu.Lock()
		task.Status = "failed"
		task.mu.Unlock()

		return
	}

	sem := make(
		chan struct{},
		d.Concurrency,
	)

	var wg sync.WaitGroup

	for i, file := range files {

		wg.Add(1)

		go func(index int, file asmr.File) {

			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}

			defer func() {
				<-sem
			}()

			d.downloadFile(
				ctx,
				task,
				workDir,
				index,
				file,
			)

		}(i, file)
	}

	wg.Wait()

	task.mu.Lock()

	if ctx.Err() != nil {
		task.Status = "cancelled"
	} else if task.Failed > 0 {
		task.Status = "completed_with_errors"
	} else {
		task.Status = "completed"
	}

	task.mu.Unlock()
}

func (d *Downloader) downloadFile(
	ctx context.Context,
	task *Task,
	workDir string,
	index int,
	file asmr.File,
) {

	output, err := safePath(
		workDir,
		file.Path,
	)

	if err != nil {

		task.mu.Lock()

		task.Files[index].Status = "failed"
		task.Files[index].Error = err.Error()
		task.Failed++

		task.mu.Unlock()

		return
	}

	if err := os.MkdirAll(
		filepath.Dir(output),
		0755,
	); err != nil {

		task.mu.Lock()

		task.Files[index].Status = "failed"
		task.Files[index].Error = err.Error()
		task.Failed++

		task.mu.Unlock()

		return
	}

	// 已经完整下载
	if file.Size > 0 {

		if stat, err := os.Stat(output); err == nil {

			if stat.Size() == file.Size {

				task.mu.Lock()

				task.Files[index].Downloaded =
					file.Size

				task.Files[index].Status =
					"skipped"

				task.Success++

				task.mu.Unlock()

				atomic.AddInt64(
					&task.BytesDownloaded,
					file.Size,
				)

				return
			}
		}
	}

	part := output + ".part"

	var lastErr error

	for attempt := 1; attempt <= d.RetryCount; attempt++ {

		if ctx.Err() != nil {
			return
		}

		err := d.downloadOnce(
			ctx,
			task,
			index,
			file,
			part,
		)

		if err == nil {

			if err := os.Rename(
				part,
				output,
			); err != nil {

				lastErr = err

				continue
			}

			task.mu.Lock()

			task.Files[index].Status =
				"completed"

			task.Success++

			task.mu.Unlock()

			return
		}

		lastErr = err

		time.Sleep(
			time.Duration(attempt) *
				time.Second,
		)
	}

	task.mu.Lock()

	task.Files[index].Status =
		"failed"

	task.Files[index].Error =
		lastErr.Error()

	task.Failed++

	task.mu.Unlock()
}

func (d *Downloader) downloadOnce(
	ctx context.Context,
	task *Task,
	index int,
	file asmr.File,
	part string,
) error {

	var existing int64

	if stat, err := os.Stat(part); err == nil {
		existing = stat.Size()
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		file.URL,
		nil,
	)

	if err != nil {
		return err
	}

	req.Header.Set(
		"User-Agent",
		"Mozilla/5.0 "+
			"(X11; Linux x86_64) "+
			"AppleWebKit/537.36 "+
			"Chrome/131.0 Safari/537.36",
	)

	req.Header.Set(
		"Referer",
		"https://asmr.one/",
	)

	if existing > 0 {
		req.Header.Set(
			"Range",
			fmt.Sprintf(
				"bytes=%d-",
				existing,
			),
		)
	}

	resp, err := d.Client.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusPartialContent {

		return fmt.Errorf(
			"HTTP %d",
			resp.StatusCode,
		)
	}

	// 如果服务器不支持 Range，则重新下载
	if existing > 0 &&
		resp.StatusCode == http.StatusOK {

		existing = 0
	}

	flags := os.O_CREATE | os.O_WRONLY

	if existing == 0 {
		flags |= os.O_TRUNC
	}

	out, err := os.OpenFile(
		part,
		flags,
		0644,
	)

	if err != nil {
		return err
	}

	defer out.Close()

	if existing > 0 {

		if _, err := out.Seek(
			existing,
			io.SeekStart,
		); err != nil {
			return err
		}
	}

	downloaded := existing

	task.mu.Lock()

	task.Files[index].Downloaded =
		downloaded

	task.Files[index].Status =
		"downloading"

	task.mu.Unlock()

	buf := make([]byte, 1024*1024)

	for {

		if ctx.Err() != nil {
			return ctx.Err()
		}

		n, err := resp.Body.Read(buf)

		if n > 0 {

			if _, writeErr := out.Write(
				buf[:n],
			); writeErr != nil {
				return writeErr
			}

			downloaded += int64(n)

			task.mu.Lock()

			task.Files[index].Downloaded =
				downloaded

			task.mu.Unlock()

			atomic.AddInt64(
				&task.BytesDownloaded,
				int64(n),
			)
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}
	}

	// 如果 API 提供文件大小，校验
	if file.Size > 0 &&
		downloaded != file.Size {

		return fmt.Errorf(
			"文件大小不一致: got=%d expected=%d",
			downloaded,
			file.Size,
		)
	}

	return nil
}
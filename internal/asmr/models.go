package asmr

type WorkInfo struct {
	Title string `json:"title"`
}

type Track struct {
	Type             string  `json:"type"`
	Title            string  `json:"title"`
	MediaDownloadURL string  `json:"mediaDownloadUrl"`
	Size             int64   `json:"size"`
	Children         []Track `json:"children"`
}

type File struct {
	Type string `json:"type"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
	Path string `json:"path"`
}

type Work struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Files []File `json:"files"`
}
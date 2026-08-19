package asmr

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const defaultAPIHost = "https://api.asmr-200.com"

var rjRegex = regexp.MustCompile(`(?i)^(?:RJ)?(\d+)$`)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient() *Client {

	baseURL := os.Getenv("ASMR_API_HOST")

	if baseURL == "" {
		baseURL = defaultAPIHost
	}

	baseURL = strings.TrimRight(
		baseURL,
		"/",
	)

	return &Client{
		BaseURL: baseURL,

		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func NormalizeRJ(value string) (string, error) {

	value = strings.TrimSpace(value)

	match := rjRegex.FindStringSubmatch(value)

	if len(match) != 2 {
		return "", fmt.Errorf(
			"无效的 RJ 号: %s",
			value,
		)
	}

	return match[1], nil
}

func (c *Client) requestJSON(
	endpoint string,
	target any,
) error {

	req, err := http.NewRequest(
		http.MethodGet,
		c.BaseURL+endpoint,
		nil,
	)

	if err != nil {
		return err
	}

	req.Header.Set(
		"User-Agent",
		"Mozilla/5.0 (X11; Linux x86_64) "+
			"AppleWebKit/537.36 "+
			"(KHTML, like Gecko) "+
			"Chrome/131.0 Safari/537.36",
	)

	req.Header.Set(
		"Origin",
		"https://asmr.one",
	)

	req.Header.Set(
		"Referer",
		"https://asmr.one/",
	)

	resp, err := c.HTTPClient.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 ||
		resp.StatusCode >= 300 {

		body, _ := io.ReadAll(
			io.LimitReader(
				resp.Body,
				4096,
			),
		)

		return fmt.Errorf(
			"API 返回 HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(
				string(body),
			),
		)
	}

	return json.NewDecoder(
		resp.Body,
	).Decode(target)
}

func (c *Client) GetWorkInfo(
	id string,
) (*WorkInfo, error) {

	var result WorkInfo

	err := c.requestJSON(
		"/api/workInfo/"+url.PathEscape(id),
		&result,
	)

	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) GetTracks(
	id string,
) ([]Track, error) {

	var result []Track

	err := c.requestJSON(
		"/api/tracks/"+url.PathEscape(id)+"?v=2",
		&result,
	)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func flattenTracks(
	tracks []Track,
	parent string,
	result *[]File,
) {

	for _, item := range tracks {

		switch item.Type {

		case "folder":

			next := parent

			if item.Title != "" {

				next = filepath.Join(
					parent,
					item.Title,
				)
			}

			flattenTracks(
				item.Children,
				next,
				result,
			)

		case "audio", "text", "image":

			if item.MediaDownloadURL == "" {
				continue
			}

			*result = append(
				*result,
				File{
					Type: item.Type,

					Name: item.Title,

					URL: item.MediaDownloadURL,

					Size: item.Size,

					Path: filepath.Join(
						parent,
						item.Title,
					),
				},
			)
		}
	}
}

func BuildFiles(
	tracks []Track,
) []File {

	result := make(
		[]File,
		0,
	)

	flattenTracks(
		tracks,
		"",
		&result,
	)

	return result
}

func (c *Client) GetWork(
	id string,
) (*Work, error) {

	info, err := c.GetWorkInfo(id)

	if err != nil {
		return nil, err
	}

	tracks, err := c.GetTracks(id)

	if err != nil {
		return nil, err
	}

	return &Work{
		ID: id,

		Title: info.Title,

		Files: BuildFiles(tracks),
	}, nil
}
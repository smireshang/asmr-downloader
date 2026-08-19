package selector

import (
	"path/filepath"
	"sort"
	"strings"

	"asmr-downloader/internal/asmr"
)

var audioExtensions = map[string]bool{
	".wav": true,
	".mp3": true,
}

var textExtensions = map[string]bool{
	".vtt": true,
	".lrc": true,
}

var imageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

func extension(name string) string {
	return strings.ToLower(
		filepath.Ext(name),
	)
}

func isAudio(name string) bool {
	return audioExtensions[
		extension(name),
	]
}

func audioPriority(name string) int {
	switch extension(name) {

	case ".wav":
		return 2

	case ".mp3":
		return 1

	default:
		return 0
	}
}

func trackKey(file asmr.File) string {
	dir := filepath.Dir(file.Path)

	name := strings.TrimSuffix(
		filepath.Base(file.Name),
		filepath.Ext(file.Name),
	)

	return strings.ToLower(
		filepath.Join(dir, name),
	)
}

func Select(files []asmr.File) []asmr.File {

	var result []asmr.File

	audio := make(
		map[string]asmr.File,
	)

	for _, file := range files {

		ext := extension(file.Name)

		// 音频
		if isAudio(file.Name) {

			key := trackKey(file)

			current, exists := audio[key]

			if !exists ||
				audioPriority(file.Name) >
					audioPriority(current.Name) {

				audio[key] = file
			}

			continue
		}

		// VTT / LRC
		if textExtensions[ext] {
			result = append(
				result,
				file,
			)

			continue
		}

		// 图片
		if imageExtensions[ext] {
			result = append(
				result,
				file,
			)

			continue
		}

		// 其他全部忽略
	}

	for _, file := range audio {
		result = append(
			result,
			file,
		)
	}

	sort.Slice(
		result,
		func(i, j int) bool {
			return strings.ToLower(
				result[i].Path,
			) < strings.ToLower(
				result[j].Path,
			)
		},
	)

	return result
}
package utility

import (
	"archive/zip"
	"fmt"
	logs "github.com/rs/zerolog/log"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func CreateFile(path string) error {
	return os.MkdirAll(path, os.ModePerm)
}

func DeleteFile(path string) error {
	return os.Remove(path)
}

func DeleteAll(path string) error {
	return os.RemoveAll(path)
}

func RemoveExceptDockerfile(path string) error {
	files, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to open directory: %w", err)
	}

	for _, file := range files {
		if file.Name() != "Dockerfile" {
			err = os.Remove(filepath.Join(path, file.Name()))
			if err != nil {
				return fmt.Errorf("failed to remove old file: %w", err)
			}
		}
	}
	return nil
}

func Unzip(src, dest string) error {
	if err := RemoveExceptDockerfile(dest); err != nil {
		return fmt.Errorf("failed to clean up old files: %w", err)
	}
	files, err := os.ReadDir(dest)
	if err != nil {
		return fmt.Errorf("failed to open directory: %w", err)
	}

	for _, file := range files {
		if file.Name() != "Dockerfile" {
			err = os.Remove(filepath.Join(dest, file.Name()))
			if err != nil {
				return fmt.Errorf("failed to remove old files: %w", err)
			}
		}
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("failed to open zip reader: %w", err)
	}
	defer func() {
		if err = r.Close(); err != nil {
			logs.Error().Err(err).Msg("failed to close zip reader")
		}
	}()
	for _, f := range r.File {
		filePath := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(filePath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", filePath)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, f.Mode()); err != nil {
				return fmt.Errorf("failed to create folder: %w", err)
			}
			continue
		}

		outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("failed to open out file: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			if err = outFile.Close(); err != nil {
				logs.Error().Err(err).Msg("failed to close out file")
			}
			return fmt.Errorf("failed to open file: %w", err)
		}

		_, err = io.Copy(outFile, rc)
		if err = rc.Close(); err != nil {
			logs.Error().Err(err).Msg("failed to close file")
		}
		if err = outFile.Close(); err != nil {
			logs.Error().Err(err).Msg("failed to close out file")
		}
		if err != nil {
			return fmt.Errorf("failed to copy content: %w", err)
		}

	}
	return nil
}

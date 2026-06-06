package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultHTTPTimeout = 10 * time.Minute
	maxErrorBodyBytes  = 4096
)

type config struct {
	SignURL   string
	MimeType  string
	StoreType string
	FilePath  string
	Title     string
}

type signRequest struct {
	ServiceName string `json:"serviceName"`
	StoreType   string `json:"storeType"`
	FileSize    int64  `json:"fileSize"`
	MimeType    string `json:"mimeType"`
	Title       string `json:"title,omitempty"`
}

type signResponse struct {
	UploadURL  string `json:"uploadUrl"`
	UploadMeta struct {
		Key       string `json:"key"`
		Signature string `json:"signature"`
	} `json:"uploadMeta"`
}

type uploadResponse struct {
	ResourceID string `json:"resourceId"`
}

func main() {
	printDebugInfo()

	if err := run(os.Args[1:], os.Getenv, newHTTPClient()); err != nil {
		fmt.Printf("upload-actions error: %v\n", err)
		os.Exit(1)
	}
}

func printDebugInfo() {
	pwd, err := filepath.Abs(".")
	if err != nil {
		fmt.Printf("Current working directory error: %v\n", err)
		return
	}
	fmt.Printf("Current working directory: %s\n", pwd)

	files, err := filepath.Glob("*")
	if err != nil {
		fmt.Printf("Current directory files error: %v\n", err)
		return
	}
	fmt.Printf("Current directory files: %v\n", files)
}

func run(args []string, getenv func(string) string, client *http.Client) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	if client == nil {
		client = newHTTPClient()
	}

	fileInfo, err := os.Stat(cfg.FilePath)
	if err != nil {
		return fmt.Errorf("file not exists: %w", err)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("file path is a directory: %s", cfg.FilePath)
	}

	signResp, err := requestUploadSign(client, cfg, fileInfo.Size())
	if err != nil {
		return err
	}

	srcFile, err := os.Open(cfg.FilePath)
	if err != nil {
		return fmt.Errorf("open file failed: %w", err)
	}
	defer srcFile.Close()

	responseBody, err := uploadFile(
		client,
		signResp.UploadURL,
		map[string]string{
			"key":   signResp.UploadMeta.Key,
			"token": signResp.UploadMeta.Signature,
		},
		fileInfo.Name(),
		srcFile,
	)
	if err != nil {
		return err
	}

	var uploadResp uploadResponse
	if err := json.Unmarshal(responseBody, &uploadResp); err != nil {
		return fmt.Errorf("decode upload response failed: %w", err)
	}
	if uploadResp.ResourceID == "" {
		return fmt.Errorf("resourceId is empty")
	}

	return writeOutput(getenv("GITHUB_OUTPUT"), "resId", uploadResp.ResourceID)
}

func parseArgs(args []string) (config, error) {
	if len(args) < 4 {
		return config{}, fmt.Errorf("expected at least 4 arguments, got %d", len(args))
	}

	cfg := config{
		SignURL:   args[0],
		MimeType:  args[1],
		StoreType: args[2],
		FilePath:  args[3],
	}
	if len(args) >= 5 {
		cfg.Title = args[4]
	}

	if cfg.SignURL == "" {
		return config{}, fmt.Errorf("signUrl is empty")
	}
	if cfg.MimeType == "" {
		return config{}, fmt.Errorf("mimeType is empty")
	}
	if cfg.StoreType == "" {
		return config{}, fmt.Errorf("st is empty")
	}
	if cfg.FilePath == "" {
		return config{}, fmt.Errorf("filePath is empty")
	}

	return cfg, nil
}

func requestUploadSign(client *http.Client, cfg config, fileSize int64) (signResponse, error) {
	signBody := signRequest{
		ServiceName: "",
		StoreType:   cfg.StoreType,
		FileSize:    fileSize,
		MimeType:    cfg.MimeType,
		Title:       cfg.Title,
	}

	signData, err := json.Marshal(signBody)
	if err != nil {
		return signResponse{}, fmt.Errorf("encode sign request failed: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, cfg.SignURL, bytes.NewReader(signData))
	if err != nil {
		return signResponse{}, fmt.Errorf("create sign request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return signResponse{}, fmt.Errorf("sign request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return signResponse{}, fmt.Errorf("read sign response failed: %w", err)
	}
	if !isSuccessStatus(resp.StatusCode) {
		return signResponse{}, fmt.Errorf("sign request failed: status %d: %s", resp.StatusCode, responseSnippet(body))
	}

	var signResp signResponse
	if err := json.Unmarshal(body, &signResp); err != nil {
		return signResponse{}, fmt.Errorf("decode sign response failed: %w", err)
	}
	if signResp.UploadURL == "" {
		return signResponse{}, fmt.Errorf("uploadUrl is empty")
	}
	if signResp.UploadMeta.Key == "" {
		return signResponse{}, fmt.Errorf("uploadMeta.key is empty")
	}
	if signResp.UploadMeta.Signature == "" {
		return signResponse{}, fmt.Errorf("uploadMeta.signature is empty")
	}

	return signResp, nil
}

func uploadFile(client *http.Client, url string, params map[string]string, filename string, file io.Reader) ([]byte, error) {
	if client == nil {
		client = newHTTPClient()
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)

	req, err := http.NewRequest(http.MethodPost, url, pipeReader)
	if err != nil {
		return nil, fmt.Errorf("create upload request failed: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	writeErrCh := make(chan error, 1)
	go func() {
		if err := writeMultipartBody(writer, params, filename, file); err != nil {
			_ = pipeWriter.CloseWithError(err)
			writeErrCh <- err
			return
		}
		writeErrCh <- pipeWriter.Close()
	}()

	resp, err := client.Do(req)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		_ = waitForBodyWriter(writeErrCh)
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		_ = pipeReader.CloseWithError(readErr)
		_ = waitForBodyWriter(writeErrCh)
		return nil, fmt.Errorf("read upload response failed: %w", readErr)
	}
	if !isSuccessStatus(resp.StatusCode) {
		statusErr := fmt.Errorf("upload request failed: status %d: %s", resp.StatusCode, responseSnippet(body))
		_ = pipeReader.CloseWithError(statusErr)
		_ = waitForBodyWriter(writeErrCh)
		return nil, statusErr
	}

	writeErr := waitForBodyWriter(writeErrCh)
	if writeErr != nil {
		return nil, fmt.Errorf("write upload body failed: %w", writeErr)
	}

	return body, nil
}

func writeMultipartBody(writer *multipart.Writer, params map[string]string, filename string, file io.Reader) error {
	formFile, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return err
	}
	if _, err := io.Copy(formFile, file); err != nil {
		return err
	}
	for key, val := range params {
		if err := writer.WriteField(key, val); err != nil {
			return err
		}
	}
	return writer.Close()
}

func writeOutput(path, name, value string) error {
	if path == "" {
		return fmt.Errorf("GITHUB_OUTPUT is empty")
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open GITHUB_OUTPUT failed: %w", err)
	}
	defer file.Close()

	if _, err := fmt.Fprintf(file, "%s=%s\n", name, value); err != nil {
		return fmt.Errorf("write GITHUB_OUTPUT failed: %w", err)
	}
	return nil
}

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: defaultHTTPTimeout}
}

func waitForBodyWriter(errCh <-chan error) error {
	return <-errCh
}

func isSuccessStatus(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}

func responseSnippet(body []byte) string {
	if len(body) <= maxErrorBodyBytes {
		return string(body)
	}
	return string(body[:maxErrorBodyBytes]) + "..."
}

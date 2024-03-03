package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/tidwall/gjson"
)

const uploadUrl = "http://file-manage.server.svc.cluster.local:8080/api/rest/file-manage/internal/proxy/upload/v1"

var (
	st       = os.Args[1]
	filePath = os.Args[2]
)

func main() {
	str, _ := os.Getwd()
	fmt.Printf("pwd: %s\n", str)
	files, _ := filepath.Glob("*")
	fmt.Println(files)
	if st == "" {
		panic("st is empty")
	}
	if filePath == "" {
		panic("filePath is empty")
	}
	fileInfo, ok := IsExists(filePath)
	if !ok {
		panic("file not exists")
	}

	srcFile, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}

	params := map[string]string{
		"fileName":    fileInfo.Name(),
		"serviceName": "xhu-timetable",
		"storeType":   st,
	}

	responseBody, err := uploadFile(uploadUrl, params, fileInfo.Name(), srcFile)
	if err != nil {
		panic(err)
	}
	fmt.Printf("responseBody: %s\n", responseBody)

	resId := gjson.Get(string(responseBody), "resourceId").String()
	fmt.Printf(`echo "resId=%s" >> $GITHUB_OUTPUT`, resId)
}

func uploadFile(url string, params map[string]string, filename string, file io.Reader) ([]byte, error) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	formFile, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(formFile, file)
	if err != nil {
		return nil, err
	}
	for key, val := range params {
		_ = writer.WriteField(key, val)
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func IsExists(path string) (os.FileInfo, bool) {
	f, err := os.Stat(path)
	return f, err == nil || os.IsExist(err)
}

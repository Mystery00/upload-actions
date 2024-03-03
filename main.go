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

	"github.com/tidwall/gjson"
)

var (
	signUrl  = os.Args[1]
	mimeType = os.Args[2]
	st       = os.Args[3]
	filePath = os.Args[4]
	title    = os.Args[5]
)

func main() {
	str, _ := os.Getwd()
	fmt.Printf("pwd: %s\n", str)
	files, _ := filepath.Glob("*")
	fmt.Println(files)
	if signUrl == "" {
		panic("signUrl is empty")
	}
	if mimeType == "" {
		panic("mimeType is empty")
	}
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
	signBody := make(map[string]interface{})
	signBody["serviceName"] = ""
	signBody["storeType"] = st
	signBody["fileSize"] = fileInfo.Size()
	signBody["mimeType"] = mimeType
	if title != "" {
		signBody["title"] = title
	}

	client := &http.Client{}
	signData, err := json.Marshal(signBody)
	if err != nil {
		panic(err)
	}
	req, err := http.NewRequest("POST", signUrl, bytes.NewReader(signData))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	uploadUrl := gjson.Get(string(body), "uploadUrl").String()
	key := gjson.Get(string(body), "uploadMeta.key").String()
	token := gjson.Get(string(body), "uploadMeta.signature").String()

	srcFile, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}

	params := map[string]string{
		"key":   key,
		"token": token,
	}

	responseBody, err := uploadFile(uploadUrl, params, fileInfo.Name(), srcFile)
	if err != nil {
		panic(err)
	}

	resId := gjson.Get(string(responseBody), "resourceId").String()
	fmt.Printf(`::set-output name=resId::%s`, resId)
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

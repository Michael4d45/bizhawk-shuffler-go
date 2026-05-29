package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/michael4d45/bizshuffle/savestate"
)

func postJSON(base, path string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return http.Post(base+path, "application/json", bytes.NewReader(data))
}

func postGameInstances(t *testing.T, base string, instances []map[string]any) {
	t.Helper()
	res, err := postJSON(base, "/api/games", map[string]any{"game_instances": instances})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("POST /api/games status %d: %s", res.StatusCode, b)
	}
}

// UploadMinimalSave posts a valid BizHawk savestate for instanceID to /save/upload.
func UploadMinimalSave(base, instanceID string) error {
	saveBytes, err := savestate.BuildMinimalBizHawkSavestate()
	if err != nil {
		return err
	}
	filename := instanceID + ".state"
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("save", filename)
	if err != nil {
		return err
	}
	if _, err := fw.Write(saveBytes); err != nil {
		return err
	}
	_ = w.WriteField("filename", filename)
	if err := w.Close(); err != nil {
		return err
	}
	res, err := http.Post(base+"/save/upload", w.FormDataContentType(), &buf)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("upload %s: %s", res.Status, b)
	}
	return nil
}

func uploadSaveMultipart(t *testing.T, base, filename string, data []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("save", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	_ = w.WriteField("filename", filename)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(base+"/save/upload", w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

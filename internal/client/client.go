package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"readerhub/internal/bundle"
	"readerhub/internal/protocol"
	"strings"
	"time"
)

type Client struct {
	URL   string
	Token string
	HTTP  *http.Client
}

func (c Client) Submit(ctx context.Context, b bundle.Bundle) (protocol.Receipt, error) {
	var receipt protocol.Receipt
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	manifest, err := json.Marshal(b.Upload)
	if err != nil {
		return receipt, err
	}
	if err = writer.WriteField("manifest", string(manifest)); err != nil {
		return receipt, err
	}
	for id, data := range b.Files {
		part, err := writer.CreateFormFile(id, id)
		if err != nil {
			return receipt, err
		}
		if _, err = part.Write(data); err != nil {
			return receipt, err
		}
	}
	if err = writer.Close(); err != nil {
		return receipt, err
	}
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 2 * time.Minute}
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/api/submissions", bytes.NewReader(body.Bytes()))
		if err != nil {
			return receipt, err
		}
		request.Header.Set("Content-Type", writer.FormDataContentType())
		request.Header.Set("Authorization", "Bearer "+c.Token)
		response, err := h.Do(request)
		if err != nil {
			lastErr = err
		} else {
			payload, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			response.Body.Close()
			if readErr == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
				if err = json.Unmarshal(payload, &receipt); err != nil {
					return receipt, err
				}
				return receipt, nil
			}
			var apiErr struct {
				Error string `json:"error"`
			}
			json.Unmarshal(payload, &apiErr)
			lastErr = fmt.Errorf("Hub HTTP %d: %s", response.StatusCode, apiErr.Error)
			if response.StatusCode < 500 {
				return receipt, lastErr
			}
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return receipt, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 300 * time.Millisecond):
			}
		}
	}
	return receipt, lastErr
}

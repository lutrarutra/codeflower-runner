package client

import (
	"context"
	"fmt"
	"net/http"
)

var PUBLIC_BACKEND_URL = "http://localhost:8000" // If you change the location of this variable, update the LDFLAGS in the release workflow

func GetAPIClient(token string) (*ClientWithResponses, error) {
	if token == "" {
		return nil, fmt.Errorf("API token is not set.")
	}

	addToken := func(ctx context.Context, req *http.Request) error {
		req.Header.Set("X-API-Token", token)
		return nil
	}

	return NewClientWithResponses(PUBLIC_BACKEND_URL, WithRequestEditorFn(addToken))
}

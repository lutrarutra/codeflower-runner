package client

import (
	"context"
	"fmt"
	"net/http"
)

func GetAPIClient(token string) (*ClientWithResponses, error) {
	if token == "" {
		return nil, fmt.Errorf("API token is not set.")
	}

	addToken := func(ctx context.Context, req *http.Request) error {
		req.Header.Set("X-API-Token", token)
		return nil
	}

	return NewClientWithResponses("http://localhost:8000", WithRequestEditorFn(addToken))
}

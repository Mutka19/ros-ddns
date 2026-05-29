package dns

import (
	"context"
	"errors"
	"os"
	"time"
)

type Provider interface {
	Update(context.Context, string, time.Time) error
}

func NewProvider() (*Provider, error) {
	providerName := os.Getenv("PROVIDER")
	if providerName == "" {
		return nil, errors.New("PROVIDER environment variable is required")
	}

	return nil, nil
}

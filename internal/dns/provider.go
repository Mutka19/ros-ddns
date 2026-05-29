package dns

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

type Provider interface {
	Update(context.Context, string, time.Time) error
}

// FactoryFunc defines the signature for a provider's constructor
type FactoryFunc func() (Provider, error)

// registry holds the registered provider creators (maps provider name to constructor function)
var registry = make(map[string]FactoryFunc)

// RegisterProvider allows registration of a provider
func RegisterProvider(name string, factory FactoryFunc) {
	registry[name] = factory
}

func NewProvider() (Provider, error) {
	providerName := os.Getenv("PROVIDER")
	if providerName == "" {
		return nil, errors.New("PROVIDER environment variable is required")
	}

	factory, exists := registry[providerName]
	if !exists {
		return nil, fmt.Errorf("unsupported provider: %s", providerName)
	}

	return factory()
}

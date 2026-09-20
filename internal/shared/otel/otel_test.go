package otel

import (
	"context"
	"testing"
	"time"
)

func TestSetupOTelSDKMergesResources(t *testing.T) {
	shutdown, err := SetupOTelSDK(context.Background(), "test-service", "127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = shutdown(ctx)
}

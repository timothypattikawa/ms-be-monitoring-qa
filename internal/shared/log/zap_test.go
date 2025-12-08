package log

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewLogger(t *testing.T) {
	logger, err := NewLogger(NewDefaultOption())
	assert.NoError(t, err)
	assert.NotNil(t, logger)

	defer logger.Close()

	type Struct struct {
		ID  int64  `json:"id"`
		Key string `json:"key"`
	}

	ctx := context.WithValue(context.Background(), "X-Request-MemberID", "fdasdf-f123jf-123nf")
	logger.WithField("user_id", 23123213).WithContext(ctx).Info("api success")
	logger.WithField("user_id", 23123213).WithContext(ctx).Error("failed something")
	logger.WithField("user_id", 23123213).WithContext(ctx).Debugf("failed something and debug it")

	mapsFromObject := Struct{
		ID:  100,
		Key: "test-sgfrt",
	}
	logger.WithMap(mapsFromObject).Info("test map object")
	logger.WithMap(&mapsFromObject).Info("test map object pointer")

	mapsFromMaps := make(map[string]any)
	mapsFromMaps["id"] = 100
	mapsFromMaps["key"] = "test map maps"
	logger.WithMap(mapsFromObject).Info("test map maps")
}

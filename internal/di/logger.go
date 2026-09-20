package di

import "github.com/Beyondtech-ID/boiler-plate-be-api/internal/shared/log"

func NewLogger() log.Logger { return log.GetLogger() }

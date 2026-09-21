package di

import "github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/shared/log"

func NewLogger() log.Logger { return log.GetLogger() }

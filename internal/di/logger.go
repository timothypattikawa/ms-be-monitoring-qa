package di

import "github.com/Beyondtech-ID/ms-backbone-emoney/internal/shared/log"

func NewLogger() log.Logger { return log.GetLogger() }

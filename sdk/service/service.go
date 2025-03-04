package service

import (
	"fmt"

	"github.com/ChenBigdata421/jxt-core/logger"
	"github.com/ChenBigdata421/jxt-core/storage"
	"gorm.io/gorm"
)

type Service struct {
	Orm   *gorm.DB
	Msg   string
	MsgID string
	Log   *logger.Helper
	Error error
	Cache storage.AdapterCache
}

func (db *Service) AddError(err error) error {
	if db.Error == nil {
		db.Error = err
	} else if err != nil {
		db.Error = fmt.Errorf("%v; %w", db.Error, err)
	}
	return db.Error
}

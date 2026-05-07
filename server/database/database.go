package database

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/esp32-rss-display/backend/server/config"
	"github.com/esp32-rss-display/backend/server/models"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Init(cfg *config.DatabaseConfig) error {
	var db *gorm.DB
	var err error

	switch cfg.Driver {
	case "mysql":
		db, err = gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
	case "sqlite":
		fallthrough
	default:
		if err := os.MkdirAll(filepath.Dir(cfg.DSN), 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}
		db, err = gorm.Open(sqlite.Open(cfg.DSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
	}

	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	if db.Migrator().HasTable(&models.Item{}) {
		if err := normalizeItemURLs(db); err != nil {
			return fmt.Errorf("failed to normalize existing item urls: %w", err)
		}
	}

	if err := db.AutoMigrate(
		&models.Device{},
		&models.Feed{},
		&models.Item{},
		&models.ItemShow{},
		&models.ItemRead{},
		&models.ItemRating{},
	); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	DB = db
	log.Println("Database connected and migrated successfully")
	return nil
}

func GetDB() *gorm.DB {
	return DB
}

func normalizeItemURLs(db *gorm.DB) error {
	type itemRow struct {
		ID     uint
		FeedID uint
		URL    string
	}

	var items []itemRow
	if err := db.Model(&models.Item{}).Select("id, feed_id, url").Where("url LIKE ?", "%#%").Order("feed_id, id").Find(&items).Error; err != nil {
		return err
	}

	for _, item := range items {
		normalized := normalizeItemURL(item.URL)
		if normalized == "" || normalized == item.URL {
			continue
		}

		if err := mergeNormalizedItem(db, item.ID, item.FeedID, normalized); err != nil {
			return err
		}
	}

	return nil
}

func mergeNormalizedItem(db *gorm.DB, sourceID, feedID uint, normalizedURL string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var source models.Item
		if err := tx.First(&source, sourceID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		if source.URL == normalizedURL {
			return nil
		}

		var existing models.Item
		err := tx.Where("feed_id = ? AND url = ?", feedID, normalizedURL).First(&existing).Error
		switch {
		case err == nil:
			if err := reassignItemReferences(tx, source.ID, existing.ID); err != nil {
				return err
			}
			return tx.Delete(&models.Item{}, source.ID).Error
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.Model(&models.Item{}).Where("id = ?", source.ID).Update("url", normalizedURL).Error
		default:
			return err
		}
	})
}

func reassignItemReferences(tx *gorm.DB, fromID, toID uint) error {
	if fromID == toID {
		return nil
	}

	if err := tx.Model(&models.Device{}).Where("current_item_id = ?", fromID).Update("current_item_id", toID).Error; err != nil {
		return err
	}

	if err := reassignRows(tx, &models.ItemShow{}, fromID, toID); err != nil {
		return err
	}
	if tx.Migrator().HasTable(&models.ItemRead{}) {
		if err := reassignRows(tx, &models.ItemRead{}, fromID, toID); err != nil {
			return err
		}
	}
	if tx.Migrator().HasTable(&models.ItemRating{}) {
		if err := reassignRows(tx, &models.ItemRating{}, fromID, toID); err != nil {
			return err
		}
	}

	return nil
}

func reassignRows(tx *gorm.DB, model any, fromID, toID uint) error {
	if !tx.Migrator().HasTable(model) {
		return nil
	}

	return tx.Model(model).Where("item_id = ?", fromID).Update("item_id", toID).Error
}

func normalizeItemURL(raw string) string {
	if raw == "" {
		return ""
	}

	idx := strings.IndexByte(raw, '#')
	if idx == -1 {
		return raw
	}

	return raw[:idx]
}
